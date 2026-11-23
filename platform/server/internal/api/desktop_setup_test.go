package api

import (
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"platform/server/internal/store"
)

func TestDesktopSetupDisabled(t *testing.T) {
	_, handler := authFixture(t)
	response := authRequest(t, handler, http.MethodGet, "/api/auth/setup", "", "", "", http.StatusOK)
	var state struct {
		Available *bool `json:"available"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &state); err != nil || state.Available == nil || *state.Available {
		t.Fatalf("unexpected setup status: %s (%v)", response.Body, err)
	}
	authRequest(t, handler, http.MethodPost, "/api/auth/setup", "", "", `{}`, http.StatusNotFound)
}

func TestDesktopSetupCreatesOwnerAndPreservesWorkspaces(t *testing.T) {
	database, _ := authFixture(t)
	handler := New(database, AuthOptions{DesktopSetup: true}).Routes()
	request := desktopSetupRequest(http.MethodGet, "127.0.0.1:8090", "", "")
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "{\"available\":true}\n" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("initial status: %d %s", response.Code, response.Body)
	}
	request = desktopSetupRequest(http.MethodPost, "127.0.0.1:8090", "http://127.0.0.1:8090", `{"workspace_name":"Alpha","username":"alice","password":"short"}`)
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusBadRequest {
		t.Fatalf("invalid password status: %d", response.Code)
	}
	body := mustJSON(map[string]string{"workspace_name": " Alpha ", "username": " alice ", "password": fixturePassword})
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, desktopSetupRequest(http.MethodPost, "[::1]:8090", "http://[::1]:8090", body))
	var created map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &created); response.Code != http.StatusCreated || err != nil || len(created) != 1 || created["workspace_id"] == "" || created["workspace_id"] == "alpha" {
		t.Fatalf("invalid creation response: %d %v", response.Code, err)
	}
	token := authLogin(t, handler, "alice")
	response = authRequest(t, handler, http.MethodGet, "/api/tenants", "", token, "", http.StatusOK)
	var memberships []map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &memberships); err != nil || len(memberships) != 1 || memberships[0]["id"] != created["workspace_id"] || memberships[0]["name"] != "Alpha" || memberships[0]["role"] != "owner" {
		t.Fatalf("unexpected memberships: %v (%v)", memberships, err)
	}
	var originalWorkspaces int
	if err := database.DB.QueryRow("SELECT COUNT(*) FROM tenants WHERE (id = 'alpha' AND name = 'Alpha') OR (id = 'beta' AND name = 'Beta')").Scan(&originalWorkspaces); err != nil || originalWorkspaces != 2 {
		t.Fatalf("existing workspaces changed: %d (%v)", originalWorkspaces, err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, desktopSetupRequest(http.MethodGet, "127.0.0.1:8090", "", ""))
	if response.Code != http.StatusOK || response.Body.String() != "{\"available\":false}\n" {
		t.Fatalf("status after creation: %d %s", response.Code, response.Body)
	}
}

func TestDesktopSetupNetworkAndOrigin(t *testing.T) {
	database, _ := authFixture(t)
	handler := New(database, AuthOptions{DesktopSetup: true}).Routes()
	const localHost = "127.0.0.1:8090"
	const localOrigin = "http://" + localHost
	for _, test := range []struct {
		name, method, host, origin, remote string
		local                              net.Addr
		status                             int
	}{
		{name: "remote peer", method: http.MethodGet, host: localHost, remote: "192.0.2.1:40001", status: http.StatusForbidden},
		{name: "non-loopback local address", method: http.MethodGet, host: localHost, local: &net.TCPAddr{IP: net.IPv4zero, Port: 8090}, status: http.StatusForbidden},
		{name: "not TCP", method: http.MethodGet, host: localHost, local: &net.UnixAddr{Name: "desktop.sock", Net: "unix"}, status: http.StatusForbidden},
		{name: "hostname", method: http.MethodGet, host: "localhost:8090", status: http.StatusForbidden},
		{name: "non-loopback host", method: http.MethodGet, host: "192.0.2.1:8090", status: http.StatusForbidden},
		{name: "foreign GET origin", method: http.MethodGet, host: localHost, origin: "http://127.0.0.1:3000", status: http.StatusForbidden},
		{name: "foreign POST origin", method: http.MethodPost, host: localHost, origin: "https://" + localHost, status: http.StatusForbidden},
		{name: "missing POST origin", method: http.MethodPost, host: localHost, status: http.StatusForbidden},
		{name: "same origin", method: http.MethodGet, host: localHost, origin: localOrigin, status: http.StatusOK},
		{name: "preflight", method: http.MethodOptions, host: localHost, origin: "https://example.test", status: http.StatusMethodNotAllowed},
	} {
		t.Run(test.name, func(t *testing.T) {
			request := desktopSetupRequest(test.method, test.host, test.origin, "")
			if test.remote != "" {
				request.RemoteAddr = test.remote
			}
			if test.local != nil {
				request = request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, test.local))
			}
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, request)
			if response.Code != test.status || response.Header().Get("Access-Control-Allow-Origin") != "" {
				t.Fatalf("status %d, CORS %q, body %s", response.Code, response.Header().Get("Access-Control-Allow-Origin"), response.Body)
			}
		})
	}
}

func TestDesktopSetupRejectsProvisionedIdentity(t *testing.T) {
	database, _ := authFixture(t)
	if err := ProvisionUser(t.Context(), database, "alice", fixturePassword, "alpha", "", "editor"); err != nil {
		t.Fatal(err)
	}
	handler := New(database, AuthOptions{DesktopSetup: true}).Routes()
	token := authLogin(t, handler, "alice")
	body := mustJSON(map[string]string{"workspace_name": "Other", "username": "alice", "password": "different-fixture-passphrase"})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, desktopSetupRequest(http.MethodPost, "127.0.0.1:8090", "http://127.0.0.1:8090", body))
	if response.Code != http.StatusConflict {
		t.Fatalf("setup with existing identity: %d %s", response.Code, response.Body)
	}
	response = authRequest(t, handler, http.MethodGet, "/api/tenants", "", token, "", http.StatusOK)
	if !strings.Contains(response.Body.String(), `"role":"editor"`) || strings.Contains(response.Body.String(), "Other") {
		t.Fatalf("existing membership changed: %s", response.Body)
	}
	authLogin(t, handler, "alice")
}

func TestDesktopSetupConcurrentRequests(t *testing.T) {
	database, _ := authFixture(t)
	start := make(chan struct{})
	statuses := make(chan int, 2)
	for _, username := range []string{"alice", "bob"} {
		handler := New(database, AuthOptions{DesktopSetup: true}).Routes()
		go func() {
			<-start
			body := mustJSON(map[string]string{"workspace_name": "New", "username": username, "password": fixturePassword})
			response := httptest.NewRecorder()
			handler.ServeHTTP(response, desktopSetupRequest(http.MethodPost, "127.0.0.1:8090", "http://127.0.0.1:8090", body))
			statuses <- response.Code
		}()
	}
	close(start)
	statusCounts := map[int]int{<-statuses: 1}
	statusCounts[<-statuses]++
	if statusCounts[http.StatusCreated] != 1 || statusCounts[http.StatusConflict] != 1 {
		t.Fatalf("unexpected concurrent results: %v", statusCounts)
	}
	var workspaces, identities, owners int
	err := database.DB.QueryRow(`SELECT (SELECT COUNT(*) FROM tenants), (SELECT COUNT(*) FROM auth_identities),
		(SELECT COUNT(*) FROM tenant_memberships WHERE role = 'owner')`).Scan(&workspaces, &identities, &owners)
	if err != nil || workspaces != 3 || identities != 1 || owners != 1 {
		t.Fatalf("workspaces=%d identities=%d owners=%d error=%v", workspaces, identities, owners, err)
	}
}

func TestDesktopSetupTCP(t *testing.T) {
	for _, listen := range []string{"127.0.0.1:0", "[::1]:0"} {
		t.Run(listen, func(t *testing.T) {
			database, err := store.Open(filepath.Join(t.TempDir(), "desktop.db"))
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			listener, err := net.Listen("tcp", listen)
			if err != nil {
				t.Fatal(err)
			}
			server := &httptest.Server{Listener: listener, Config: &http.Server{Handler: New(database, AuthOptions{DesktopSetup: true}).Routes()}}
			server.Start()
			defer server.Close()
			body := mustJSON(map[string]string{"workspace_name": "Desktop", "username": "alice", "password": fixturePassword})
			request, err := http.NewRequestWithContext(t.Context(), http.MethodPost, server.URL+"/api/auth/setup", strings.NewReader(body))
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Origin", server.URL)
			request.Header.Set("Content-Type", "application/json")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			if response.StatusCode != http.StatusCreated {
				t.Fatalf("setup over %s returned %s", listen, response.Status)
			}
		})
	}
}

func desktopSetupRequest(method, host, origin, body string) *http.Request {
	request := httptest.NewRequest(method, "http://"+host+"/api/auth/setup", strings.NewReader(body))
	address := &net.TCPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 8090}
	request.RemoteAddr = "127.0.0.1:40001"
	request.Header.Set("Content-Type", "application/json")
	if origin != "" {
		request.Header.Set("Origin", origin)
	}
	return request.WithContext(context.WithValue(request.Context(), http.LocalAddrContextKey, address))
}
