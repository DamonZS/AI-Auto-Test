package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"platform/server/internal/store"
)

func TestMCPJSONRPCLifecycleAndTenantIsolation(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	h := New(st).Routes()

	initialize := sendMCPRequest(t, h, "tenant-a", map[string]any{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "initialize",
		"params":  map[string]any{"protocolVersion": "2025-06-18"},
	})
	if initialize["error"] != nil {
		t.Fatalf("initialize returned error: %#v", initialize["error"])
	}
	if got := initialize["result"].(map[string]any)["serverInfo"].(map[string]any)["name"]; got != "test-platform" {
		t.Fatalf("unexpected server name: %v", got)
	}

	toolsList := sendMCPRequest(t, h, "tenant-a", map[string]any{
		"jsonrpc": "2.0",
		"id":      2,
		"method":  "tools/list",
	})
	tools := toolsList["result"].(map[string]any)["tools"].([]any)
	if !containsTool(tools, "list_cases") || !containsTool(tools, "get_run_status") {
		t.Fatalf("tools/list omitted required tools: %#v", tools)
	}

	listCases := sendMCPRequest(t, h, "tenant-a", map[string]any{
		"jsonrpc": "2.0",
		"id":      3,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "list_cases",
			"arguments": map[string]any{},
		},
	})
	cases := decodeMCPTextResult(t, listCases)
	if len(cases) != 1 || cases[0].(map[string]any)["id"] != "case-a" {
		t.Fatalf("tenant-a read unexpected cases: %#v", cases)
	}

	unknownTenant := sendMCPRequest(t, h, "tenant-missing", map[string]any{
		"jsonrpc": "2.0",
		"id":      4,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "list_cases",
			"arguments": map[string]any{},
		},
	})
	if unknownTenant["error"] == nil {
		t.Fatal("unknown tenant was accepted")
	}

	crossTenantRun := sendMCPRequest(t, h, "tenant-a", map[string]any{
		"jsonrpc": "2.0",
		"id":      5,
		"method":  "tools/call",
		"params": map[string]any{
			"name":      "get_run_status",
			"arguments": map[string]any{"run_id": "run-b"},
		},
	})
	if crossTenantRun["error"] == nil {
		t.Fatal("tenant-a read tenant-b run")
	}

	withoutFormalLogin := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":6,"method":"tools/list"}`))
	withoutFormalLogin.Header.Set("X-Agent-Token", "tenant:tenant-a")
	withoutFormalLogin.Header.Set("Authorization", "")
	response := httptest.NewRecorder()
	h.ServeHTTP(response, withoutFormalLogin)
	if response.Code != http.StatusOK {
		t.Fatalf("valid agent token unexpectedly required formal login/RBAC: status=%d body=%s", response.Code, response.Body.String())
	}

	for origin, status := range map[string]int{"https://example.com": 200, "https://other.invalid": 403, "null": 403} {
		request := httptest.NewRequest(http.MethodPost, "https://example.com/mcp", bytes.NewBufferString(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
		request.Header.Set("X-Agent-Token", "tenant:tenant-a")
		request.Header.Set("Origin", origin)
		response := httptest.NewRecorder()
		h.ServeHTTP(response, request)
		if response.Code != status {
			t.Fatalf("Origin %q got status %d, want %d", origin, response.Code, status)
		}
	}
}

func TestMCPStillQueuesLegacyDraftCases(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	response := sendMCPRequest(t, New(st).Routes(), "tenant-a", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "tools/call",
		"params": map[string]any{"name": "create_test_task", "arguments": map[string]any{"case_id": "case-a"}},
	})
	result, _ := response["result"].(map[string]any)
	queued, _ := result["structuredContent"].(map[string]any)
	if queued["status"] != "queued" || queued["run_id"] == "" || queued["run_id"] == nil {
		t.Fatalf("legacy draft no longer queues: %#v", response)
	}
}

func openTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(t.TempDir() + "/test.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func seedMCPData(t *testing.T, st *store.Store) {
	t.Helper()
	statements := []struct {
		query string
		args  []any
	}{
		{"INSERT INTO tenants (id, name) VALUES (?, ?), (?, ?)", []any{"tenant-a", "Tenant A", "tenant-b", "Tenant B"}},
		{"INSERT INTO projects (id, tenant_id, name, base_url) VALUES (?, ?, ?, ?), (?, ?, ?, ?)", []any{"project-a", "tenant-a", "A", "https://a.test", "project-b", "tenant-b", "B", "https://b.test"}},
		{"INSERT INTO cases (id, tenant_id, project_id, name, scenario) VALUES (?, ?, ?, ?, ?), (?, ?, ?, ?, ?)", []any{"case-a", "tenant-a", "project-a", "Case A", "{}", "case-b", "tenant-b", "project-b", "Case B", "{}"}},
		{"INSERT INTO runs (id, tenant_id, case_id, status) VALUES (?, ?, ?, ?), (?, ?, ?, ?)", []any{"run-a", "tenant-a", "case-a", "queued", "run-b", "tenant-b", "case-b", "queued"}},
	}
	for _, statement := range statements {
		if _, err := st.DB.Exec(statement.query, statement.args...); err != nil {
			t.Fatal(err)
		}
	}
}

func sendMCPRequest(t *testing.T, h http.Handler, tenantID string, body map[string]any) map[string]any {
	t.Helper()
	payload, err := json.Marshal(body)
	if err != nil {
		t.Fatal(err)
	}
	r := httptest.NewRequest(http.MethodPost, "/mcp", bytes.NewReader(payload))
	r.Header.Set("X-Agent-Token", "tenant:"+tenantID)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	var response map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatal(err)
	}
	return response
}

func decodeMCPTextResult(t *testing.T, response map[string]any) []any {
	t.Helper()
	if response["error"] != nil {
		t.Fatalf("MCP call returned error: %#v", response["error"])
	}
	content := response["result"].(map[string]any)["content"].([]any)
	text := content[0].(map[string]any)["text"].(string)
	var result []any
	if err := json.Unmarshal([]byte(text), &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func containsTool(tools []any, name string) bool {
	for _, item := range tools {
		if item.(map[string]any)["name"] == name {
			return true
		}
	}
	return false
}
