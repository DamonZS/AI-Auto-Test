package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"platform/server/internal/store"
)

func TestSuiteRunColumnMigratesExistingStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.DB.Exec(`ALTER TABLE runs DROP COLUMN suite_run_id;
		DROP TABLE suite_runs;
		ALTER TABLE suites DROP COLUMN case_ids;
		INSERT INTO suites (id, tenant_id, name) VALUES ('old-suite', 'tenant-a', 'Old Suite');
		INSERT INTO tenants (id, name) VALUES ('tenant-a', 'Tenant A');
		INSERT INTO runs (id, tenant_id, case_id, version_id) VALUES ('old-run', 'tenant-a', 'case-a', 'version-a')`); err != nil {
		legacy.Close()
		t.Fatal(err)
	}
	if err := legacy.Close(); err != nil {
		t.Fatal(err)
	}
	st, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	var versionID, suiteRunID string
	if err := st.DB.QueryRow("SELECT version_id, suite_run_id FROM runs WHERE id = 'old-run'").Scan(&versionID, &suiteRunID); err != nil {
		t.Fatal(err)
	}
	if versionID != "version-a" || suiteRunID != "" {
		t.Fatalf("migrated run version_id = %q, suite_run_id = %q", versionID, suiteRunID)
	}
	if response := serveTenant(New(st).Routes(), http.MethodGet, "/api/suites", "tenant-a", nil); response.Code != http.StatusOK || !bytes.Contains(response.Body.Bytes(), []byte(`"case_ids":[]`)) {
		t.Fatalf("list migrated suites status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestSuiteCreateListAndRunSummariesAreTenantScoped(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec("INSERT INTO cases (id, tenant_id, project_id, name, scenario) VALUES (?, ?, ?, ?, ?)", "case-c", "tenant-a", "project-a", "Case C", "{}"); err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id IN ('case-a', 'case-c')"); err != nil {
		t.Fatal(err)
	}
	h := New(st).Routes()

	response := serveTenant(h, http.MethodPost, "/api/suites", "tenant-a", []byte(`{"name":"smoke"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("create suite status = %d, body = %s", response.Code, response.Body.String())
	}
	var created struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	if created.ID == "" || created.Name != "smoke" {
		t.Fatalf("unexpected created suite: %#v", created)
	}

	response = serveTenant(h, http.MethodPost, "/api/suites", "tenant-b", []byte(`{"name":"other"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("create tenant-b suite status = %d, body = %s", response.Code, response.Body.String())
	}

	response = serveTenant(h, http.MethodGet, "/api/suites", "tenant-a", nil)
	var suites []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &suites); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || len(suites) != 1 || suites[0]["name"] != "smoke" {
		t.Fatalf("tenant-a suites = status %d, %#v", response.Code, suites)
	}

	response = serveTenant(h, http.MethodPost, "/api/suites/"+created.ID+"/run", "tenant-a", []byte(`{"case_ids":["case-c","case-a"]}`))
	if response.Code != http.StatusOK {
		t.Fatalf("create suite run status = %d, body = %s", response.Code, response.Body.String())
	}
	var started struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &started); err != nil {
		t.Fatal(err)
	}
	response = serveTenant(New(st).Routes(), http.MethodGet, "/api/suites", "tenant-a", nil)
	if err := json.Unmarshal(response.Body.Bytes(), &suites); err != nil || response.Code != http.StatusOK {
		t.Fatalf("read suite members: %d %s", response.Code, response.Body.String())
	}
	if members, _ := json.Marshal(suites[0]["case_ids"]); string(members) != `["case-c","case-a"]` {
		t.Fatalf("suite members = %s", members)
	}
	historyPath := "/api/suites/" + created.ID + "/runs"
	assertHistory := func(status string) {
		t.Helper()
		response := serveTenant(h, http.MethodGet, historyPath, "tenant-a", nil)
		var batches []map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &batches); err != nil || response.Code != http.StatusOK || len(batches) != 1 || batches[0]["id"] != started.ID || batches[0]["status"] != status {
			t.Fatalf("expected batch %s with status %s: %d %s", started.ID, status, response.Code, response.Body.String())
		}
	}
	assertHistory("queued")

	if _, err := st.DB.Exec("UPDATE runs SET status = 'passed', version_id = ? WHERE id = 'run-a'", started.ID); err != nil {
		t.Fatal(err)
	}
	postAgent := func(path, tenant, body string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
		request.Header.Set("X-Agent-Token", "tenant:"+tenant)
		recorder := httptest.NewRecorder()
		h.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s status = %d, body = %s", path, recorder.Code, recorder.Body.String())
		}
		return recorder
	}
	for index, caseID := range []string{"case-c", "case-a"} {
		claimed := postAgent("/api/runs/heartbeat", "tenant-a", "{}")
		assertHistory("running")
		var payload struct {
			Run struct {
				ID        string `json:"run_id"`
				CaseID    string `json:"case_id"`
				VersionID string `json:"version_id"`
			} `json:"run"`
		}
		if err := json.Unmarshal(claimed.Body.Bytes(), &payload); err != nil {
			t.Fatal(err)
		}
		if payload.Run.ID == "" || payload.Run.CaseID != caseID || payload.Run.VersionID != "" {
			t.Fatalf("unexpected claim for %s: %#v", caseID, payload.Run)
		}
		result := `{"status":"passed","passed":true}`
		if index == 1 {
			result = `{"status":"failed","needs_review":true}`
		}
		postAgent("/api/runs/"+payload.Run.ID+"/finish", "tenant-a", result)
	}
	assertHistory("failed")
	if response := serveTenant(h, http.MethodGet, historyPath, "tenant-b", nil); response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant history status = %d, want 404", response.Code)
	}
	if remaining := postAgent("/api/runs/heartbeat", "tenant-a", "{}"); remaining.Body.String() != "{\"run\":null}\n" {
		t.Fatalf("tenant-a queue should be empty: %s", remaining.Body.String())
	}

	response = serveTenant(h, http.MethodGet, "/api/suite-runs/"+started.ID, "tenant-a", nil)
	var summary map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil {
		t.Fatal(err)
	}
	if response.Code != http.StatusOK || summary["total"] != float64(2) || summary["passed"] != float64(1) || summary["failed"] != float64(1) || summary["needs_review"] != float64(1) {
		t.Fatalf("suite run summary = status %d, %#v", response.Code, summary)
	}
	caseRuns := summary["runs"].([]any)
	if caseRuns[0].(map[string]any)["case_id"] != "case-c" || caseRuns[1].(map[string]any)["case_id"] != "case-a" {
		t.Fatalf("suite run lost case order: %#v", caseRuns)
	}

	response = serveTenant(h, http.MethodGet, "/api/suite-runs/"+started.ID, "tenant-b", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant suite run status = %d, want 404", response.Code)
	}

	response = serveTenant(h, http.MethodPost, "/api/suites/"+created.ID+"/run", "tenant-a", []byte(`{"case_ids":["case-a"]}`))
	var second struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &second); err != nil || response.Code != http.StatusOK || second.ID == "" || second.ID == started.ID {
		t.Fatalf("second batch: %d %s", response.Code, response.Body.String())
	}
	claimed := postAgent("/api/runs/heartbeat", "tenant-a", "{}")
	var secondClaim struct {
		Run struct {
			ID string `json:"run_id"`
		} `json:"run"`
	}
	if err := json.Unmarshal(claimed.Body.Bytes(), &secondClaim); err != nil || secondClaim.Run.ID == "" {
		t.Fatalf("second claim: %s", claimed.Body.String())
	}
	postAgent("/api/runs/"+secondClaim.Run.ID+"/finish", "tenant-a", `{"status":"needs_review","needs_review":true}`)
	response = serveTenant(h, http.MethodGet, "/api/suite-runs/"+second.ID, "tenant-a", nil)
	if err := json.Unmarshal(response.Body.Bytes(), &summary); err != nil || response.Code != http.StatusOK || summary["failed"] != float64(0) || summary["needs_review"] != float64(1) {
		t.Fatalf("review batch summary: %d %s", response.Code, response.Body.String())
	}
	response = serveTenant(h, http.MethodGet, historyPath, "tenant-a", nil)
	want := `[{"id":"` + second.ID + `","status":"needs_review"},{"id":"` + started.ID + `","status":"failed"}]` + "\n"
	if response.Code != http.StatusOK || response.Body.String() != want {
		t.Fatalf("batch history: %d %s", response.Code, response.Body.String())
	}
}

func TestSuiteRunRejectsForeignCaseWithoutCreatingPartialRuns(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()
	response := serveTenant(h, http.MethodPost, "/api/suites", "tenant-a", []byte(`{"name":"smoke"}`))
	var suite struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &suite); err != nil {
		t.Fatal(err)
	}
	response = serveTenant(h, http.MethodPost, "/api/suites/"+suite.ID+"/run", "tenant-a", []byte(`{"case_ids":["case-a"]}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("draft-case suite run status = %d, want 409", response.Code)
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id = 'case-a'"); err != nil {
		t.Fatal(err)
	}
	response = serveTenant(h, http.MethodPost, "/api/suites/"+suite.ID+"/run", "tenant-b", []byte(`{"case_ids":["case-b"]}`))
	if response.Code != http.StatusNotFound {
		t.Fatalf("foreign suite run status = %d, want 404", response.Code)
	}

	response = serveTenant(h, http.MethodPost, "/api/suites/"+suite.ID+"/run", "tenant-a", []byte(`{"case_ids":["case-a","case-b"]}`))
	if response.Code != http.StatusNotFound {
		t.Fatalf("foreign-case suite run status = %d, want 404, body = %s", response.Code, response.Body.String())
	}
	var count int
	if err := st.DB.QueryRow("SELECT count(*) FROM runs WHERE tenant_id = ?", "tenant-a").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("foreign-case request partially created runs, count = %d", count)
	}
}
