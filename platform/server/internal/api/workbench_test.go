package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"platform/server/internal/store"
)

const workbenchTestDocuments = "/api/workbench/documents"

func workbenchTestServer(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	st := openTestStore(t)
	seedMCPData(t, st)
	for range 2 {
		if err := store.MigrateWorkbench(st.DB); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := st.DB.Exec(`INSERT INTO versions (id, tenant_id, project_id, name) VALUES
		('v-a', 'tenant-a', 'project-a', 'A1'), ('v-a2', 'tenant-a', 'project-a', 'A2'),
		('v-b', 'tenant-b', 'project-b', 'B1')`); err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(st, AuthOptions{InsecureDevAuth: true}).registerWorkbench(mux)
	return st, mux
}

func workbenchRequest(t *testing.T, h http.Handler, method, path, tenant, payload string, want int) *httptest.ResponseRecorder {
	t.Helper()
	response := serveTenant(h, method, path, tenant, []byte(payload))
	if response.Code != want {
		t.Fatalf("%s %s: want %d, got %d: %s", method, path, want, response.Code, response.Body.String())
	}
	return response
}

func workbenchResponse[Result any](t *testing.T, response *httptest.ResponseRecorder) Result {
	t.Helper()
	var result Result
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestWorkbenchAuthenticatedReviewUsesSessionActor(t *testing.T) {
	st, _ := workbenchTestServer(t)
	if err := ProvisionUser(t.Context(), st, "alice", fixturePassword, "tenant-a", "", "editor"); err != nil {
		t.Fatal(err)
	}
	handler := New(st, AuthOptions{}).Routes()
	token := authLogin(t, handler, "alice")
	authRequest(t, handler, "GET", workbenchTestDocuments, "tenant-a", "", "", 401)
	authRequest(t, handler, "GET", workbenchTestDocuments, "tenant-b", token, "", 403)
	created := workbenchResponse[workbenchDocument](t, authRequest(t, handler, "POST", workbenchTestDocuments, "tenant-a", token,
		`{"kind":"peer_review","project_id":"project-a","version_id":"v-a","title":"同行评审","fields":{"subject":"执行计划","participants":"QA","review_date":"2026-11-23","conclusion":"通过","follow_up":"无"}}`, 201))
	path := workbenchTestDocuments + "/" + created.ID
	authRequest(t, handler, "POST", path+"/review", "tenant-a", token, `{"revision":1,"action":"submit"}`, 200)
	approved := workbenchResponse[workbenchDocument](t, authRequest(t, handler, "POST", path+"/review", "tenant-a", token,
		`{"revision":2,"action":"approve","reviewer":"impostor"}`, 200))
	if approved.Reviewer != "alice" || approved.Status != "approved" {
		t.Fatalf("unexpected approval: %#v", approved)
	}
	download := authRequest(t, handler, "GET", path+"/export", "tenant-a", token, "", 200)
	if html := download.Body.String(); !strings.Contains(html, "alice") || strings.Contains(html, "impostor") {
		t.Fatalf("export must render the authenticated reviewer: %s", html)
	}
	var auditCount int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE tenant_id = ? AND target = ? AND actor = ?`,
		"tenant-a", created.ID, "alice").Scan(&auditCount); err != nil || auditCount != 2 {
		t.Fatalf("authenticated review audit count = %d, err = %v", auditCount, err)
	}
}

func TestWorkbenchChecklistLifecycle(t *testing.T) {
	st, h := workbenchTestServer(t)
	templates := workbenchResponse[[]struct {
		Kind   string `json:"kind"`
		Checks []struct {
			Key string `json:"key"`
		} `json:"checks"`
	}](t, workbenchRequest(t, h, "GET", "/api/workbench/templates", "tenant-a", "", 200))
	var gotKinds []string
	for _, template := range templates {
		gotKinds = append(gotKinds, template.Kind)
	}
	if got := strings.Join(gotKinds, ","); got != "test_application,test_plan,entrance_checklist,peer_review,daily_report,exit_checklist,test_report" {
		t.Fatalf("template kinds = %s", got)
	}
	workbenchRequest(t, h, "POST", workbenchTestDocuments, "tenant-a", `{"kind":"test_plan","project_id":"project-a","version_id":"v-b","title":"Plan","fields":{}}`, 404)
	workbenchRequest(t, h, "POST", workbenchTestDocuments, "tenant-a", `{"kind":"entrance_checklist","project_id":"project-a","version_id":"v-a","title":"Gate","fields":{"unknown":"bad"}}`, 400)
	created := workbenchResponse[map[string]any](t, workbenchRequest(t, h, "POST", workbenchTestDocuments, "tenant-a",
		`{"kind":"entrance_checklist","project_id":"project-a","version_id":"v-a","title":"Gate","fields":{},"checks":{"smoke_passed":{"result":"fail","evidence":"环境不可用"}}}`, 201))
	path := workbenchTestDocuments + "/" + created["id"].(string)
	other := workbenchResponse[[]any](t, workbenchRequest(t, h, "GET", workbenchTestDocuments, "tenant-b", "", 200))
	if len(other) != 0 {
		t.Fatalf("other tenant list = %v", other)
	}
	workbenchRequest(t, h, "GET", path, "tenant-b", "", 404)
	workbenchRequest(t, h, "PATCH", path, "tenant-b", `{"revision":1,"title":"Gate","fields":{}}`, 404)
	workbenchRequest(t, h, "POST", path+"/review", "tenant-b", `{"revision":1,"action":"submit"}`, 404)
	workbenchRequest(t, h, "GET", path+"/export", "tenant-b", "", 404)
	workbenchRequest(t, h, "GET", path+"/export", "tenant-a", "", 409)
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":1,"action":"approve","reviewer":"QA"}`, 409)
	blocked := workbenchResponse[struct {
		Missing []string `json:"missing_sections"`
	}](t, workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":1,"action":"submit"}`, 422))
	if len(blocked.Missing) != 7 || blocked.Missing[6] != "smoke_passed" {
		t.Fatalf("admission missing sections = %v", blocked.Missing)
	}
	checks := map[string]any{}
	for _, check := range templates[2].Checks {
		checks[check.Key] = map[string]string{"result": "pass", "evidence": "已验收"}
	}
	edit := map[string]any{"revision": 1, "title": "Gate ready", "fields": map[string]string{}, "checks": checks}
	payload := mustJSON(edit)
	workbenchRequest(t, h, "PATCH", path, "tenant-a", payload, 200)
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":2,"action":"submit"}`, 200)
	workbenchRequest(t, h, "PATCH", path, "tenant-a", payload, 409)
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":3,"action":"reject","reviewer":"QA","note":"补冒烟证据"}`, 200)
	edit["revision"] = 4
	revised := workbenchResponse[map[string]any](t, workbenchRequest(t, h, "PATCH", path, "tenant-a", mustJSON(edit), 200))
	if revised["status"] != "draft" || revised["reviewer"] != "" || revised["review_note"] != "" {
		t.Fatalf("returned draft = %#v", revised)
	}
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":5,"action":"submit"}`, 200)
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":3,"action":"approve","reviewer":"QA"}`, 409)
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":6,"action":"approve","reviewer":"QA"}`, 200)
	workbenchRequest(t, h, "PATCH", path, "tenant-a", `{"revision":7,"title":"new","fields":{}}`, 409)
	download := workbenchRequest(t, h, "GET", path+"/export", "tenant-a", "", 200)
	if !strings.Contains(download.Header().Get("Content-Disposition"), "attachment;") ||
		!strings.Contains(download.Body.String(), "已验收") || !strings.Contains(download.Body.String(), "Gate ready") {
		t.Fatalf("checklist export incorrect: %s", download.Body.String())
	}
	var auditCount int
	if err := st.DB.QueryRow(`SELECT COUNT(*) FROM audit_logs WHERE tenant_id = ? AND target = ?`, "tenant-a", created["id"]).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("review audit count = %d, err = %v", auditCount, err)
	}
}

func TestWorkbenchSubmissionIsAtomicWithAudit(t *testing.T) {
	st, h := workbenchTestServer(t)
	created := workbenchResponse[map[string]any](t, workbenchRequest(t, h, "POST", workbenchTestDocuments, "tenant-a",
		`{"kind":"daily_report","project_id":"project-a","version_id":"v-a","title":"日报","fields":{"report_date":"2026-11-23","progress":"完成用例评审","blockers":"无","next_steps":"冒烟验证"}}`, 201))
	path := workbenchTestDocuments + "/" + created["id"].(string)
	if _, err := st.DB.Exec(`CREATE TRIGGER reject_workbench_audit BEFORE INSERT ON audit_logs
		BEGIN SELECT RAISE(ABORT, 'audit unavailable'); END;`); err != nil {
		t.Fatal(err)
	}
	const submit = `{"revision":1,"action":"submit"}`
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", submit, 500)
	doc := workbenchResponse[map[string]any](t, workbenchRequest(t, h, "GET", path, "tenant-a", "", 200))
	if doc["status"] != "draft" || doc["revision"] != float64(1) || doc["metrics"] != nil {
		t.Fatalf("audit failure left a partial transition: %#v", doc)
	}
	if _, err := st.DB.Exec(`DROP TRIGGER reject_workbench_audit`); err != nil {
		t.Fatal(err)
	}
	doc = workbenchResponse[map[string]any](t, workbenchRequest(t, h, "POST", path+"/review", "tenant-a", submit, 200))
	metrics := doc["metrics"].(map[string]any)
	if doc["status"] != "in_review" || metrics["execution_rate"] != float64(0) || metrics["pass_rate"] != float64(0) {
		t.Fatalf("submission retry did not freeze valid zero rates: %#v", doc)
	}
}

func TestWorkbenchReportFreezesTenantVersionMetrics(t *testing.T) {
	st, h := workbenchTestServer(t)
	if _, err := st.DB.Exec(`INSERT INTO requirements (id, tenant_id, project_id, code, title) VALUES
		('req-a', 'tenant-a', 'project-a', 'A', 'Covered'), ('req-uncovered', 'tenant-a', 'project-a', 'B', 'Uncovered');
		UPDATE cases SET status = 'approved', requirement_id = 'req-a' WHERE id = 'case-a';
		INSERT INTO cases (id, tenant_id, project_id, name, scenario, status) VALUES
		('case-c', 'tenant-a', 'project-a', 'C', '{}', 'approved');
		INSERT INTO runs (id, tenant_id, case_id, version_id, status, started_at, ended_at) VALUES
		('old-a', 'tenant-a', 'case-a', 'v-a', 'failed', '2026-01-01 10:00:00', '2026-01-01 10:10:00'),
		('passed-a', 'tenant-a', 'case-a', 'v-a', 'passed', '2026-01-01 11:00:00', '2026-01-01 11:10:00'),
		('running-a', 'tenant-a', 'case-a', 'v-a', 'running', '2026-01-01 11:30:00', NULL),
		('other-version', 'tenant-a', 'case-c', 'v-a2', 'passed', '2026-01-02 10:00:00', '2026-01-02 10:10:00'),
		('other-tenant', 'tenant-b', 'case-b', 'v-a', 'failed', '2026-01-03 10:00:00', '2026-01-03 10:10:00');
		INSERT INTO bugs (id, tenant_id, project_id, bug_code, title, priority, severity, status, run_id) VALUES
		('bug-a', 'tenant-a', 'project-a', 'BUG-A', 'OPEN DEFECT', ' p1 ', 'S2', 'new', 'old-a'),
		('bug-a2', 'tenant-a', 'project-a', 'BUG-A2', 'OTHER VERSION', 'P0', 'S1', 'new', 'other-version'),
		('bug-b', 'tenant-b', 'project-b', 'BUG-B', 'OTHER TENANT', 'P0', 'S1', 'new', 'other-tenant')`); err != nil {
		t.Fatal(err)
	}
	created := workbenchResponse[map[string]any](t, workbenchRequest(t, h, "POST", workbenchTestDocuments, "tenant-a",
		`{"kind":"test_report","project_id":"project-a","version_id":"v-a","title":"<script>report</script>","risk_level":"P3","fields":{"test_scope_environment":"支付，预发布","residual_risks":"修复待确认","conclusion":"有条件通过"}}`, 201))
	path := workbenchTestDocuments + "/" + created["id"].(string)
	submitted := workbenchResponse[struct {
		Metrics struct {
			Cases       map[string]int `json:"case_statistics"`
			Coverage    map[string]int `json:"requirement_coverage"`
			Defects     map[string]int `json:"defect_distribution"`
			Severities  map[string]int `json:"severity_distribution"`
			Execution   float64        `json:"execution_rate"`
			Pass        float64        `json:"pass_rate"`
			DefectTotal int            `json:"defect_total"`
			Started     string         `json:"started_at"`
			Ended       string         `json:"ended_at"`
			NeedsReview bool           `json:"requires_human_review"`
		} `json:"metrics"`
	}](t, workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":1,"action":"submit"}`, 200))
	metrics := submitted.Metrics
	if metrics.Cases["total"] != 2 || metrics.Cases["executed"] != 1 || metrics.Cases["passed"] != 1 ||
		metrics.Coverage["total"] != 2 || metrics.Coverage["covered"] != 1 || metrics.Execution != 0.5 || metrics.Pass != 1 ||
		metrics.DefectTotal != 1 || len(metrics.Defects) != 1 || metrics.Defects["P1"] != 1 || metrics.Severities["S2"] != 1 || !metrics.NeedsReview ||
		metrics.Started != "2026-01-01 10:00:00" || metrics.Ended != "2026-01-01 11:10:00" {
		t.Fatalf("report metrics = %#v", metrics)
	}
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":2,"action":"approve"}`, 400)
	if _, err := st.DB.Exec(`UPDATE runs SET status = 'failed' WHERE id = 'passed-a'; UPDATE bugs SET status = 'closed' WHERE id = 'bug-a'`); err != nil {
		t.Fatal(err)
	}
	workbenchRequest(t, h, "POST", path+"/review", "tenant-a", `{"revision":2,"action":"approve","reviewer":"QA","note":"核准"}`, 200)
	approved := workbenchResponse[map[string]any](t, workbenchRequest(t, h, "GET", path, "tenant-a", "", 200))
	if approved["status"] != "approved" || approved["reviewer"] != "QA" || approved["reviewed_at"] == "" {
		t.Fatalf("approved document = %#v", approved)
	}
	download := workbenchRequest(t, h, "GET", path+"/export", "tenant-a", "", 200)
	html := download.Body.String()
	if !strings.Contains(html, "&lt;script&gt;report&lt;/script&gt;") || strings.Contains(html, "<script>") ||
		!strings.Contains(html, "100.0%") || !strings.Contains(html, "OPEN DEFECT") || strings.Contains(html, "OTHER VERSION") {
		t.Fatalf("unexpected exported snapshot: %s", html)
	}
}
