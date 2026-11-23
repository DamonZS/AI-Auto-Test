package api

import (
	"encoding/json"
	"net/http"
	"testing"

	"platform/server/internal/store"
)

func requirementFixture(t *testing.T) (*store.Store, http.Handler, string, string, string) {
	t.Helper()
	database := openTestStore(t)
	seedMCPData(t, database)
	if _, err := database.DB.Exec(`
		DROP TRIGGER IF EXISTS requirements_initial_revision;
		INSERT INTO projects (id, tenant_id, name, base_url) VALUES ('other-a', 'tenant-a', 'Other', 'https://other.test');
		INSERT INTO requirements (id, tenant_id, project_id, code, title, content)
		VALUES ('legacy-req', 'tenant-a', 'project-a', 'REQ-1', '原始标题', '原始内容');
		INSERT INTO versions (id, tenant_id, project_id, name) VALUES
		('version-a', 'tenant-a', 'project-a', '1.0'), ('version-empty', 'tenant-a', 'project-a', '2.0'),
		('version-other', 'tenant-a', 'other-a', 'Other'), ('version-b', 'tenant-b', 'project-b', '1.0');
		UPDATE cases SET requirement_id = 'legacy-req' WHERE id IN ('case-a', 'case-b');
		INSERT INTO cases (id, tenant_id, project_id, requirement_id, name, scenario)
		VALUES ('case-other', 'tenant-a', 'other-a', 'legacy-req', 'Out of scope', '{}');
	`); err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err := store.MigrateRequirements(database.DB); err != nil {
			t.Fatal(err)
		}
	}
	for _, account := range []struct{ name, tenant, role string }{
		{"requirements-editor", "tenant-a", "editor"}, {"requirements-viewer", "tenant-a", "viewer"}, {"requirements-other", "tenant-b", "owner"},
	} {
		if err := ProvisionUser(t.Context(), database, account.name, fixturePassword, account.tenant, "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(database, AuthOptions{}).Routes()
	return database, handler, authLogin(t, handler, "requirements-editor"), authLogin(t, handler, "requirements-viewer"), authLogin(t, handler, "requirements-other")
}

func TestRequirementsImportAndLegacyCreation(t *testing.T) {
	database, handler, editor, viewer, _ := requirementFixture(t)
	const importPath = "/api/requirements/import"
	csv := mustJSON(map[string]string{
		"project_id": "project-a", "filename": "fixture.csv", "text": "\ufeffcode,title,content\r\nREQ-2,登录,\"第一行,带逗号\r\n第二行\"\r\n,退出,\r\n",
	})
	authRequest(t, handler, "POST", importPath, "tenant-a", "", csv, http.StatusUnauthorized)
	authRequest(t, handler, "POST", importPath, "tenant-a", viewer, csv, http.StatusForbidden)
	response := authRequest(t, handler, "POST", importPath, "tenant-a", editor, csv, http.StatusCreated)
	var created struct{ IDs []string }
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || len(created.IDs) != 2 {
		t.Fatalf("import response = %s (%v)", response.Body, err)
	}
	response = authRequest(t, handler, "GET", "/api/requirements/"+created.IDs[0]+"/history", "tenant-a", viewer, "", http.StatusOK)
	var history []struct {
		Revision int `json:"revision_no"`
		Content  string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil || len(history) != 1 || history[0].Revision != 1 || history[0].Content != "第一行,带逗号\n第二行" {
		t.Fatalf("import history = %s (%v)", response.Body, err)
	}
	for _, payload := range []string{
		`{"project_id":"project-a","filename":"支付.md","text":"# 支付\n\n原文"}`,
		`{"project_id":"project-a","filename":"验收.txt","text":"验收步骤","code":"REQ-3","title":"验收测试"}`,
	} {
		authRequest(t, handler, "POST", importPath, "tenant-a", editor, payload, http.StatusCreated)
	}
	authRequest(t, handler, "POST", importPath, "tenant-a", editor, `{"project_id":"project-a","filename":"bad.csv","text":"code,title,content\nGOOD,合法,内容\nBAD,,内容"}`, http.StatusBadRequest)
	authRequest(t, handler, "POST", importPath, "tenant-a", editor, `{"project_id":"project-b","filename":"trial.txt","text":"越界"}`, http.StatusNotFound)
	authRequest(t, handler, "POST", importPath, "tenant-a", editor, `{"project_id":"project-a","filename":"trial.txt","text":"`+string([]byte{0xff})+`"}`, http.StatusBadRequest)
	response = authRequest(t, handler, "POST", "/api/requirements", "tenant-a", editor, `{"project_id":"project-a","title":"手工创建"}`, http.StatusOK)
	var manual struct{ ID string }
	if err := json.Unmarshal(response.Body.Bytes(), &manual); err != nil || manual.ID == "" {
		t.Fatalf("manual response = %s (%v)", response.Body, err)
	}
	authRequest(t, handler, "GET", "/api/requirements/"+manual.ID+"/history", "tenant-a", editor, "", http.StatusOK)
	authRequest(t, handler, "POST", "/api/requirements", "tenant-a", editor, `{"project_id":"project-b","title":"越界"}`, http.StatusInternalServerError)
	var count int
	if err := database.DB.QueryRow(`SELECT count(*) FROM requirements WHERE tenant_id = 'tenant-a'`).Scan(&count); err != nil || count != 6 {
		t.Fatalf("requirements count = %d (%v)", count, err)
	}
	if err := database.DB.QueryRow(`SELECT count(*) FROM requirements WHERE (title = '支付' AND content = '# 支付' || char(10,10) || '原文') OR (title = '验收测试' AND code = 'REQ-3' AND content = '验收步骤')`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("text import count = %d (%v)", count, err)
	}
}

func TestRequirementsRevisionHistoryAndVersionImpact(t *testing.T) {
	_, handler, editor, viewer, other := requirementFixture(t)
	const requirement = "/api/requirements/legacy-req"
	const impact = "/api/versions/version-a/requirement-impact"
	const link = requirement + "/revisions/2/versions/"
	authRequest(t, handler, "PATCH", requirement, "tenant-a", viewer, `{"content":"不允许"}`, http.StatusForbidden)
	authRequest(t, handler, "PATCH", requirement, "tenant-b", other, `{"content":"不允许"}`, http.StatusNotFound)
	authRequest(t, handler, "PATCH", requirement, "tenant-a", editor, `{"content":"第二版"}`, http.StatusOK)
	authRequest(t, handler, "PUT", link+"version-a", "tenant-a", viewer, "", http.StatusForbidden)
	authRequest(t, handler, "PUT", link+"version-a", "tenant-a", editor, "", http.StatusNoContent)
	authRequest(t, handler, "PUT", link+"version-a", "tenant-a", editor, "", http.StatusNoContent)
	authRequest(t, handler, "PUT", link+"version-other", "tenant-a", editor, "", http.StatusNotFound)
	authRequest(t, handler, "PUT", link+"version-b", "tenant-a", editor, "", http.StatusNotFound)
	authRequest(t, handler, "PUT", requirement+"/revisions/1/versions/version-a", "tenant-a", editor, "", http.StatusNoContent)
	authRequest(t, handler, "PATCH", requirement, "tenant-a", editor, `{"content":"第三版"}`, http.StatusOK)
	authRequest(t, handler, "GET", requirement+"/history", "tenant-b", other, "", http.StatusNotFound)
	authRequest(t, handler, "GET", impact, "tenant-b", other, "", http.StatusNotFound)
	empty := authRequest(t, handler, "GET", "/api/versions/version-empty/requirement-impact", "tenant-a", viewer, "", http.StatusOK)
	if empty.Body.String() != "{\"affected_case_count\":0,\"changes\":[]}\n" {
		t.Fatalf("empty impact = %s", empty.Body)
	}
	response := authRequest(t, handler, "GET", impact, "tenant-a", viewer, "", http.StatusOK)
	type snapshot struct {
		Revision             int `json:"revision_no"`
		Code, Title, Content string
	}
	var report struct {
		Count   int `json:"affected_case_count"`
		Changes []struct {
			RequirementID string `json:"requirement_id"`
			Count         int    `json:"affected_case_count"`
			Previous      *snapshot
			Current       snapshot
		}
	}
	if err := json.Unmarshal(response.Body.Bytes(), &report); err != nil || report.Count != 1 || len(report.Changes) != 2 {
		t.Fatalf("version impact = %s (%v)", response.Body, err)
	}
	first, second := report.Changes[0], report.Changes[1]
	if first.Previous != nil || first.Current.Content != "原始内容" || second.RequirementID != "legacy-req" || second.Count != 1 || second.Previous == nil || second.Previous.Content != "原始内容" || second.Current.Revision != 2 || second.Current.Content != "第二版" || second.Current.Title != "原始标题" || second.Current.Code != "REQ-1" {
		t.Fatalf("impact history = %s", response.Body)
	}
	response = authRequest(t, handler, "GET", requirement+"/history", "tenant-a", viewer, "", http.StatusOK)
	var history []snapshot
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil || len(history) != 3 || history[0].Content != "原始内容" || history[2].Content != "第三版" {
		t.Fatalf("migrated history = %s (%v)", response.Body, err)
	}
}
