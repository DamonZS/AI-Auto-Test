package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestRunStepEvidenceIsPersistedAndTenantScoped(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	server := New(st)
	handler := server.Routes()
	finish := []byte(`{"status":"failed","steps":[{"step":1,"action":"navigate","status":"passed","duration_ms":85},{"step":2,"action":"assert_text","status":"failed","duration_ms":30}]}`)
	response := serveAgent(server, "/api/runs/run-a/finish", "tenant-b", finish)
	if response.Code != http.StatusNotFound {
		t.Fatalf("foreign worker finish returned %d", response.Code)
	}
	response = serveAgent(server, "/api/runs/run-a/finish", "tenant-a", finish)
	if response.Code != http.StatusOK {
		t.Fatalf("finish returned %d: %s", response.Code, response.Body)
	}
	response = serveTenant(handler, http.MethodGet, "/api/runs/run-a", "tenant-a", nil)
	var detail struct {
		Status string `json:"status"`
		Steps  []struct {
			Number   int    `json:"step"`
			Status   string `json:"status"`
			Duration int    `json:"duration_ms"`
		} `json:"steps"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &detail); err != nil {
		t.Fatal(err)
	}
	if detail.Status != "failed" || len(detail.Steps) != 2 || detail.Steps[0].Duration != 85 || detail.Steps[1].Number != 2 || detail.Steps[1].Status != "failed" {
		t.Fatalf("unexpected run detail: %+v", detail)
	}
	if response := serveTenant(handler, http.MethodGet, "/api/runs/run-a", "tenant-b", nil); response.Code != http.StatusNotFound {
		t.Fatalf("foreign tenant run lookup returned %d", response.Code)
	}
}

func TestSampledCaseMustBeApprovedBeforeDirectRun(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec("UPDATE cases SET generation_mode = 'sampling' WHERE id = ?", "case-a"); err != nil {
		t.Fatal(err)
	}
	handler := New(st).Routes()
	response := serveTenant(handler, http.MethodPost, "/api/cases/case-a/run", "tenant-a", []byte(`{}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("sampled draft run status = %d: %s", response.Code, response.Body)
	}
	if _, err := st.DB.Exec("UPDATE cases SET status = 'approved' WHERE id = ?", "case-a"); err != nil {
		t.Fatal(err)
	}
	response = serveTenant(handler, http.MethodPost, "/api/cases/case-a/run", "tenant-a", []byte(`{}`))
	if response.Code != http.StatusOK {
		t.Fatalf("approved sampled case run status = %d: %s", response.Code, response.Body)
	}
}

func TestRunUsesCaseTestEnvironment(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	_, err := st.DB.Exec(`UPDATE runs SET status = 'passed';
		INSERT INTO environments (id, tenant_id, project_id, name, base_url) VALUES
		('env-a', 'tenant-a', 'project-a', 'Staging', 'https://staging-a.test'),
		('env-b', 'tenant-b', 'project-b', 'Staging', 'https://staging-b.test')`)
	if err != nil {
		t.Fatal(err)
	}
	server := New(st)
	handler := server.Routes()
	response := serveTenant(handler, http.MethodPost, "/api/cases", "tenant-a", []byte(`{"project_id":"project-a","environment_id":"env-b","name":"case"}`))
	if response.Code != http.StatusBadRequest {
		t.Fatalf("foreign environment accepted: %d %s", response.Code, response.Body)
	}
	response = serveTenant(handler, http.MethodPost, "/api/cases", "tenant-a", []byte(`{"project_id":"project-a","environment_id":"env-a","name":"case"}`))
	var created struct{ ID string }
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || response.Code != http.StatusOK || created.ID == "" {
		t.Fatalf("create case: %d %s", response.Code, response.Body)
	}
	response = serveTenant(handler, http.MethodPost, "/api/cases/"+created.ID+"/run", "tenant-a", []byte(`{}`))
	if response.Code != http.StatusOK {
		t.Fatalf("queue case: %d %s", response.Code, response.Body)
	}
	response = serveAgent(server, "/api/runs/heartbeat", "tenant-a", nil)
	var payload struct {
		Run struct {
			BaseURL string `json:"base_url"`
		} `json:"run"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &payload); err != nil || payload.Run.BaseURL != "https://staging-a.test" {
		t.Fatalf("unexpected selected environment: %d %s", response.Code, response.Body)
	}
}
