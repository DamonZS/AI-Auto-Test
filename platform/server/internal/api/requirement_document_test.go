package api

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestRequirementDocumentPreviewAndCommit(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	handler := New(database).Routes()
	upload := func(project, preview string) *httptest.ResponseRecorder {
		t.Helper()
		var content bytes.Buffer
		multipartBody := multipart.NewWriter(&content)
		part, err := multipartBody.CreateFormFile("file", "订单说明.csv")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := part.Write([]byte("\ufeffcode,title,content\nR-101,订单查询,订单列表应显示已确认状态。\n")); err != nil {
			t.Fatal(err)
		}
		if err := multipartBody.Close(); err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/api/requirements/documents?project_id="+project+"&preview="+preview, &content)
		request.Header.Set(tenantHeader, "tenant-a")
		request.Header.Set("Content-Type", multipartBody.FormDataContentType())
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)
		return response
	}
	if response := upload("project-b", "1"); response.Code != http.StatusNotFound {
		t.Fatalf("foreign project preview: %d %s", response.Code, response.Body)
	}
	preview := upload("project-a", "1")
	if preview.Code != http.StatusOK || !bytes.Contains(preview.Body.Bytes(), []byte("订单列表应显示已确认状态。")) {
		t.Fatalf("preview: %d %s", preview.Code, preview.Body)
	}
	var count int
	if err := database.DB.QueryRow("SELECT count(*) FROM requirements WHERE code = 'R-101'").Scan(&count); err != nil || count != 0 {
		t.Fatalf("preview created data: count=%d err=%v", count, err)
	}
	commit := upload("project-a", "0")
	var result struct{ IDs []string }
	if err := json.Unmarshal(commit.Body.Bytes(), &result); err != nil || commit.Code != http.StatusCreated || len(result.IDs) != 1 {
		t.Fatalf("import: %d %s", commit.Code, commit.Body)
	}
	history := serveTenant(handler, http.MethodGet, "/api/requirements/"+result.IDs[0]+"/history", "tenant-a", nil)
	if history.Code != http.StatusOK || !bytes.Contains(history.Body.Bytes(), []byte(`"revision_no":1`)) {
		t.Fatalf("initial revision: %d %s", history.Code, history.Body)
	}
}
