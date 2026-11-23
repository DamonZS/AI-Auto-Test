package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const desktopWorkerOrigin = "http://127.0.0.1:8090"

func TestDesktopWorkerActivation(t *testing.T) {
	database, _ := authFixture(t)
	auth := New(database, AuthOptions{}).Routes()
	for _, account := range []struct{ username, tenant, role string }{
		{"owner", "alpha", "owner"}, {"owner", "beta", "owner"}, {"editor", "alpha", "editor"},
	} {
		if err := ProvisionUser(context.Background(), database, account.username, fixturePassword, account.tenant, "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	owner := authLogin(t, auth, "owner")
	editor := authLogin(t, auth, "editor")
	issue := func(tenant, scope string) struct{ ID, Token string } {
		t.Helper()
		response := authRequest(t, auth, "POST", "/api/auth/service-tokens", tenant, owner, `{"scope":"`+scope+`"}`, http.StatusCreated)
		var issued struct{ ID, Token string }
		if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || issued.Token == "" {
			t.Fatal("missing issued worker token")
		}
		return issued
	}
	alpha := issue("alpha", "worker")
	beta := issue("beta", "worker")
	mcp := issue("alpha", "mcp")
	expired := issue("alpha", "worker")
	if _, err := database.DB.Exec("UPDATE auth_tokens SET expires_at = 0 WHERE id = ?", expired.ID); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	server := New(database, AuthOptions{DesktopSetup: true, DesktopWorkerDir: dir})
	handler := server.Routes()
	request := func(method, origin, tenant, session, token string, status int) string {
		t.Helper()
		return desktopWorkerRequest(t, handler, method, origin, tenant, session, token, status)
	}
	if body := request("GET", "", "alpha", owner, "", http.StatusOK); !strings.Contains(body, `"inactive"`) {
		t.Fatalf("unexpected initial status: %s", body)
	}
	desktopWorkerRequest(t, auth, "POST", desktopWorkerOrigin, "alpha", owner, alpha.Token, http.StatusNotFound)
	request("POST", "", "alpha", owner, alpha.Token, http.StatusForbidden)
	request("POST", "http://localhost:8090", "alpha", owner, alpha.Token, http.StatusForbidden)
	request("POST", desktopWorkerOrigin, "alpha", "", alpha.Token, http.StatusUnauthorized)
	request("POST", desktopWorkerOrigin, "alpha", editor, alpha.Token, http.StatusForbidden)
	request("POST", desktopWorkerOrigin, "alpha", alpha.Token, alpha.Token, http.StatusForbidden)
	for _, token := range []string{"invalid", beta.Token, mcp.Token, expired.Token} {
		request("POST", desktopWorkerOrigin, "alpha", owner, token, http.StatusBadRequest)
	}
	managed := filepath.Join(dir, "desktop-worker-token.txt")
	if _, err := os.Stat(managed); !os.IsNotExist(err) {
		t.Fatal("a rejected activation created the managed token file")
	}

	manual := filepath.Join(dir, "worker-token.txt")
	if err := os.WriteFile(manual, []byte(beta.Token), 0600); err != nil {
		t.Fatal(err)
	}
	if body := request("GET", "", "alpha", owner, "", http.StatusOK); !strings.Contains(body, `"external"`) {
		t.Fatalf("unexpected manual credential status: %s", body)
	}
	request("POST", desktopWorkerOrigin, "alpha", owner, alpha.Token, http.StatusConflict)
	if stored, err := os.ReadFile(manual); err != nil || string(stored) != beta.Token {
		t.Fatal("manual token was changed")
	}
	if err := os.Remove(manual); err != nil {
		t.Fatal(err)
	}

	if body := request("POST", desktopWorkerOrigin, "alpha", owner, alpha.Token, http.StatusOK); !strings.Contains(body, `"active"`) {
		t.Fatalf("unexpected activation result: %s", body)
	}
	if stored, err := os.ReadFile(managed); err != nil || strings.TrimSpace(string(stored)) != alpha.Token {
		t.Fatal("managed token was not stored")
	}
	authRequest(t, auth, "POST", "/api/runs/heartbeat", "", alpha.Token, "{}", http.StatusOK)
	if body := request("GET", "", "beta", owner, "", http.StatusOK); !strings.Contains(body, `"other_workspace"`) {
		t.Fatalf("unexpected other workspace status: %s", body)
	}
	request("POST", desktopWorkerOrigin, "beta", owner, beta.Token, http.StatusConflict)
	authRequest(t, auth, "DELETE", "/api/auth/service-tokens/"+alpha.ID, "alpha", owner, "", http.StatusNoContent)
	request("POST", desktopWorkerOrigin, "beta", owner, beta.Token, http.StatusOK)
	if stored, err := os.ReadFile(managed); err != nil || strings.TrimSpace(string(stored)) != beta.Token {
		t.Fatal("revoked managed token was not replaced")
	}
}

func desktopWorkerRequest(t *testing.T, handler http.Handler, method, origin, tenant, session, token string, status int) string {
	t.Helper()
	request := httptest.NewRequest(method, desktopWorkerOrigin+"/api/desktop/worker", strings.NewReader(`{"token":"`+token+`"}`))
	request.RemoteAddr = "127.0.0.1:51000"
	request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8090}))
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	request.Header.Set(tenantHeader, tenant)
	if session != "" {
		request.Header.Set("Authorization", "Bearer "+session)
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != status || response.Header().Get("Cache-Control") != "no-store" || response.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Fatalf("%s: got status %d, want %d with no-store and no CORS", method, response.Code, status)
	}
	if token != "" && strings.Contains(response.Body.String(), token) && len(token) > 10 {
		t.Fatal("response exposed the token")
	}
	return response.Body.String()
}
