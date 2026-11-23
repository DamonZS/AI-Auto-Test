package api

import (
	"encoding/json"
	"net/http"
	"testing"
)

func TestManualResultUsesExistingExecutionRecords(t *testing.T) {
	database := openTestStore(t)
	seedMCPData(t, database)
	if _, err := database.DB.Exec("UPDATE cases SET type = 'functional', status = 'approved' WHERE id = 'case-a'"); err != nil {
		t.Fatal(err)
	}
	handler := New(database).Routes()
	path := "/api/cases/case-a/manual-result"
	body := []byte(`{"status":"passed","report":"核对列表和空态均符合预期"}`)
	if response := serveTenant(handler, http.MethodPost, path, "tenant-b", body); response.Code != http.StatusConflict {
		t.Fatalf("foreign result: %d %s", response.Code, response.Body)
	}
	response := serveTenant(handler, http.MethodPost, path, "tenant-a", body)
	var result struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || response.Code != http.StatusCreated || result.RunID == "" {
		t.Fatalf("record result: %d %s", response.Code, response.Body)
	}
	var status, report string
	var passed bool
	if err := database.DB.QueryRow("SELECT status, passed, report FROM runs WHERE id = ?", result.RunID).Scan(&status, &passed, &report); err != nil || status != "passed" || !passed || report != "核对列表和空态均符合预期" {
		t.Fatalf("unexpected stored manual result: %q %t %q (%v)", status, passed, report, err)
	}
}
