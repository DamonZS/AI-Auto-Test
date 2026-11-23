package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"platform/server/internal/store"
)

func TestProjectWorkspace(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	if _, err := database.DB.Exec(`
		INSERT INTO projects (id, tenant_id, name, base_url) VALUES ('other-a', 'tenant-a', 'Other', 'https://other.test');
		INSERT INTO versions (id, tenant_id, project_id, name) VALUES
			('release-a', 'tenant-a', 'project-a', '1.0'), ('release-other', 'tenant-a', 'other-a', '1.0'),
			('release-b', 'tenant-b', 'project-b', '1.0');
		INSERT INTO environments (id, tenant_id, project_id, name, base_url)
			VALUES ('legacy', 'tenant-a', 'project-a', '旧环境', 'https://old.test');
		INSERT INTO runs (id, tenant_id, case_id, status, passed) VALUES
			('foreign-run', 'tenant-b', 'case-a', 'finished', 0);
		UPDATE runs SET status = 'finished', passed = 1 WHERE id = 'run-a';
		UPDATE cases SET status = 'approved' WHERE id = 'case-a';
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.MigrateProjectWorkspace(database.DB); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []struct{ name, tenant, role string }{
		{"workspace-editor", "tenant-a", "editor"}, {"workspace-viewer", "tenant-a", "viewer"}, {"workspace-other", "tenant-b", "owner"},
	} {
		if err := ProvisionUser(t.Context(), database, account.name, fixturePassword, account.tenant, "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	mux := New(database, AuthOptions{}).Routes()
	editor := authLogin(t, mux, "workspace-editor")
	viewer := authLogin(t, mux, "workspace-viewer")
	other := authLogin(t, mux, "workspace-other")
	const project = "/api/projects/project-a"
	const environments = project + "/environments"
	const legacy = "/api/environments/legacy"
	const edit = `{"name":"项目新版","base_url":"https://project.test/path","allowed_hosts":"api.project.test","code":"QA-02","lead":"测试负责人","lifecycle":"planning"}`
	authRequest(t, mux, "PATCH", project, "tenant-a", viewer, edit, http.StatusForbidden)
	authRequest(t, mux, "PATCH", project, "tenant-b", other, edit, http.StatusNotFound)
	authRequest(t, mux, "PATCH", project, "tenant-a", editor, `{"base_url":"invalid"}`, http.StatusBadRequest)
	authRequest(t, mux, "PATCH", project, "tenant-a", editor, edit, http.StatusNoContent)
	response := authRequest(t, mux, "GET", project, "tenant-a", viewer, "", http.StatusOK)
	var detail struct {
		Name         string
		BaseURL      string `json:"base_url"`
		AllowedHosts string `json:"allowed_hosts"`
		Code         string
		Lead         string
		Lifecycle    string
		Metrics      map[string]int
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil || detail.Name != "项目新版" ||
		detail.BaseURL != "https://project.test/path" || detail.AllowedHosts != "api.project.test" ||
		detail.Code != "QA-02" || detail.Lead != "测试负责人" || detail.Lifecycle != "planning" ||
		detail.Metrics["approved_cases"] != 1 || detail.Metrics["versions"] != 1 || detail.Metrics["environments"] != 1 ||
		detail.Metrics["runs"] != 1 || detail.Metrics["passed_runs"] != 1 {
		t.Fatalf("project detail = %s (%v)", response.Body, err)
	}
	authRequest(t, mux, "GET", project, "tenant-b", other, "", http.StatusNotFound)
	for _, invalid := range []string{`{"level":"preview"}`, `{"source":"invalid"}`, `{"name":" "}`} {
		authRequest(t, mux, "PATCH", legacy, "tenant-a", editor, invalid, http.StatusBadRequest)
	}
	for _, version := range []string{"release-other", "release-b"} {
		authRequest(t, mux, "PATCH", legacy, "tenant-a", editor, mustJSON(map[string]string{"version_id": version}), http.StatusNotFound)
	}
	const environment = `{"name":"预发","base_url":"https://staging.test","level":"staging","version_id":"release-a"}`
	authRequest(t, mux, "POST", environments, "tenant-a", viewer, environment, http.StatusForbidden)
	authRequest(t, mux, "POST", environments, "tenant-b", other, environment, http.StatusNotFound)
	response = authRequest(t, mux, "POST", environments, "tenant-a", editor, environment, http.StatusCreated)
	var created struct{ ID string }
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("create environment = %s (%v)", response.Body, err)
	}
	authRequest(t, mux, "PATCH", legacy, "tenant-a", editor, `{"level":"test","version_id":"release-a"}`, http.StatusNoContent)
	authRequest(t, mux, "PATCH", legacy, "tenant-a", editor, `{"version_id":""}`, http.StatusNoContent)
	authRequest(t, mux, "POST", "/api/environments", "tenant-a", editor, `{"project_id":"project-a","name":"兼容","base_url":"https://legacy.test"}`, http.StatusOK)
	response = authRequest(t, mux, "GET", "/api/environments", "tenant-a", viewer, "", http.StatusOK)
	var rows []map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil || len(rows) != 3 {
		t.Fatalf("environment list = %s (%v)", response.Body, err)
	}
	for _, row := range rows {
		switch row["id"] {
		case "legacy":
			if row["name"] != "旧环境" || row["level"] != "test" || row["version_id"] != "" {
				t.Fatalf("legacy environment = %v", row)
			}
		case created.ID:
			if row["level"] != "staging" || row["version_id"] != "release-a" || row["source"] != "platform" || row["status"] != "unknown" {
				t.Fatalf("new environment = %v", row)
			}
		default:
			if row["level"] != "" || row["version_id"] != "" {
				t.Fatalf("legacy creation defaults = %v", row)
			}
		}
	}
}
