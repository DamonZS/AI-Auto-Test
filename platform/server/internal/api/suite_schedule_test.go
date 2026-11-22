package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"
	"time"

	"platform/server/internal/store"
)

func TestWeeklySuiteScheduleQueuesOrderedMembersOnce(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec(`INSERT INTO cases (id, tenant_id, project_id, name, scenario, status, dataset_id, data_row_id)
		VALUES ('case-c', 'tenant-a', 'project-a', 'Case C', '{}', 'approved', 'dataset-c', 'row-c');
		UPDATE cases SET status = 'approved' WHERE id IN ('case-a', 'case-b');
		INSERT INTO suites (id, tenant_id, name, case_ids) VALUES
		('suite-a', 'tenant-a', 'Suite A', '["case-c","case-a"]'),
		('suite-b', 'tenant-b', 'Suite B', '["case-a"]')`); err != nil {
		t.Fatal(err)
	}
	server := New(st)
	handler := server.Routes()
	path := "/api/suites/suite-a/schedule"
	request := []byte(`{"enabled":true,"weekday":1,"hour":4,"minute":0}`)
	call := func(method, path, tenant string, body []byte, want int) string {
		t.Helper()
		response := serveTenant(handler, method, path, tenant, body)
		if response.Code != want {
			t.Fatalf("%s %s for %s: status = %d, want %d, body = %s", method, path, tenant, response.Code, want, response.Body.String())
		}
		return response.Body.String()
	}
	for _, method := range []string{http.MethodGet, http.MethodPut, http.MethodDelete} {
		call(method, path, "tenant-b", request, http.StatusNotFound)
	}
	call(http.MethodPut, path, "tenant-a", []byte(`{"weekday":7}`), http.StatusBadRequest)
	var schedule struct {
		SuiteID string `json:"suite_id"`
		NextRun string `json:"next_run_at"`
	}
	if err := json.Unmarshal([]byte(call(http.MethodPut, path, "tenant-a", request, http.StatusOK)), &schedule); err != nil || schedule.SuiteID != "suite-a" {
		t.Fatalf("saved schedule = %+v, err = %v", schedule, err)
	}
	if _, err := time.Parse(time.RFC3339, schedule.NextRun); err != nil {
		t.Fatal(err)
	}
	call(http.MethodGet, path, "tenant-a", nil, http.StatusOK)
	call(http.MethodPut, "/api/suites/suite-b/schedule", "tenant-b", request, http.StatusOK)
	due := time.Date(2026, 11, 23, 4, 0, 0, 0, time.Local)
	if queued := server.fireDueSchedules(due.Add(-time.Minute)); len(queued) != 0 {
		t.Fatalf("early tick queued %v", queued)
	}
	queued := server.fireDueSchedules(due)
	if len(queued) != 1 {
		t.Fatalf("due tick queued %v, want one suite batch", queued)
	}
	var summary struct {
		Runs []struct {
			CaseID string `json:"case_id"`
			Status string `json:"status"`
		} `json:"runs"`
	}
	summaryPath := "/api/suite-runs/" + queued[0]
	if err := json.Unmarshal([]byte(call(http.MethodGet, summaryPath, "tenant-a", nil, http.StatusOK)), &summary); err != nil {
		t.Fatal(err)
	}
	if len(summary.Runs) != 2 || summary.Runs[0].CaseID != "case-c" || summary.Runs[1].CaseID != "case-a" || summary.Runs[0].Status != "queued" || summary.Runs[1].Status != "queued" {
		t.Fatalf("unexpected child runs: %+v", summary.Runs)
	}
	var dataset, row string
	if err := st.DB.QueryRow(`SELECT dataset_id, data_row_id FROM runs WHERE suite_run_id = ? AND case_id = 'case-c'`, queued[0]).Scan(&dataset, &row); err != nil || dataset != "dataset-c" || row != "row-c" {
		t.Fatalf("child binding = %q/%q, err = %v", dataset, row, err)
	}
	call(http.MethodGet, summaryPath, "tenant-b", nil, http.StatusNotFound)
	call(http.MethodDelete, path, "tenant-a", nil, http.StatusOK)
	if got := call(http.MethodGet, path, "tenant-a", nil, http.StatusOK); got != "{\"schedule\":null}\n" {
		t.Fatalf("deleted schedule = %s", got)
	}
	call(http.MethodPut, path, "tenant-a", request, http.StatusOK)
	if duplicate := New(st).fireDueSchedules(due.Add(30 * time.Second)); len(duplicate) != 0 {
		t.Fatalf("same minute queued %v", duplicate)
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'draft' WHERE id = 'case-a'"); err != nil {
		t.Fatal(err)
	}
	if skipped := server.fireDueSchedules(due.AddDate(0, 0, 7)); len(skipped) != 0 {
		t.Fatalf("batch with draft member queued %v", skipped)
	}
	var count int
	if err := st.DB.QueryRow("SELECT count(*) FROM suite_runs").Scan(&count); err != nil || count != 1 {
		t.Fatalf("persisted suite runs = %d, err = %v", count, err)
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id = 'case-a'"); err != nil {
		t.Fatal(err)
	}
	if next := server.fireDueSchedules(due.AddDate(0, 0, 14)); len(next) != 1 || next[0] == queued[0] {
		t.Fatalf("next eligible week queued %v, want a new batch", next)
	}
	call(http.MethodPut, path, "tenant-a", []byte(`{"enabled":false,"weekday":1,"hour":4,"minute":0}`), http.StatusOK)
	if disabled := server.fireDueSchedules(due.AddDate(0, 0, 21)); len(disabled) != 0 {
		t.Fatalf("disabled schedule queued %v", disabled)
	}
}

func TestWeeklySuiteScheduleOpensExistingStore(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	seedMCPData(t, legacy)
	if _, err := legacy.DB.Exec(`DROP TABLE IF EXISTS suite_schedules;
		INSERT INTO suites (id, tenant_id, name) VALUES ('old-suite', 'tenant-a', 'Old Suite');
		INSERT INTO case_schedules (case_id, tenant_id, weekday, hour, minute) VALUES ('case-a', 'tenant-a', 1, 4, 0)`); err != nil {
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
	handler := New(st).Routes()
	response := serveTenant(handler, http.MethodGet, "/api/suites/old-suite/schedule", "tenant-a", nil)
	if response.Code != http.StatusOK || response.Body.String() != "{\"schedule\":null}\n" {
		t.Fatalf("old suite schedule = %d %s", response.Code, response.Body.String())
	}
	response = serveTenant(handler, http.MethodGet, "/api/cases/case-a/schedule", "tenant-a", nil)
	if response.Code != http.StatusOK || !json.Valid(response.Body.Bytes()) {
		t.Fatalf("old case schedule = %d %s", response.Code, response.Body.String())
	}
	var count int
	if err := st.DB.QueryRow("SELECT count(*) FROM case_schedules WHERE case_id = 'case-a'").Scan(&count); err != nil || count != 1 {
		t.Fatalf("old case schedules = %d, err = %v", count, err)
	}
}
