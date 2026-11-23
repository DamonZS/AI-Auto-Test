package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"platform/server/internal/store"
)

func TestUIHealingApprovalCreatesRevision(t *testing.T) {
	database, _ := authFixture(t)
	handler := New(database, AuthOptions{}).Routes()
	for _, account := range []struct{ username, tenant, role string }{
		{"alice", "alpha", "owner"}, {"bob", "alpha", "editor"}, {"alice", "beta", "owner"},
	} {
		if err := ProvisionUser(t.Context(), database, account.username, fixturePassword, account.tenant, "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	owner, editor := authLogin(t, handler, "alice"), authLogin(t, handler, "bob")
	workerResponse := authRequest(t, handler, "POST", "/api/auth/service-tokens", "alpha", owner, `{"scope":"worker"}`, http.StatusCreated)
	var worker struct{ Token string }
	if err := json.Unmarshal(workerResponse.Body.Bytes(), &worker); err != nil || worker.Token == "" {
		t.Fatalf("invalid worker response: %v", err)
	}
	const scenario = `{"expected_result":"Dashboard loads","steps":[{"action":"navigate","url":"https://example.test"},{"action":"click","role":"button","name":"Old name","frame":"login","screenshot":true,"expected_result":"Signed in"},{"action":"assert_text","text":"Welcome"}]}`
	if _, err := database.DB.Exec(`INSERT INTO projects (id, tenant_id, name, base_url) VALUES ('project-a', 'alpha', 'Project', 'https://example.test');
		INSERT INTO cases (id, tenant_id, project_id, name, type, status, scenario, version_no) VALUES ('case-a', 'alpha', 'project-a', 'Sign in', 'ui', 'approved', ?, 3)`, scenario); err != nil {
		t.Fatal(err)
	}
	queue := func() string {
		var run struct {
			ID string `json:"run_id"`
		}
		response := authRequest(t, handler, "POST", "/api/cases/case-a/run", "alpha", editor, `{}`, http.StatusOK)
		if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil || run.ID == "" {
			t.Fatalf("invalid run response: %v", err)
		}
		response = authRequest(t, handler, "POST", "/api/runs/heartbeat", "beta", worker.Token, ``, http.StatusOK)
		if !strings.Contains(response.Body.String(), run.ID) {
			t.Fatalf("claimed the wrong run: %s", response.Body.String())
		}
		return "/api/runs/" + run.ID + "/finish"
	}
	const finish = `{"status":"passed","passed":true,"self_healed":true,"patch":{"step_seq":2,"old_selector":"Old name","new_selector":"New name"}}`
	finishURL := queue()
	authRequest(t, handler, "POST", finishURL, "alpha", owner, finish, http.StatusForbidden)
	for _, invalid := range []string{
		strings.Replace(finish, `"step_seq":2`, `"step_seq":3`, 1),
		strings.Replace(finish, `"step_seq":2`, `"step_seq":0`, 1),
		strings.Replace(finish, `"Old name"`, `"Wrong name"`, 1),
		strings.Replace(finish, `"New name"`, `"Old name"`, 1),
		strings.Replace(finish, `"self_healed":true`, `"self_healed":false`, 1),
		strings.Replace(finish, `"status":"passed"`, `"status":"failed"`, 1),
	} {
		authRequest(t, handler, "POST", finishURL, "beta", worker.Token, invalid, http.StatusBadRequest)
	}
	authRequest(t, handler, "POST", finishURL, "beta", worker.Token, finish, http.StatusOK)
	staleFinishURL := queue()
	settled := func(status string, version any) string {
		response := authRequest(t, handler, "GET", "/api/patches", "alpha", owner, "", http.StatusOK)
		var rows []map[string]any
		if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil {
			t.Fatal(err)
		}
		for _, row := range rows {
			if row["status"] == status && row["case_id"] == "case-a" && row["case_name"] == "Sign in" && row["role"] == "button" && row["applied_version_no"] == version {
				return row["id"].(string)
			}
		}
		t.Fatalf("patch state %s/%v missing: %v", status, version, rows)
		return ""
	}
	patchID := settled("pending", nil)
	const approve = `{"decision":"approve","reviewer":"forged"}`
	decideURL := "/api/patches/" + patchID + "/decide"
	authRequest(t, handler, "POST", decideURL, "alpha", editor, approve, http.StatusForbidden)
	if response := authRequest(t, handler, "GET", "/api/patches", "beta", owner, "", http.StatusOK); strings.TrimSpace(response.Body.String()) != `[]` {
		t.Fatalf("cross-tenant list: %s", response.Body.String())
	}
	authRequest(t, handler, "POST", decideURL, "beta", owner, approve, http.StatusConflict)
	response := authRequest(t, handler, "POST", decideURL, "alpha", owner, approve, http.StatusOK)
	if !strings.Contains(response.Body.String(), `"applied_version_no":4`) || settled("approved", float64(4)) != patchID {
		t.Fatalf("approval missing applied version: %s", response.Body.String())
	}
	var actor, actualScenario, status string
	var version int
	if err := database.DB.QueryRow(`SELECT actor FROM audit_logs WHERE tenant_id = 'alpha' AND action = 'patch.approve' AND target = ?`, patchID).Scan(&actor); err != nil || actor != "alice" {
		t.Fatalf("wrong audit actor: %q (%v)", actor, err)
	}
	if err := database.DB.QueryRow(`SELECT scenario, status, version_no FROM cases WHERE id = 'case-a'`).Scan(&actualScenario, &status, &version); err != nil || status != "approved" || version != 4 {
		t.Fatalf("unexpected case state: %s v%d (%v)", status, version, err)
	}
	var actual, expected any
	if err := json.Unmarshal([]byte(actualScenario), &actual); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(strings.Replace(scenario, "Old name", "New name", 1)), &expected); err != nil || mustJSON(actual) != mustJSON(expected) {
		t.Fatalf("approval altered other scenario fields: %s (%v)", actualScenario, err)
	}
	response = authRequest(t, handler, "GET", "/api/cases/case-a/history", "alpha", owner, "", http.StatusOK)
	var history []struct {
		Version  int `json:"version_no"`
		Scenario string
	}
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil || len(history) != 2 || history[0].Version != 3 || history[0].Scenario != scenario || history[1].Version != 4 || history[1].Scenario != actualScenario {
		t.Fatalf("missing case history: %s (%v)", response.Body.String(), err)
	}
	authRequest(t, handler, "POST", decideURL, "alpha", owner, approve, http.StatusConflict)
	authRequest(t, handler, "POST", staleFinishURL, "alpha", worker.Token, finish, http.StatusOK)
	staleID := settled("pending", nil)
	staleURL := "/api/patches/" + staleID + "/decide"
	if _, err := database.DB.Exec(`UPDATE cases SET scenario = ? WHERE id = 'case-a'`, scenario); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", staleURL, "alpha", owner, approve, http.StatusConflict)
	if settled("pending", nil) != staleID {
		t.Fatal("version conflict consumed the pending patch")
	}
	authRequest(t, handler, "POST", staleURL, "alpha", owner, `{"decision":"reject"}`, http.StatusOK)
	if settled("rejected", nil) != staleID {
		t.Fatal("rejected patch unexpectedly changed")
	}
	var current string
	var byCase int
	if err := database.DB.QueryRow(`SELECT scenario, version_no, (SELECT COUNT(*) FROM case_revisions WHERE tenant_id = 'alpha' AND case_id = 'case-a')
		FROM cases WHERE id = 'case-a'`).Scan(&current, &version, &byCase); err != nil || current != scenario || version != 4 || byCase != 2 {
		t.Fatalf("conflict or rejection changed the case: v%d, %d revisions (%v)", version, byCase, err)
	}

	for _, invalidScenario := range []string{
		"Sign in and check dashboard",
		strings.Replace(scenario, `"assert_text"`, `"unsupported"`, 1),
		strings.Replace(scenario, `,"frame":"login"`, `,"frame":12`, 1),
		strings.Replace(scenario, `,{"action":"assert_text","text":"Welcome"}`, "", 1),
	} {
		if _, err := database.DB.Exec(`UPDATE cases SET scenario = ? WHERE id = 'case-a'`, invalidScenario); err != nil {
			t.Fatal(err)
		}
		authRequest(t, handler, "POST", queue(), "alpha", worker.Token, finish, http.StatusBadRequest)
	}
	if _, err := database.DB.Exec(`UPDATE cases SET scenario = ?, type = 'api' WHERE id = 'case-a'`, scenario); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", queue(), "alpha", worker.Token, finish, http.StatusBadRequest)
	if _, err := database.DB.Exec(`UPDATE cases SET type = 'ui' WHERE id = 'case-a'`); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", queue(), "alpha", worker.Token, finish, http.StatusOK)
	mismatchedID := settled("pending", nil)
	if _, err := database.DB.Exec(`UPDATE cases SET scenario = ? WHERE id = 'case-a'`, actualScenario); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", "/api/patches/"+mismatchedID+"/decide", "alpha", owner, approve, http.StatusConflict)
	if settled("pending", nil) != mismatchedID {
		t.Fatal("name conflict consumed the pending patch")
	}
}

func TestUIHealingMigrationRetainsLegacyPatches(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, setupErr := database.DB.Exec(`ALTER TABLE heal_patches DROP COLUMN role;
		ALTER TABLE heal_patches DROP COLUMN applied_version_no;
		ALTER TABLE runs DROP COLUMN claimed_case_version_no;
		INSERT INTO tenants (id, name) VALUES ('alpha', 'Alpha');
		INSERT INTO cases (id, tenant_id, project_id, name, scenario) VALUES ('case-a', 'alpha', 'project-a', 'Legacy UI', '{}');
		INSERT INTO runs (id, tenant_id, case_id, status) VALUES ('run-a', 'alpha', 'case-a', 'passed');
		INSERT INTO heal_patches (id, tenant_id, run_id, step_seq, old_selector, new_selector) VALUES
		('pending-a', 'alpha', 'run-a', 1, 'Old name', 'New name'), ('approved-a', 'alpha', 'run-a', 1, 'Old name', 'New name');
		UPDATE heal_patches SET status = 'approved' WHERE id = 'approved-a'`)
	if closeErr := database.Close(); setupErr != nil || closeErr != nil {
		t.Fatalf("could not prepare legacy database: %v / %v", setupErr, closeErr)
	}
	database, err = store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if err := ProvisionUser(t.Context(), database, "alice", fixturePassword, "alpha", "", "owner"); err != nil {
		t.Fatal(err)
	}
	handler := New(database, AuthOptions{}).Routes()
	owner := authLogin(t, handler, "alice")
	authRequest(t, handler, "POST", "/api/patches/pending-a/decide", "alpha", owner, `{"decision":"approve"}`, http.StatusConflict)
	authRequest(t, handler, "POST", "/api/patches/pending-a/decide", "alpha", owner, `{"decision":"reject"}`, http.StatusOK)
	response := authRequest(t, handler, "GET", "/api/patches", "alpha", owner, "", http.StatusOK)
	var rows []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &rows); err != nil || len(rows) != 2 {
		t.Fatalf("migration lost legacy rows: %s (%v)", response.Body.String(), err)
	}
	for _, row := range rows {
		if row["applied_version_no"] != nil || row["role"] != nil || row["case_id"] != "case-a" || row["case_name"] != "Legacy UI" {
			t.Fatalf("unexpected legacy metadata: %v", row)
		}
		if (row["id"] == "pending-a" && row["status"] != "rejected") || (row["id"] == "approved-a" && row["status"] != "approved") {
			t.Fatalf("unexpected legacy decision: %v", row)
		}
	}
}
