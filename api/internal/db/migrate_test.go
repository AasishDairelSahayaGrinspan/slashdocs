package db_test

import (
	"context"
	"database/sql"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"

	"github.com/pressly/goose/v3"

	aidb "github.com/razorpay/aidocs/api/internal/db"
	"github.com/razorpay/aidocs/api/internal/repo"
	pgrepo "github.com/razorpay/aidocs/api/internal/repo/postgres"
	"github.com/razorpay/aidocs/api/internal/testdb"
)

func TestCommentMentionMigrationsSplitTriggerFromLegacyCleanup(t *testing.T) {
	databaseURL := testdb.New(t)
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	setMigrationTestFS(t)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(database, "migrations", 17); err != nil {
		t.Fatalf("down to migration 17: %v", err)
	}

	seedMigrationGraph(t, database)
	const marker = "\u2063"
	insertMigrationComment(t, database, "cmt_legacy_marker", "Hello "+marker+"@admin"+marker)

	// Migration 18 is deliberately short: it installs schema and the trigger,
	// but does not scan and rewrite the existing comments table.
	if err := goose.UpTo(database, "migrations", 18); err != nil {
		t.Fatalf("up to migration 18: %v", err)
	}
	if got := migrationCommentBody(t, database, "cmt_legacy_marker"); !strings.Contains(got, marker) {
		t.Fatalf("migration 18 unexpectedly scanned legacy body %q", got)
	}
	insertMigrationComment(t, database, "cmt_after_trigger_legacy", "Legacy "+marker+"@admin"+marker)
	if got := migrationCommentBody(t, database, "cmt_after_trigger_legacy"); got != "Legacy @admin" {
		t.Fatalf("post-18 legacy body = %q, want database invariant to remove markers", got)
	}
	insertMigrationComment(t, database, "cmt_structured", "New @admin")

	// Status updates preserve associations. Any body write from an old writer,
	// including a marker-only change with the same visible text, clears offsets
	// whose identity and placement can no longer be trusted.
	if _, err := database.Exec(`
		INSERT INTO comment_mentions(comment_id, principal_type, principal_id, offset_utf16, length_utf16, label_text)
		VALUES ('cmt_structured', 'user', 'usr_migration', 4, 6, '@admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE comments SET status = 'resolved' WHERE id = 'cmt_structured'`); err != nil {
		t.Fatal(err)
	}
	assertMigrationMentionCount(t, database, "cmt_structured", 1)
	if _, err := database.Exec(`UPDATE comments SET body = body || chr(8291) WHERE id = 'cmt_structured'`); err != nil {
		t.Fatal(err)
	}
	if got := migrationCommentBody(t, database, "cmt_structured"); got != "New @admin" {
		t.Fatalf("marker-only old-writer body update = %q", got)
	}
	assertMigrationMentionCount(t, database, "cmt_structured", 0)
	if _, err := database.Exec(`
		INSERT INTO comment_mentions(comment_id, principal_type, principal_id, offset_utf16, length_utf16, label_text)
		VALUES ('cmt_structured', 'user', 'usr_migration', 4, 6, '@admin')`); err != nil {
		t.Fatal(err)
	}
	if _, err := database.Exec(`UPDATE comments SET body = 'Changed ' || chr(8291) || '@admin' WHERE id = 'cmt_structured'`); err != nil {
		t.Fatal(err)
	}
	if got := migrationCommentBody(t, database, "cmt_structured"); got != "Changed @admin" {
		t.Fatalf("old-writer body update = %q", got)
	}
	assertMigrationMentionCount(t, database, "cmt_structured", 0)

	// Exercise the production migrator through the latest embedded migration;
	// migration 19's cleanup must remain intact as later schema changes run.
	if err := aidb.Migrate(databaseURL); err != nil {
		t.Fatalf("production migrate through latest: %v", err)
	}
	if got := migrationCommentBody(t, database, "cmt_legacy_marker"); got != "Hello @admin" {
		t.Fatalf("migration 19 cleanup body = %q", got)
	}
	if got := migrationCommentBody(t, database, "cmt_after_trigger_legacy"); got != "Legacy @admin" {
		t.Fatalf("migration 18 invariant body changed after cleanup: %q", got)
	}
	assertMentionMigrationObjects(t, database)
	version, err := goose.GetDBVersion(database)
	wantVersion := latestMigrationVersion(t)
	if err != nil || version != wantVersion {
		t.Fatalf("migration version = %d, %v; want latest %d", version, err, wantVersion)
	}

	// The cleanup cannot reconstruct markers, so migration 19's Down remains a
	// deliberate no-op even after later migrations are rolled back. Migration
	// 18's objects must remain usable at version 18.
	setMigrationTestFS(t)
	if err := goose.DownTo(database, "migrations", 18); err != nil {
		t.Fatalf("down migration 19: %v", err)
	}
	if got := migrationCommentBody(t, database, "cmt_legacy_marker"); got != "Hello @admin" {
		t.Fatalf("migration 19 Down reconstructed body %q", got)
	}
	assertMentionMigrationObjects(t, database)

	// A full rollback removes the invariant and all structured metadata. Legacy
	// servers can write their marker format again, but migration 19's historical
	// cleanup remains irreversible.
	if err := goose.DownTo(database, "migrations", 17); err != nil {
		t.Fatalf("down migration 18: %v", err)
	}
	assertMentionMigrationObjectsAbsent(t, database)
	insertMigrationComment(t, database, "cmt_after_rollback", "Rollback "+marker+"@admin"+marker)
	if got := migrationCommentBody(t, database, "cmt_after_rollback"); got != "Rollback "+marker+"@admin"+marker {
		t.Fatalf("legacy write after rollback = %q, want markers preserved", got)
	}
	if err := aidb.Migrate(databaseURL); err != nil {
		t.Fatalf("restore latest migrations: %v", err)
	}
}

func TestCaseInsensitiveUserEmailMigrationPreservesIdentityReferences(t *testing.T) {
	databaseURL := testdb.New(t)
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	setMigrationTestFS(t)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(database, "migrations", 22); err != nil {
		t.Fatalf("down to migration 22: %v", err)
	}

	if _, err := database.Exec(`
		INSERT INTO users(id,email,name,picture_url,google_sub,created_at) VALUES
		  ('usr_email_real','Owner@Example.COM','Signed-in owner','real.png','oidc:real','2024-01-03T00:00:00Z'),
		  ('usr_email_placeholder',' owner@example.com ','Invited owner','',NULL, '2024-01-02T00:00:00Z'),
		  ('usr_email_placeholder_two','OWNER@example.com','','',NULL, '2024-01-01T00:00:00Z');

		INSERT INTO folders(id,title,owner_type,owner_id) VALUES
		  ('fld_email_merge','Email merge root','user','usr_email_placeholder');
		INSERT INTO documents(id,title,owner_id,owner_type,parent_folder_id) VALUES
		  ('doc_email_merge','Email merge document','usr_email_placeholder','user','fld_email_merge');
		INSERT INTO versions(id,document_id,number,html_blob_key,sha256,created_by_type,created_by_id) VALUES
		  ('ver_email_base','doc_email_merge',1,'email-base.html','base','user','usr_email_placeholder'),
		  ('ver_email_head','doc_email_merge',NULL,'email-head.html','head','user','usr_email_placeholder_two');
		UPDATE documents SET current_version_id='ver_email_base' WHERE id='doc_email_merge';

		INSERT INTO service_accounts(id,name,owner_user_id,created_by_user_id) VALUES
		  ('sa_email_merge','email-merge-bot','usr_email_placeholder','usr_email_real');
		INSERT INTO cli_credentials(id,user_id,name,token_hash) VALUES
		  ('cred_email_merge','usr_email_placeholder','legacy','email-merge-token');

		INSERT INTO resource_grants(
		  id,resource_type,resource_id,principal_type,principal_id,role,
		  granted_by_type,granted_by_user_id,created_at,revoked_at
		) VALUES
		  ('gr_email_real','document','doc_email_merge','user','usr_email_real','viewer','user','usr_email_real','2024-01-01T00:00:00Z','2024-01-04T00:00:00Z'),
		  ('gr_email_placeholder','document','doc_email_merge','user','usr_email_placeholder','editor','user','usr_email_placeholder_two','2024-01-02T00:00:00Z',NULL);

		INSERT INTO document_workspace_entries(document_id,principal_type,principal_id,added_at) VALUES
		  ('doc_email_merge','user','usr_email_real','2024-01-03T00:00:00Z'),
		  ('doc_email_merge','user','usr_email_placeholder','2024-01-01T00:00:00Z');
		INSERT INTO folder_workspace_entries(folder_id,principal_type,principal_id,added_at,last_opened_at) VALUES
		  ('fld_email_merge','user','usr_email_real','2024-01-03T00:00:00Z','2024-01-03T00:00:00Z'),
		  ('fld_email_merge','user','usr_email_placeholder','2024-01-01T00:00:00Z','2024-01-05T00:00:00Z');
		INSERT INTO pins(principal_type,principal_id,resource_type,resource_id,pinned_at) VALUES
		  ('user','usr_email_real','document','doc_email_merge','2024-01-03T00:00:00Z'),
		  ('user','usr_email_placeholder','document','doc_email_merge','2024-01-01T00:00:00Z');

		INSERT INTO comments(
		  id,document_id,created_on_version_id,author_type,author_id,author_email,author_name,
		  body,selected_text,anchor_json,root_comment_id
		) VALUES (
		  'cmt_email_merge','doc_email_merge','ver_email_base','user','usr_email_placeholder',
		  ' owner@example.com ','Invited owner','Mention','text','{}'::jsonb,'cmt_email_merge'
		);
		INSERT INTO comment_mentions(comment_id,principal_type,principal_id,offset_utf16,length_utf16,label_text)
		VALUES ('cmt_email_merge','user','usr_email_placeholder_two',0,8,'@owner');

		INSERT INTO ownership_transfers(id,service_account_id,from_user_id,to_user_id,status)
		VALUES ('xfer_email_merge','sa_email_merge','usr_email_real','usr_email_placeholder','pending');
		INSERT INTO document_kv(document_id,key,value,revision,updated_by_type,updated_by_id,updated_by_email)
		VALUES ('doc_email_merge','identity','{}'::jsonb,1,'user','usr_email_placeholder',' owner@example.com ');
		INSERT INTO document_kv_events(document_id,revision,changes,updated_by_type,updated_by_id,updated_by_email)
		VALUES ('doc_email_merge',1,'[]'::jsonb,'user','usr_email_placeholder_two','OWNER@example.com');

		INSERT INTO repositories(
		  id,name,slug,scope,namespace_id,namespace_name,root_folder_id,
		  created_by_type,created_by_id,created_by_email,created_by_name
		) VALUES (
		  'repo_email_merge','Email merge','email-merge','personal','usr_email_placeholder',
		  'Invited owner','fld_email_merge','user','usr_email_placeholder',' owner@example.com ','Invited owner'
		);
		INSERT INTO pull_requests(
		  id,repository_id,number,document_id,base_version_id,head_version_id,patchset,
		  title,author_type,author_id,author_email,author_name
		) VALUES (
		  'pr_email_merge','repo_email_merge',1,'doc_email_merge','ver_email_base','ver_email_head',1,
		  'Email merge PR','user','usr_email_placeholder_two','OWNER@example.com','Invited owner'
		);
		INSERT INTO pull_request_changes(
		  id,pull_request_id,patchset,position,operation,document_id,folder_id,
		  base_version_id,head_version_id,base_title,title,head_html_blob_key,head_sha256
		) VALUES (
		  'pr_email_merge_change', 'pr_email_merge',1,1,'update','doc_email_merge','fld_email_merge',
		  'ver_email_base','ver_email_head','Email merge document','Email merge document','email-head.html','head'
		);
		INSERT INTO pull_request_reviews(
		  id,pull_request_id,head_version_id,patchset,reviewer_type,reviewer_id,
		  reviewer_email,reviewer_name,state,body,submitted_at
		) VALUES
		  ('review_email_real','pr_email_merge','ver_email_head',1,'user','usr_email_real','Owner@Example.COM','Signed-in owner','approved','old','2024-01-01T00:00:00Z'),
		  ('review_email_placeholder','pr_email_merge','ver_email_head',1,'user','usr_email_placeholder',' owner@example.com ','Invited owner','changes_requested','new','2024-01-02T00:00:00Z');

		INSERT INTO connected_knowledge_records(kind,id,repository_id,user_id,payload) VALUES (
		  'notification','notification_email_merge','repo_email_merge','usr_email_placeholder',
		  '{"user_id":"usr_email_placeholder","member_ids":["usr_email_placeholder_two"],"temporary_grant_id":"gr_email_placeholder","review_id":"review_email_placeholder"}'::jsonb
		);
	`); err != nil {
		t.Fatalf("seed case-colliding identities: %v", err)
	}

	if err := goose.UpTo(database, "migrations", 23); err != nil {
		t.Fatalf("up migration 23: %v", err)
	}

	var userID, email, googleSub string
	if err := database.QueryRow(`SELECT id,email::text,google_sub FROM users WHERE email='OWNER@EXAMPLE.COM'`).Scan(&userID, &email, &googleSub); err != nil {
		t.Fatalf("case-insensitive user lookup: %v", err)
	}
	if userID != "usr_email_real" || email != "owner@example.com" || googleSub != "oidc:real" {
		t.Fatalf("canonical user = %q %q %q", userID, email, googleSub)
	}
	var duplicateCount int
	if err := database.QueryRow(`SELECT count(*) FROM users WHERE id IN ('usr_email_placeholder','usr_email_placeholder_two')`).Scan(&duplicateCount); err != nil || duplicateCount != 0 {
		t.Fatalf("duplicate users remaining = %d, err=%v", duplicateCount, err)
	}

	var documentOwner, folderOwner, serviceAccountOwner, credentialOwner string
	if err := database.QueryRow(`
		SELECT d.owner_id,f.owner_id,sa.owner_user_id,c.user_id
		FROM documents d
		JOIN folders f ON f.id='fld_email_merge'
		JOIN service_accounts sa ON sa.id='sa_email_merge'
		JOIN cli_credentials c ON c.id='cred_email_merge'
		WHERE d.id='doc_email_merge'`).Scan(&documentOwner, &folderOwner, &serviceAccountOwner, &credentialOwner); err != nil {
		t.Fatal(err)
	}
	for label, got := range map[string]string{
		"document owner": documentOwner, "folder owner": folderOwner,
		"service account owner": serviceAccountOwner, "credential owner": credentialOwner,
	} {
		if got != "usr_email_real" {
			t.Fatalf("%s = %q, want canonical user", label, got)
		}
	}

	var grantID, grantPrincipal, grantRole string
	var grantRevoked sql.NullTime
	if err := database.QueryRow(`
		SELECT id,principal_id,role,revoked_at
		FROM resource_grants
		WHERE resource_type='document' AND resource_id='doc_email_merge' AND principal_type='user'`).Scan(
		&grantID, &grantPrincipal, &grantRole, &grantRevoked,
	); err != nil {
		t.Fatal(err)
	}
	if grantID != "gr_email_real" || grantPrincipal != "usr_email_real" || grantRole != "editor" || grantRevoked.Valid {
		t.Fatalf("merged grant = id:%q principal:%q role:%q revoked:%v", grantID, grantPrincipal, grantRole, grantRevoked)
	}

	var transferTarget, repositoryNamespace, repositoryCreator, pullRequestAuthor string
	if err := database.QueryRow(`
		SELECT x.to_user_id,r.namespace_id,r.created_by_id,pr.author_id
		FROM ownership_transfers x
		JOIN repositories r ON r.id='repo_email_merge'
		JOIN pull_requests pr ON pr.id='pr_email_merge'
		WHERE x.id='xfer_email_merge'`).Scan(&transferTarget, &repositoryNamespace, &repositoryCreator, &pullRequestAuthor); err != nil {
		t.Fatal(err)
	}
	for label, got := range map[string]string{
		"transfer target": transferTarget, "repository namespace": repositoryNamespace,
		"repository creator": repositoryCreator, "pull request author": pullRequestAuthor,
	} {
		if got != "usr_email_real" {
			t.Fatalf("%s = %q, want canonical user", label, got)
		}
	}

	var reviewCount int
	var reviewID, reviewerID, reviewState string
	if err := database.QueryRow(`
		SELECT count(*),min(id),min(reviewer_id),min(state)
		FROM pull_request_reviews WHERE pull_request_id='pr_email_merge'`).Scan(
		&reviewCount, &reviewID, &reviewerID, &reviewState,
	); err != nil {
		t.Fatal(err)
	}
	if reviewCount != 1 || reviewID != "review_email_real" || reviewerID != "usr_email_real" || reviewState != "changes_requested" {
		t.Fatalf("merged review = count:%d id:%q reviewer:%q state:%q", reviewCount, reviewID, reviewerID, reviewState)
	}

	var payload string
	if err := database.QueryRow(`SELECT payload::text FROM connected_knowledge_records WHERE kind='notification' AND id='notification_email_merge'`).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	for _, stale := range []string{"usr_email_placeholder", "usr_email_placeholder_two", "gr_email_placeholder", "review_email_placeholder"} {
		if strings.Contains(payload, stale) {
			t.Fatalf("knowledge payload retained %q: %s", stale, payload)
		}
	}
	for _, current := range []string{"usr_email_real", "gr_email_real", "review_email_real"} {
		if !strings.Contains(payload, current) {
			t.Fatalf("knowledge payload missing %q: %s", current, payload)
		}
	}

	store, err := pgrepo.Connect(context.Background(), databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(store.Close)
	loggedIn, err := store.UpsertGoogleUser(context.Background(), "usr_email_new", " OWNER@example.com ", "Refreshed owner", "oidc:real", "refreshed.png")
	if err != nil {
		t.Fatalf("login after identity migration: %v", err)
	}
	if loggedIn.ID != "usr_email_real" || loggedIn.Email != "owner@example.com" {
		t.Fatalf("logged-in identity = %#v", loggedIn)
	}
	invited, err := store.EnsureUserByEmail(context.Background(), "OwNeR@ExAmPlE.cOm")
	if err != nil || invited.ID != loggedIn.ID {
		t.Fatalf("case-insensitive share target = %#v, err=%v", invited, err)
	}
	role, err := store.RoleForDocument(context.Background(), loggedIn, "doc_email_merge")
	if err != nil || role != repo.RoleOwner {
		t.Fatalf("preserved effective role = %q, err=%v", role, err)
	}
}

func TestCaseInsensitiveUserEmailMigrationRejectsAmbiguousAuthenticatedIdentities(t *testing.T) {
	databaseURL := testdb.New(t)
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := database.Close(); err != nil {
			t.Errorf("close database: %v", err)
		}
	})
	setMigrationTestFS(t)
	if err := goose.SetDialect("postgres"); err != nil {
		t.Fatal(err)
	}
	if err := goose.DownTo(database, "migrations", 22); err != nil {
		t.Fatalf("down to migration 22: %v", err)
	}
	if _, err := database.Exec(`
		INSERT INTO users(id,email,name,google_sub) VALUES
		  ('usr_email_subject_one','Collision@Example.com','First subject','oidc:subject-one'),
		  ('usr_email_subject_two','collision@example.COM','Second subject','oidc:subject-two')`); err != nil {
		t.Fatal(err)
	}

	err = goose.UpTo(database, "migrations", 23)
	if err == nil || !strings.Contains(err.Error(), "cannot merge multiple authenticated users") {
		t.Fatalf("migration error = %v, want explicit authenticated-identity conflict", err)
	}
	version, versionErr := goose.GetDBVersion(database)
	if versionErr != nil || version != 22 {
		t.Fatalf("version after rejected migration = %d, %v; want 22", version, versionErr)
	}
	var count int
	if err := database.QueryRow(`
		SELECT count(*) FROM users
		WHERE id IN ('usr_email_subject_one','usr_email_subject_two')
		  AND google_sub IS NOT NULL`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("authenticated rows after rejected migration = %d, want both preserved", count)
	}
	if got := migrationCommentBody(t, database, "cmt_after_rollback"); got != "Rollback @admin" {
		t.Fatalf("cleanup after re-upgrade body = %q", got)
	}
}

func setMigrationTestFS(t *testing.T) {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	goose.SetBaseFS(os.DirFS(filepath.Dir(filename)))
}

func latestMigrationVersion(t *testing.T) int64 {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("resolve migration test path")
	}
	matches, err := filepath.Glob(filepath.Join(filepath.Dir(filename), "migrations", "*.sql"))
	if err != nil || len(matches) == 0 {
		t.Fatalf("list migrations: %v", err)
	}
	base := filepath.Base(matches[len(matches)-1])
	prefix, _, ok := strings.Cut(base, "_")
	if !ok {
		t.Fatalf("invalid migration filename %q", base)
	}
	version, err := strconv.ParseInt(prefix, 10, 64)
	if err != nil {
		t.Fatalf("parse migration version %q: %v", prefix, err)
	}
	return version
}

func seedMigrationGraph(t *testing.T, database *sql.DB) {
	t.Helper()
	tx, err := database.Begin()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = tx.Rollback() }()
	statements := []string{
		`INSERT INTO users(id, email, name) VALUES ('usr_migration', 'migration@example.com', 'admin')`,
		`INSERT INTO documents(id, title, owner_id, owner_type) VALUES ('doc_migration', 'Migration', 'usr_migration', 'user')`,
		`INSERT INTO versions(id, document_id, number, html_blob_key, sha256, created_by_type, created_by_id) VALUES ('ver_migration', 'doc_migration', 1, 'migration.html', 'sha', 'user', 'usr_migration')`,
		`UPDATE documents SET current_version_id = 'ver_migration' WHERE id = 'doc_migration'`,
	}
	for _, statement := range statements {
		if _, err := tx.Exec(statement); err != nil {
			t.Fatal(err)
		}
	}
	if err := tx.Commit(); err != nil {
		t.Fatal(err)
	}
}

func insertMigrationComment(t *testing.T, database *sql.DB, id, body string) {
	t.Helper()
	if _, err := database.Exec(`
		INSERT INTO comments(id, document_id, created_on_version_id, author_type, author_id, author_email, author_name, body, selected_text, anchor_json, root_comment_id)
		VALUES ($1, 'doc_migration', 'ver_migration', 'user', 'usr_migration', 'migration@example.com', 'admin', $2, 'fixture', '{}'::jsonb, $1)`, id, body); err != nil {
		t.Fatal(err)
	}
}

func migrationCommentBody(t *testing.T, database *sql.DB, id string) string {
	t.Helper()
	var body string
	if err := database.QueryRow(`SELECT body FROM comments WHERE id = $1`, id).Scan(&body); err != nil {
		t.Fatal(err)
	}
	return body
}

func assertMigrationMentionCount(t *testing.T, database *sql.DB, commentID string, want int) {
	t.Helper()
	var got int
	if err := database.QueryRow(`SELECT count(*) FROM comment_mentions WHERE comment_id = $1`, commentID).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatalf("comment %s mention count = %d, want %d", commentID, got, want)
	}
}

func assertMentionMigrationObjects(t *testing.T, database *sql.DB) {
	t.Helper()
	var table, index sql.NullString
	if err := database.QueryRow(`SELECT to_regclass('public.comment_mentions')::text, to_regclass('public.idx_comment_mentions_principal')::text`).Scan(&table, &index); err != nil {
		t.Fatal(err)
	}
	if !table.Valid || !index.Valid {
		t.Fatalf("migration objects = table %v index %v", table, index)
	}
	var trigger bool
	if err := database.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'normalize_comment_body_mentions_trigger' AND NOT tgisinternal)`).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	if !trigger {
		t.Fatal("normalization trigger is missing")
	}
}

func assertMentionMigrationObjectsAbsent(t *testing.T, database *sql.DB) {
	t.Helper()
	var table, index sql.NullString
	if err := database.QueryRow(`SELECT to_regclass('public.comment_mentions')::text, to_regclass('public.idx_comment_mentions_principal')::text`).Scan(&table, &index); err != nil {
		t.Fatal(err)
	}
	if table.Valid || index.Valid {
		t.Fatalf("mention migration objects remain after rollback: table %v index %v", table, index)
	}
	var trigger bool
	if err := database.QueryRow(`SELECT EXISTS (SELECT 1 FROM pg_trigger WHERE tgname = 'normalize_comment_body_mentions_trigger' AND NOT tgisinternal)`).Scan(&trigger); err != nil {
		t.Fatal(err)
	}
	if trigger {
		t.Fatal("normalization trigger remains after rollback")
	}
}
