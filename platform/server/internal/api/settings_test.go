package api

import (
	"encoding/json"
	"net/http"
	"path/filepath"
	"testing"

	"platform/server/internal/store"
)

func TestTenantPlaywrightSettings(t *testing.T) {
	database, _ := authFixture(t)
	for _, tenant := range []string{"alpha", "beta"} {
		if err := ProvisionUser(t.Context(), database, "alice", fixturePassword, tenant, "", "owner"); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(database, AuthOptions{}).Routes()
	owner := authLogin(t, handler, "alice")
	checkBrowser := func(response []byte, expected string) {
		t.Helper()
		var settings map[string]any
		if err := json.Unmarshal(response, &settings); err != nil {
			t.Fatal(err)
		}
		if actual := settings["playwright_browser"]; actual != expected {
			t.Fatalf("Playwright browser = %v, want %q", actual, expected)
		}
	}
	const legacyBody = `{"provider":"openai-compatible","model":"fixture","base_url":"https://model.example.test","max_steps":40,"max_tokens":100,"max_duration_seconds":10`
	checkBrowser(authRequest(t, handler, "PUT", "/api/settings", "alpha", owner, legacyBody+`,"playwright_browser":"firefox"}`, http.StatusOK).Body.Bytes(), "firefox")
	checkBrowser(authRequest(t, handler, "PUT", "/api/settings", "alpha", owner, legacyBody+`}`, http.StatusOK).Body.Bytes(), "firefox")
	authRequest(t, handler, "PUT", "/api/settings", "alpha", owner, legacyBody+`,"playwright_browser":"ego"}`, http.StatusBadRequest)
	checkBrowser(authRequest(t, handler, "GET", "/api/settings", "beta", owner, "", http.StatusOK).Body.Bytes(), "")
	issued := authRequest(t, handler, "POST", "/api/auth/service-tokens", "alpha", owner, `{"scope":"worker"}`, http.StatusCreated)
	var credential struct{ Token string }
	if err := json.Unmarshal(issued.Body.Bytes(), &credential); err != nil || credential.Token == "" {
		t.Fatal("missing worker token")
	}
	checkBrowser(authRequest(t, handler, "GET", "/api/agent/settings", "beta", credential.Token, "", http.StatusOK).Body.Bytes(), "firefox")
	checkBrowser(authRequest(t, handler, "PUT", "/api/settings", "alpha", owner, legacyBody+`,"playwright_browser":""}`, http.StatusOK).Body.Bytes(), "")
	const browserOnly = `{"provider":"openai-compatible","max_steps":40,"max_tokens":100,"max_duration_seconds":10,"playwright_browser":"msedge"`
	checkBrowser(authRequest(t, handler, "PUT", "/api/settings", "beta", owner, browserOnly+`}`, http.StatusOK).Body.Bytes(), "msedge")
	authRequest(t, handler, "PUT", "/api/settings", "beta", owner, browserOnly+`,"model":"fixture"}`, http.StatusBadRequest)
}

func TestTenantPlaywrightSettingsMigration(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.db")
	legacy, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	_, err = legacy.DB.Exec(`ALTER TABLE tenant_settings DROP COLUMN playwright_browser;
		INSERT INTO tenant_settings (tenant_id, model, laya_enabled) VALUES ('alpha', 'retained', 1)`)
	if closeErr := legacy.Close(); err != nil || closeErr != nil {
		t.Fatalf("prepare legacy settings: %v, %v", err, closeErr)
	}
	upgraded, err := store.Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer upgraded.Close()
	settings, err := New(upgraded).readSettings("alpha")
	if err != nil {
		t.Fatal(err)
	}
	if settings.PlaywrightBrowser != "" || settings.Model != "retained" || settings.LayaEnabled != 1 {
		t.Fatal("legacy settings or the inherited browser default changed")
	}
}
