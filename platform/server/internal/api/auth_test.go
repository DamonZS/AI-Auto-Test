package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"platform/server/internal/store"
)

const fixturePassword = "fixture-only-password"

func TestAuthDefaultRequiresCredentials(t *testing.T) {
	t.Setenv("PLATFORM_INSECURE_DEV_AUTH", "")
	_, handler := authFixture(t)
	for _, path := range []string{"/api/tenants", "/api/overview", "/api/agent/settings"} {
		authRequest(t, handler, "GET", path, "alpha", "", "", http.StatusUnauthorized)
	}
	authRequest(t, handler, "POST", "/api/tenants", "", "", `{"name":"Public"}`, http.StatusUnauthorized)
	authRequest(t, handler, "GET", "/api/agent/settings", "", "tenant:alpha", "", http.StatusUnauthorized)
	authRequest(t, handler, "POST", "/mcp", "", "tenant:alpha", `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`, http.StatusUnauthorized)
}

func TestAuthMembershipAndSessionLifecycle(t *testing.T) {
	database, _ := authFixture(t)
	handler := New(database, AuthOptions{}).Routes()
	for _, account := range []struct{ name, tenant, role string }{
		{"alice", "alpha", "owner"}, {"alice", "beta", "viewer"}, {"bob", "alpha", "editor"},
	} {
		if err := ProvisionUser(t.Context(), database, account.name, fixturePassword, account.tenant, "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	authRequest(t, handler, "POST", "/api/auth/login", "", "", `{"username":"alice","password":"incorrect"}`, http.StatusUnauthorized)
	owner := authLogin(t, handler, "alice")
	editor := authLogin(t, handler, "bob")
	var tenants []map[string]string
	response := authRequest(t, handler, "GET", "/api/tenants", "", owner, "", http.StatusOK)
	if err := json.Unmarshal(response.Body.Bytes(), &tenants); err != nil || len(tenants) != 2 || tenants[0]["role"] != "owner" || tenants[1]["role"] != "viewer" {
		t.Fatalf("unexpected memberships: %v (%v)", tenants, err)
	}
	authRequest(t, handler, "GET", "/api/overview", "", owner, "", http.StatusBadRequest)
	authRequest(t, handler, "GET", "/api/overview", "beta", owner, "", http.StatusOK)
	authRequest(t, handler, "GET", "/api/overview", "beta", editor, "", http.StatusForbidden)
	project := `{"name":"Health","base_url":"https://example.test"}`
	authRequest(t, handler, "POST", "/api/projects", "beta", owner, project, http.StatusForbidden)
	authRequest(t, handler, "POST", "/api/projects", "alpha", editor, project, http.StatusOK)
	authRequest(t, handler, "PUT", "/api/settings", "alpha", editor, `{}`, http.StatusForbidden)
	authRequest(t, handler, "POST", "/api/patches/fixture/decide", "alpha", editor, `{}`, http.StatusForbidden)
	var created struct{ ID string }
	response = authRequest(t, handler, "POST", "/api/tenants", "", editor, `{"name":"Separate workspace"}`, http.StatusOK)
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatal("tenant creation did not return an ID")
	}
	authRequest(t, handler, "GET", "/api/auth/service-tokens", created.ID, editor, "", http.StatusOK)
	authRequest(t, handler, "GET", "/api/overview", created.ID, owner, "", http.StatusForbidden)
	authRequest(t, handler, "POST", "/api/auth/logout", "", owner, "", http.StatusNoContent)
	authRequest(t, handler, "GET", "/api/tenants", "", owner, "", http.StatusUnauthorized)
	if err := ProvisionUser(t.Context(), database, "bob", "rotated-fixture-password", "alpha", "", "viewer"); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "GET", "/api/tenants", "", editor, "", http.StatusUnauthorized)
}

func TestAuthServiceScopeAndRevocation(t *testing.T) {
	database, _ := authFixture(t)
	handler := New(database, AuthOptions{}).Routes()
	if err := ProvisionUser(t.Context(), database, "alice", fixturePassword, "alpha", "", "owner"); err != nil {
		t.Fatal(err)
	}
	owner := authLogin(t, handler, "alice")
	type credential struct {
		ID        string `json:"id"`
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	issue := func(scope string) credential {
		response := authRequest(t, handler, "POST", "/api/auth/service-tokens", "alpha", owner, `{"scope":"`+scope+`"}`, http.StatusCreated)
		var issued credential
		if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || issued.ID == "" || issued.Token == "" || issued.ExpiresAt == "" {
			t.Fatal("missing service credential fields")
		}
		return issued
	}
	worker, mcp := issue("worker"), issue("mcp")
	authRequest(t, handler, "POST", "/api/auth/service-tokens", "alpha", owner, `{"scope":"web"}`, http.StatusBadRequest)
	listed := authRequest(t, handler, "GET", "/api/auth/service-tokens", "alpha", owner, "", http.StatusOK).Body.String()
	if strings.Contains(listed, worker.Token) || strings.Contains(listed, mcp.Token) || !strings.Contains(listed, worker.ID) || !strings.Contains(listed, mcp.ID) {
		t.Fatal("unexpected service credential listing")
	}
	settings := authRequest(t, handler, "GET", "/api/agent/settings", "beta", worker.Token, "", http.StatusOK).Body.String()
	if !strings.Contains(settings, `"tenant_id":"alpha"`) {
		t.Fatal("worker settings were not bound to the credential's tenant")
	}
	toolList := `{"jsonrpc":"2.0","id":1,"method":"tools/list"}`
	request := httptest.NewRequest("POST", "/mcp", strings.NewReader(toolList))
	request.Header.Set("X-Agent-Token", mcp.Token)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), "list_cases") {
		t.Fatalf("MCP tools/list status = %d", response.Code)
	}
	authRequest(t, handler, "GET", "/api/agent/settings", "alpha", mcp.Token, "", http.StatusForbidden)
	authRequest(t, handler, "POST", "/mcp", "alpha", worker.Token, toolList, http.StatusForbidden)
	authRequest(t, handler, "POST", "/mcp", "alpha", owner, toolList, http.StatusForbidden)
	authRequest(t, handler, "GET", "/api/tenants", "", worker.Token, "", http.StatusForbidden)
	authRequest(t, handler, "DELETE", "/api/auth/service-tokens/"+worker.ID, "alpha", owner, "", http.StatusNoContent)
	authRequest(t, handler, "GET", "/api/agent/settings", "alpha", worker.Token, "", http.StatusUnauthorized)
	if _, err := database.DB.Exec("UPDATE auth_tokens SET expires_at = 0 WHERE id = ? OR scope = 'web'", mcp.ID); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "GET", "/api/tenants", "", owner, "", http.StatusUnauthorized)
	authRequest(t, handler, "POST", "/mcp", "", mcp.Token, toolList, http.StatusUnauthorized)
}

func TestAuthLegacyDatabaseMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	legacy, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.Exec(`CREATE TABLE tenants(id TEXT PRIMARY KEY, name TEXT NOT NULL, plan TEXT DEFAULT 'standard', created_at TEXT DEFAULT (datetime('now')));
		CREATE TABLE users(id TEXT PRIMARY KEY, tenant_id TEXT NOT NULL, name TEXT NOT NULL, role TEXT NOT NULL, created_at TEXT);
		INSERT INTO tenants (id, name) VALUES ('legacy', 'Legacy workspace');
		INSERT INTO users (id, tenant_id, name, role) VALUES ('legacy-user', 'legacy', 'alice', 'owner');`)
	if closeErr := legacy.Close(); err != nil || closeErr != nil {
		t.Fatalf("create legacy fixture: %v, close: %v", err, closeErr)
	}
	database, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	handler := New(database, AuthOptions{}).Routes()
	authRequest(t, handler, "POST", "/api/auth/login", "", "", `{"username":"alice","password":"unregistered-fixture-account"}`, http.StatusUnauthorized)
	if err := ProvisionUser(t.Context(), database, "alice", fixturePassword, "legacy", "", "owner"); err != nil {
		t.Fatal(err)
	}
	response := authRequest(t, handler, "GET", "/api/tenants", "", authLogin(t, handler, "alice"), "", http.StatusOK)
	if !strings.Contains(response.Body.String(), `"name":"Legacy workspace"`) {
		t.Fatal("legacy workspace missing after explicit provisioning")
	}
	var role string
	if err := database.DB.QueryRow("SELECT role FROM users WHERE id = 'legacy-user'").Scan(&role); err != nil || role != "owner" {
		t.Fatalf("legacy users were changed: %v", err)
	}
}

func TestAuthMemberProvidesActor(t *testing.T) {
	database, _ := authFixture(t)
	if err := ProvisionUser(t.Context(), database, "alice", fixturePassword, "alpha", "", "editor"); err != nil {
		t.Fatal(err)
	}
	server := New(database, AuthOptions{})
	token := authLogin(t, server.Routes(), "alice")
	next := func(w http.ResponseWriter, r *http.Request, tenantID string) {
		write(w, http.StatusOK, map[string]string{"actor": authenticatedActor(r)})
	}
	response := authRequest(t, server.tenant(next), "POST", "/api/workbench/fixture/review", "alpha", token, `{"reviewer":"someone-else"}`, http.StatusOK)
	if response.Body.String() != "{\"actor\":\"alice\"}\n" {
		t.Fatal("tenant middleware did not pass the authenticated username")
	}
	legacy := New(database, AuthOptions{InsecureDevAuth: true})
	response = authRequest(t, legacy.tenant(next), "POST", "/api/workbench/fixture/review", "alpha", "", `{}`, http.StatusOK)
	if response.Body.String() != "{\"actor\":\"\"}\n" {
		t.Fatal("legacy request unexpectedly has an authenticated actor")
	}
}

func authFixture(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	database, err := store.Open(filepath.Join(t.TempDir(), "auth.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.DB.Exec("INSERT INTO tenants (id, name) VALUES ('alpha', 'Alpha'), ('beta', 'Beta')"); err != nil {
		t.Fatal(err)
	}
	return database, New(database).Routes()
}

func authLogin(t *testing.T, handler http.Handler, username string) string {
	t.Helper()
	body, err := json.Marshal(map[string]string{"username": username, "password": fixturePassword})
	if err != nil {
		t.Fatal(err)
	}
	response := authRequest(t, handler, "POST", "/api/auth/login", "", "", string(body), http.StatusOK)
	var issued struct {
		Token     string `json:"token"`
		ExpiresAt string `json:"expires_at"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || issued.Token == "" || issued.ExpiresAt == "" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("invalid login response")
	}
	return issued.Token
}

func authRequest(t *testing.T, handler http.Handler, method, path, tenantID, token, body string, status int) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/json")
	if tenantID != "" {
		request.Header.Set(tenantHeader, tenantID)
	}
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status {
		t.Fatalf("%s %s: got %d want %d", method, path, response.Code, status)
	}
	return response
}
