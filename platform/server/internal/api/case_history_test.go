package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestCaseScenarioRevisionIsAvailableOnlyToItsTenant(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()

	response := serveTenant(h, http.MethodPost, "/api/cases", "tenant-a", []byte(`{"project_id":"project-a","name":"history case","scenario":"original scenario"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("create case status = %d, body = %s", response.Code, response.Body.String())
	}
	var created map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	cid := created["id"]

	response = serveTenant(h, http.MethodPost, "/api/cases/"+cid+"/review", "tenant-a", []byte(`{"decision":"revise","reviewer":"reviewer-a","scenario":"revised scenario"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("revise case status = %d, body = %s", response.Code, response.Body.String())
	}

	response = serveTenant(h, http.MethodGet, "/api/cases/"+cid+"/history", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list case history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0]["version_no"] != float64(1) || history[0]["scenario"] != "original scenario" || history[1]["version_no"] != float64(2) || history[1]["scenario"] != "revised scenario" {
		t.Fatalf("unexpected case history: %#v", history)
	}

	response = serveTenant(h, http.MethodGet, "/api/cases/"+cid+"/history", "tenant-b", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant case history status = %d, want 404", response.Code)
	}
}

func TestCaseReviewWithoutContentChangeDoesNotCreateRevision(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()

	response := serveTenant(h, http.MethodPost, "/api/cases/case-a/review", "tenant-a", []byte(`{"decision":"submit_review"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("submit review status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serveTenant(h, http.MethodGet, "/api/cases/case-a/history", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list case history status = %d, body = %s", response.Code, response.Body.String())
	}
	if response.Code != http.StatusOK {
		t.Fatalf("list case history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 {
		t.Fatalf("workflow status change created content revision: %#v", history)
	}
}

func TestCreateCaseStoresInitialContentRevision(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()
	response := serveTenant(h, http.MethodPost, "/api/cases", "tenant-a", []byte(`{"project_id":"project-a","name":"new case","scenario":"first scenario"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("create case status = %d, body = %s", response.Code, response.Body.String())
	}
	var created map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	response = serveTenant(h, http.MethodGet, "/api/cases/"+created["id"]+"/history", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list case history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 1 || history[0]["version_no"] != float64(1) || history[0]["scenario"] != "first scenario" {
		t.Fatalf("unexpected initial case revision: %#v", history)
	}
}

func TestDraftCaseScenarioUpdatePreservesLegacyScenarioRevision(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()
	response := serveTenant(h, http.MethodPatch, "/api/cases/case-a", "tenant-a", []byte(`{"scenario":"legacy updated"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("update legacy case status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serveTenant(h, http.MethodGet, "/api/cases/case-a/history", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list legacy case history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 || history[0]["version_no"] != float64(1) || history[0]["scenario"] != "{}" || history[1]["version_no"] != float64(2) || history[1]["scenario"] != "legacy updated" {
		t.Fatalf("unexpected legacy case history: %#v", history)
	}
}

func TestDraftCaseScenarioUpdateCreatesTenantScopedRevision(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()
	scenario := `{"steps":[{"action":"wait_text","text":"Ready"}]}`
	body, err := json.Marshal(map[string]string{"scenario": scenario})
	if err != nil {
		t.Fatal(err)
	}
	response := serveTenant(h, http.MethodPatch, "/api/cases/case-a", "tenant-a", body)
	if response.Code != http.StatusOK {
		t.Fatalf("update case status = %d, body = %s", response.Code, response.Body.String())
	}
	var storedVersion int
	var storedScenario string
	if err := st.DB.QueryRow("SELECT version_no, scenario FROM case_revisions WHERE tenant_id = ? AND case_id = ? ORDER BY version_no DESC LIMIT 1", "tenant-a", "case-a").Scan(&storedVersion, &storedScenario); err != nil {
		t.Fatal(err)
	}
	if storedVersion != 2 || storedScenario != scenario {
		t.Fatalf("stored revision = (%d, %q), want version 2 with updated scenario", storedVersion, storedScenario)
	}
	response = serveTenant(h, http.MethodGet, "/api/cases/case-a/history", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("list case history status = %d, body = %s", response.Code, response.Body.String())
	}
	var history []map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil {
		t.Fatal(err)
	}
	if len(history) != 2 {
		t.Fatalf("case revision count = %d, want 2", len(history))
	}
	if history[1]["version_no"] != float64(2) {
		t.Fatalf("new case revision version = %#v, want 2", history[1]["version_no"])
	}
	if history[1]["scenario"] != scenario {
		t.Fatalf("new case revision scenario = %#v", history[1]["scenario"])
	}
	response = serveTenant(h, http.MethodPatch, "/api/cases/case-a", "tenant-b", []byte(`{"scenario":"cross tenant"}`))
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant update status = %d, want 404", response.Code)
	}
}

func TestOnlyDraftCasesCanBeEdited(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()
	response := serveTenant(h, http.MethodPost, "/api/cases/case-a/review", "tenant-a", []byte(`{"decision":"submit_review"}`))
	if response.Code != http.StatusOK {
		t.Fatalf("submit review status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serveTenant(h, http.MethodPatch, "/api/cases/case-a", "tenant-a", []byte(`{"scenario":"changed"}`))
	if response.Code != http.StatusConflict {
		t.Fatalf("edit in-review case status = %d, want 409", response.Code)
	}
}
