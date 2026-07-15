package postgres

import (
	"context"
	"fmt"
	"log"
	"path"
	"sort"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/razorpay/aidocs/api/internal/auth"
	dbsqlc "github.com/razorpay/aidocs/api/internal/db/sqlc"
	"github.com/razorpay/aidocs/api/internal/repo"
)

type storedRepositoryImportFile struct {
	input     repo.RepositoryImportFile
	document  repo.Document
	versionID string
	blobKey   string
}

// ImportRepository uploads immutable blobs first, then creates the complete
// repository tree in one PostgreSQL transaction. Any upload or transaction
// failure compensates by deleting all newly written blobs, so a partial import
// is never visible through aidocs.
//
//nolint:gocyclo // Folder creation, document/version inserts, and blob compensation are one atomic import workflow.
func (s *Store) ImportRepository(ctx context.Context, creator auth.Principal, params repo.CreateRepositoryParams, kind string, inputs []repo.RepositoryImportFile) (repo.RepositoryImportResult, error) {
	files, err := repo.ValidateRepositoryImport(kind, inputs)
	if err != nil {
		return repo.RepositoryImportResult{}, err
	}
	if params.Name == "" || params.Slug == "" || (params.Scope != repo.RepositoryScopePersonal && params.Scope != repo.RepositoryScopeOrganization) || params.RootFolderID != "" || params.DocumentID != "" {
		return repo.RepositoryImportResult{}, fmt.Errorf("%w: invalid repository settings", repo.ErrInvalidRepositoryImport)
	}

	rootID := newID("fld")
	repositoryID := newID("repo")
	directoryIDs := map[string]string{"": rootID}
	for _, directory := range importDirectories(files) {
		directoryIDs[directory] = newID("fld")
	}
	stored := make([]storedRepositoryImportFile, 0, len(files))
	for _, file := range files {
		documentID := newID("doc")
		versionID := newID("ver")
		directory := path.Dir(file.Path)
		if directory == "." {
			directory = ""
		}
		stored = append(stored, storedRepositoryImportFile{
			input: file,
			document: repo.Document{
				ID: documentID, Title: importTitle(file.Path), Owner: creator,
				CurrentVersionID: versionID, ParentFolderID: directoryIDs[directory],
			},
			versionID: versionID,
			blobKey:   fmt.Sprintf("docs/%s/%s.html", documentID, versionID),
		})
	}

	uploaded := make([]string, 0, len(stored))
	committed := false
	defer func() {
		if committed {
			return
		}
		for _, key := range uploaded {
			if cleanupErr := s.blobs.Delete(context.WithoutCancel(ctx), key); cleanupErr != nil {
				log.Printf("repository import rollback blob cleanup failed blob_key=%q error=%v", key, cleanupErr)
			}
		}
	}()
	for _, file := range stored {
		if err := s.blobs.Put(ctx, file.blobKey, htmlContentType, file.input.HTML); err != nil {
			return repo.RepositoryImportResult{}, err
		}
		uploaded = append(uploaded, file.blobKey)
	}

	tx, err := s.db.Begin(ctx)
	if err != nil {
		return repo.RepositoryImportResult{}, err
	}
	defer rollback(ctx, tx)
	q := s.q.WithTx(tx)
	if creator.Type == auth.PrincipalUser {
		if err := upsertUser(ctx, q, creator); err != nil {
			return repo.RepositoryImportResult{}, err
		}
	}

	if _, err := tx.Exec(ctx, `INSERT INTO folders(id,title,owner_type,owner_id) VALUES($1,$2,$3,$4)`, rootID, params.Name, string(creator.Type), creator.ID); err != nil {
		return repo.RepositoryImportResult{}, repositoryWriteError(err)
	}
	for _, directory := range importDirectories(files) {
		parent := path.Dir(directory)
		if parent == "." {
			parent = ""
		}
		if _, err := tx.Exec(ctx, `INSERT INTO folders(id,title,owner_type,owner_id,parent_folder_id) VALUES($1,$2,$3,$4,$5)`, directoryIDs[directory], path.Base(directory), string(creator.Type), creator.ID, directoryIDs[parent]); err != nil {
			return repo.RepositoryImportResult{}, repositoryWriteError(err)
		}
	}
	for _, file := range stored {
		if _, err := tx.Exec(ctx, `INSERT INTO documents(id,title,owner_type,owner_id,parent_folder_id) VALUES($1,$2,$3,$4,$5)`, file.document.ID, file.document.Title, string(creator.Type), creator.ID, file.document.ParentFolderID); err != nil {
			return repo.RepositoryImportResult{}, repositoryWriteError(err)
		}
		if err := q.InsertInitialVersion(ctx, dbsqlc.InsertInitialVersionParams{
			ID: file.versionID, DocumentID: file.document.ID, HtmlBlobKey: file.blobKey,
			Sha256: importSHA(file.input.HTML), CreatedByType: string(creator.Type), CreatedByID: creator.ID,
		}); err != nil {
			return repo.RepositoryImportResult{}, err
		}
		if err := q.UpdateDocumentCurrentVersion(ctx, dbsqlc.UpdateDocumentCurrentVersionParams{CurrentVersionID: textValue(file.versionID), ID: file.document.ID}); err != nil {
			return repo.RepositoryImportResult{}, err
		}
	}

	_, err = tx.Exec(ctx, `
		INSERT INTO repositories(
			id,name,slug,description,scope,namespace_id,namespace_name,root_folder_id,
			created_by_type,created_by_id,created_by_email,created_by_name,
			required_approvals,protected
		) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14)`,
		repositoryID, params.Name, params.Slug, params.Description, params.Scope,
		params.NamespaceID, params.NamespaceName, rootID, string(creator.Type), creator.ID,
		creator.Email, creator.Name, params.RequiredApprovals, params.Protected)
	if err != nil {
		return repo.RepositoryImportResult{}, repositoryWriteError(err)
	}
	if params.GrantOrgViewer {
		_, err = tx.Exec(ctx, `
			INSERT INTO resource_grants(
				id,resource_type,resource_id,principal_type,principal_id,role,
				granted_by_type,granted_by_user_id
			) VALUES($1,'folder',$2,'anyone','','viewer',$3,$4)`,
			newID("gr"), rootID, string(creator.Type), creator.ID)
		if err != nil {
			return repo.RepositoryImportResult{}, err
		}
	}

	repository, err := scanRepository(tx.QueryRow(ctx, repositorySelect+` WHERE r.id=$1`, repositoryID))
	if err != nil {
		return repo.RepositoryImportResult{}, err
	}
	// A connection failure during Commit is outcome-ambiguous: PostgreSQL may
	// have committed even though the client did not receive confirmation. Keep
	// the blobs in that case so a successfully committed repository can never
	// reference deleted content. A later orphan sweep can safely remove blobs
	// when the transaction actually rolled back.
	committed = true
	if err := tx.Commit(ctx); err != nil {
		return repo.RepositoryImportResult{}, err
	}
	documents := make([]repo.Document, 0, len(stored))
	for _, file := range stored {
		documents = append(documents, file.document)
	}
	return repo.RepositoryImportResult{Repository: repository, Documents: documents}, nil
}

func importDirectories(files []repo.RepositoryImportFile) []string {
	// The package-level validator owns ordering. Reconstructing the directory
	// list here keeps the storage implementation independent of unexported
	// domain helpers.
	seen := map[string]bool{}
	for _, file := range files {
		for directory := path.Dir(file.Path); directory != "." && directory != ""; directory = path.Dir(directory) {
			seen[directory] = true
		}
	}
	directories := make([]string, 0, len(seen))
	for directory := range seen {
		directories = append(directories, directory)
	}
	sortImportDirectories(directories)
	return directories
}

func sortImportDirectories(directories []string) {
	sort.Slice(directories, func(i, j int) bool {
		leftDepth := strings.Count(directories[i], "/")
		rightDepth := strings.Count(directories[j], "/")
		if leftDepth == rightDepth {
			return directories[i] < directories[j]
		}
		return leftDepth < rightDepth
	})
}

func importTitle(relativePath string) string {
	base := path.Base(relativePath)
	return strings.TrimSuffix(base, path.Ext(base))
}

func importSHA(html []byte) string { return sha(html) }

func textValue(value string) pgtype.Text { return pgtype.Text{String: value, Valid: true} }
