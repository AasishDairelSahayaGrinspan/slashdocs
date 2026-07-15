//go:build testfixtures

package server_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"testing"
)

type repositoryImportUpload struct {
	path string
	html string
}

func repositoryImportBody(t *testing.T, fields map[string]string, uploads []repositoryImportUpload) (*bytes.Buffer, string) {
	t.Helper()
	var body bytes.Buffer
	writer := multipart.NewWriter(&body)
	for key, value := range fields {
		if err := writer.WriteField(key, value); err != nil {
			t.Fatal(err)
		}
	}
	for _, upload := range uploads {
		if upload.path != "" {
			if err := writer.WriteField("paths", upload.path); err != nil {
				t.Fatal(err)
			}
		}
		part, err := writer.CreateFormFile("files", upload.path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte(upload.html)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return &body, writer.FormDataContentType()
}

func TestImportRepositoryFromLocalFolder(t *testing.T) {
	h := newTestServer()
	body, contentType := repositoryImportBody(t, map[string]string{
		"name": "Uploaded handbook", "slug": "uploaded-handbook", "scope": "personal",
		"import_kind": "folder", "required_approvals": "2", "protected": "true",
	}, []repositoryImportUpload{
		{path: "guides/start.html", html: "<html><body>start</body></html>"},
		{path: "overview.htm", html: "<html><body>overview</body></html>"},
	})
	response := doRaw(t, h, http.MethodPost, "/v1/repositories/import", body, map[string]string{
		"Content-Type": contentType, "X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, response, http.StatusCreated)
	var result struct {
		Repository repositoryHTTPResponse `json:"repository"`
		Documents  []struct {
			ID               string `json:"id"`
			Title            string `json:"title"`
			CurrentVersionID string `json:"current_version_id"`
			ParentFolderID   string `json:"parent_folder_id"`
		} `json:"documents"`
	}
	decodeRepositoryJSON(t, response, &result)
	if result.Repository.ID == "" || result.Repository.Slug != "uploaded-handbook" || result.Repository.RequiredApprovals != 2 {
		t.Fatalf("repository import response = %s", response.Body.String())
	}
	if len(result.Documents) != 2 || result.Documents[0].Title != "start" || result.Documents[1].Title != "overview" {
		t.Fatalf("repository import documents = %+v", result.Documents)
	}
	if result.Documents[0].ParentFolderID == result.Repository.RootFolderID || result.Documents[1].ParentFolderID != result.Repository.RootFolderID {
		t.Fatalf("repository import did not preserve folder structure: %+v", result.Documents)
	}
	for _, document := range result.Documents {
		if document.ID == "" || document.CurrentVersionID == "" {
			t.Fatalf("repository import document is incomplete: %+v", document)
		}
	}
	assertRepositoryContainsDocument(t, h, result.Repository.ID, result.Documents[0].ID, repositoryOwner)
	assertRepositoryContainsDocument(t, h, result.Repository.ID, result.Documents[1].ID, repositoryOwner)
}

func TestImportRepositoryFromLocalLooseDocument(t *testing.T) {
	h := newTestServer()
	body, contentType := repositoryImportBody(t, map[string]string{
		"name": "Uploaded document", "scope": "personal", "import_kind": "document",
	}, []repositoryImportUpload{{path: "runbook.html", html: "<html><body>runbook</body></html>"}})
	response := doRaw(t, h, http.MethodPost, "/v1/repositories/import", body, map[string]string{
		"Content-Type": contentType, "X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, response, http.StatusCreated)
	var result struct {
		Repository repositoryHTTPResponse `json:"repository"`
		Documents  []struct {
			Title          string `json:"title"`
			ParentFolderID string `json:"parent_folder_id"`
		} `json:"documents"`
	}
	decodeRepositoryJSON(t, response, &result)
	if len(result.Documents) != 1 || result.Documents[0].Title != "runbook" || result.Documents[0].ParentFolderID != result.Repository.RootFolderID {
		t.Fatalf("loose document import response = %s", response.Body.String())
	}
}

func TestImportRepositoryRejectsTraversalWithoutPartialState(t *testing.T) {
	h := newTestServer()
	body, contentType := repositoryImportBody(t, map[string]string{
		"name": "Unsafe upload", "slug": "unsafe-upload", "scope": "personal", "import_kind": "folder",
	}, []repositoryImportUpload{
		{path: "valid.html", html: "<p>valid</p>"},
		{path: "../escape.html", html: "<p>escape</p>"},
	})
	response := doRaw(t, h, http.MethodPost, "/v1/repositories/import", body, map[string]string{
		"Content-Type": contentType, "X-Test-Principal": repositoryOwner,
	})
	assertStatus(t, response, http.StatusBadRequest)
	assertRepositoryErrorCode(t, response, "invalid_repository_import")

	list := do(t, h, http.MethodGet, "/v1/repositories", "", map[string]string{"X-Test-Principal": repositoryOwner})
	assertStatus(t, list, http.StatusOK)
	var repositories struct {
		Items []repositoryHTTPResponse `json:"items"`
	}
	decodeRepositoryJSON(t, list, &repositories)
	for _, repository := range repositories.Items {
		if repository.Slug == "unsafe-upload" {
			t.Fatalf("failed import left repository behind: %s", list.Body.String())
		}
	}
}
