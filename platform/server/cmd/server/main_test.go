package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"platform/server/internal/api"
	"platform/server/internal/store"
)

func TestCombinedDeploymentServesWebAndMCP(t *testing.T) {
	directory := t.TempDir()
	webDirectory := filepath.Join(directory, "web")
	if err := os.Mkdir(webDirectory, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(webDirectory, "index.html"), []byte("Auto-Test fixture"), 0o644); err != nil {
		t.Fatal(err)
	}
	database, err := store.Open(filepath.Join(directory, "platform.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = database.Close() })
	if _, err := database.DB.Exec("INSERT INTO tenants (id, name) VALUES ('fixture', 'Fixture')"); err != nil {
		t.Fatal(err)
	}
	handler := routes(api.New(database, api.AuthOptions{InsecureDevAuth: true}).Routes(), webDirectory)
	webResponse := httptest.NewRecorder()
	handler.ServeHTTP(webResponse, httptest.NewRequest(http.MethodGet, "/", nil))
	if webResponse.Code != http.StatusOK || webResponse.Body.String() != "Auto-Test fixture" {
		t.Fatalf("web response = %d %s", webResponse.Code, webResponse.Body.String())
	}
	request := httptest.NewRequest(http.MethodPost, "/mcp", strings.NewReader(`{"jsonrpc":"2.0","id":1,"method":"tools/list"}`))
	request.Header.Set("X-Agent-Token", "tenant:fixture")
	mcpResponse := httptest.NewRecorder()
	handler.ServeHTTP(mcpResponse, request)
	if mcpResponse.Code != http.StatusOK || !strings.Contains(mcpResponse.Body.String(), "list_cases") {
		t.Fatalf("MCP response = %d %s", mcpResponse.Code, mcpResponse.Body.String())
	}
}
