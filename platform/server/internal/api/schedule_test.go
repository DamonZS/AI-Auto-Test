package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"testing"
	"time"
)

func TestWeeklyScheduleCreatesRunOnlyAtDueMinute(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id = ? AND tenant_id = ?", "case-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	s := New(st)
	h := s.Routes()
	response := serveTenant(h, http.MethodPut, "/api/cases/case-a/schedule", "tenant-a", []byte(`{"enabled":true,"weekday":1,"hour":4,"minute":0}`))
	if response.Code != http.StatusOK {
		t.Fatalf("save schedule status = %d, body = %s", response.Code, response.Body.String())
	}
	var schedule map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &schedule); err != nil {
		t.Fatal(err)
	}
	if schedule["case_id"] != "case-a" || schedule["weekday"].(float64) != 1 {
		t.Fatalf("unexpected schedule: %#v", schedule)
	}
	var before int
	if err := st.DB.QueryRow("SELECT count(*) FROM runs WHERE tenant_id = ? AND case_id = ?", "tenant-a", "case-a").Scan(&before); err != nil {
		t.Fatal(err)
	}

	got := s.fireDueSchedules(time.Date(2026, 11, 23, 4, 0, 0, 0, time.Local))
	if len(got) != 1 {
		t.Fatalf("due schedule created %d runs, want 1", len(got))
	}
	got = s.fireDueSchedules(time.Date(2026, 11, 23, 4, 0, 30, 0, time.Local))
	if len(got) != 0 {
		t.Fatalf("same minute created duplicate runs: %#v", got)
	}

	var count int
	if err := st.DB.QueryRow("SELECT count(*) FROM runs WHERE tenant_id = ? AND case_id = ?", "tenant-a", "case-a").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != before+1 {
		t.Fatalf("runs count = %d, want %d", count, before+1)
	}
}

func TestWeeklyScheduleRequiresApprovedCaseAndStopsAfterRevision(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	s := New(st)
	h := s.Routes()
	request := []byte(`{"enabled":true,"weekday":1,"hour":4,"minute":0}`)
	response := serveTenant(h, http.MethodPut, "/api/cases/case-a/schedule", "tenant-a", request)
	if response.Code != http.StatusConflict {
		t.Fatalf("draft schedule status = %d, want %d", response.Code, http.StatusConflict)
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id = ? AND tenant_id = ?", "case-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	response = serveTenant(h, http.MethodPut, "/api/cases/case-a/schedule", "tenant-a", request)
	if response.Code != http.StatusOK {
		t.Fatalf("approved schedule status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'draft' WHERE id = ? AND tenant_id = ?", "case-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	got := s.fireDueSchedules(time.Date(2026, 11, 23, 4, 0, 0, 0, time.Local))
	if len(got) != 0 {
		t.Fatalf("draft case schedule created runs: %#v", got)
	}
}

func TestWeeklyScheduleIsTenantScopedAndRejectsInvalidTime(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()
	response := serveTenant(h, http.MethodPut, "/api/cases/case-a/schedule", "tenant-b", []byte(`{"enabled":true,"weekday":1,"hour":4,"minute":0}`))
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant schedule status = %d, want 404", response.Code)
	}
	response = serveTenant(h, http.MethodPut, "/api/cases/case-a/schedule", "tenant-a", []byte(`{"enabled":true,"weekday":7,"hour":24,"minute":60}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid schedule status = %d, want 400", response.Code)
	}
}

func TestWeeklyScheduleDelete(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id = ? AND tenant_id = ?", "case-a", "tenant-a"); err != nil {
		t.Fatal(err)
	}
	h := New(st).Routes()
	response := serveTenant(h, http.MethodPut, "/api/cases/case-a/schedule", "tenant-a", bytes.NewBufferString(`{"enabled":true,"weekday":1,"hour":4,"minute":0}`).Bytes())
	if response.Code != http.StatusOK {
		t.Fatalf("save schedule status = %d", response.Code)
	}
	response = serveTenant(h, http.MethodDelete, "/api/cases/case-a/schedule", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("delete schedule status = %d", response.Code)
	}
	response = serveTenant(h, http.MethodGet, "/api/cases/case-a/schedule", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get schedule status = %d", response.Code)
	}
	var body map[string]any
	_ = json.Unmarshal(response.Body.Bytes(), &body)
	if body["schedule"] != nil {
		t.Fatalf("schedule still present: %#v", body)
	}
}
