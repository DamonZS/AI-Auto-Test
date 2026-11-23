package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestBugSeverityAndRetestPreserveWorkflow(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	_, err := database.DB.Exec(`UPDATE cases SET status = 'approved' WHERE id = 'case-a';
		UPDATE runs SET version_id = 'regression-version' WHERE id = 'run-a';
		INSERT INTO bugs (id, tenant_id, project_id, bug_code, title, status, run_id)
		VALUES ('defect-retest', 'tenant-a', 'project-a', 'BUG-A-1', 'Regression', 'fixed', 'run-a')`)
	if err != nil {
		t.Fatal(err)
	}
	handler := New(database).Routes()
	severity := serveTenant(handler, http.MethodPost, "/api/bugs/defect-retest/transition", "tenant-a", []byte(`{"to":"fixed","severity":"S2"}`))
	if severity.Code != http.StatusOK {
		t.Fatalf("change severity: %d %s", severity.Code, severity.Body)
	}
	foreign := serveTenant(handler, http.MethodPost, "/api/bugs/defect-retest/retest", "tenant-b", nil)
	if foreign.Code != http.StatusConflict {
		t.Fatalf("cross-tenant retest: %d %s", foreign.Code, foreign.Body)
	}
	response := serveTenant(handler, http.MethodPost, "/api/bugs/defect-retest/retest", "tenant-a", nil)
	var result struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusOK {
		t.Fatalf("retest: %d %s", response.Code, response.Body)
	}
	var caseID, versionID, runStatus, bugStatus, level string
	err = database.DB.QueryRow(`SELECT r.case_id, r.version_id, r.status, b.status, b.severity
		FROM runs r JOIN bugs b ON b.tenant_id = r.tenant_id
		WHERE r.id = ? AND b.id = 'defect-retest'`, result.RunID).Scan(&caseID, &versionID, &runStatus, &bugStatus, &level)
	if err != nil || caseID != "case-a" || versionID != "regression-version" || runStatus != "queued" || bugStatus != "fixed" || level != "S2" {
		t.Fatalf("unexpected retest state: %q %q %q %q %q (%v)", caseID, versionID, runStatus, bugStatus, level, err)
	}
}
