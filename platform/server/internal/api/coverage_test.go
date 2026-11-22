package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"platform/server/internal/store"
)

const coveragePath = "/api/versions/coverage-v1/coverage"

func coverageFixture(t *testing.T) (*store.Store, http.Handler) {
	t.Helper()
	database := openTestStore(t)
	seedMCPData(t, database)
	for range 2 {
		if err := store.MigrateCoverage(database.DB); err != nil {
			t.Fatal(err)
		}
	}
	_, err := database.DB.Exec(`INSERT INTO projects (id, tenant_id, name, base_url)
		VALUES ('project-a2', 'tenant-a', 'A2', 'https://a2.test');
		INSERT INTO versions (id, tenant_id, project_id, name) VALUES
		('coverage-v1', 'tenant-a', 'project-a', 'v1'),
		('coverage-v2', 'tenant-a', 'project-a2', 'v2'),
		('coverage-v3', 'tenant-b', 'project-b', 'v3')`)
	if err != nil {
		t.Fatal(err)
	}
	mux := http.NewServeMux()
	New(database, AuthOptions{InsecureDevAuth: true}).registerCoverage(mux)
	return database, mux
}

func coverageRequest(t *testing.T, handler http.Handler, method, path, tenant, payload string, want int) *httptest.ResponseRecorder {
	t.Helper()
	response := serveTenant(handler, method, path, tenant, []byte(payload))
	if response.Code != want {
		t.Fatalf("%s %s: got %d, want %d", method, path, response.Code, want)
	}
	return response
}

func coveragePayload(format, content string) string {
	return mustJSON(map[string]string{"format": format, "content": content})
}

func TestCoverageImportAndReportSummary(t *testing.T) {
	database, handler := coverageFixture(t)
	empty := coverageRequest(t, handler, "GET", coveragePath, "tenant-a", "", 200)
	if strings.TrimSpace(empty.Body.String()) != "[]" {
		t.Fatal("a version with no evidence must return an empty array")
	}
	goFixture := "mode: atomic\r\nprivate/fixture.go:1.2,3.3 4 0\r\n" +
		"private/fixture.go:1.2,3.3 4 7\r\nprivate/fixture.go:3.4,5.5 2 0\r\n"
	response := coverageRequest(t, handler, "POST", coveragePath, "tenant-a", coveragePayload("go-coverprofile", goFixture), 200)
	if got := strings.TrimSpace(response.Body.String()); got != `{"project_id":"project-a","version_id":"coverage-v1","format":"go-coverprofile","statements":{"covered":4,"total":6}}` {
		t.Fatalf("Go summary = %s", got)
	}
	const cobertura = `<?xml version="1.0"?>
<!DOCTYPE coverage SYSTEM "coverage.dtd">
<coverage line-rate="1" branch-rate="1"><sources><source>private/path</source></sources>
  <packages><package><classes>
    <class filename="private/fixture.py"><methods><method><lines><line number="99" hits="1"/></lines></method></methods><lines>
      <line number="7" hits="2" branch="true" condition-coverage="50% (1/2)"/>
      <line number="8" hits="0"/>
    </lines></class>
    <class filename="private/fixture.py"><lines><line number="7" hits="1" branch="true" condition-coverage="50% (1/2)"/></lines></class>
    <class filename="private/another.py"><lines><line number="7" hits="0" branch="true" condition-coverage="0% (0/3)"/></lines></class>
  </classes></package></packages>
</coverage>`
	response = coverageRequest(t, handler, "POST", coveragePath, "tenant-a", coveragePayload("cobertura", cobertura), 200)
	if got := strings.TrimSpace(response.Body.String()); got != `{"project_id":"project-a","version_id":"coverage-v1","format":"cobertura","lines":{"covered":1,"total":3},"branches":{"covered":1,"total":5}}` {
		t.Fatalf("Cobertura summary = %s", got)
	}
	for _, mode := range []string{"set", "count"} {
		coverageRequest(t, handler, "POST", coveragePath, "tenant-a", coveragePayload("go-coverprofile", "mode: "+mode+"\nprivate/fixture.go:1.1,2.1 2 0\n"), 200)
	}
	response = coverageRequest(t, handler, "GET", coveragePath, "tenant-a", "", 200)
	var snapshots []store.CoverageSnapshot
	if err := json.Unmarshal(response.Body.Bytes(), &snapshots); err != nil || len(snapshots) != 2 ||
		snapshots[0].Format != "cobertura" || snapshots[1].Statements == nil || *snapshots[1].Statements != (store.CoverageCount{Total: 2}) {
		t.Fatalf("latest summaries = %s, error = %v", response.Body.String(), err)
	}
	tx, err := database.DB.BeginTx(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback()
	reportSnapshots, err := store.ListVersionCoverage(t.Context(), tx, "tenant-a", "coverage-v1")
	if err != nil || mustJSON(reportSnapshots) != mustJSON(snapshots) {
		t.Fatalf("report summaries = %s, error = %v", mustJSON(reportSnapshots), err)
	}
	var columns string
	if err := tx.QueryRow(`SELECT group_concat(name, ',') FROM pragma_table_info('version_coverage')`).Scan(&columns); err != nil ||
		columns != "tenant_id,project_id,version_id,format,covered,total,branches_covered,branches_total" {
		t.Fatalf("unexpected persisted coverage fields = %s, error = %v", columns, err)
	}
}

func TestCoverageInvalidInputPreservesEvidence(t *testing.T) {
	_, handler := coverageFixture(t)
	const valid = "mode: set\nprivate/fixture.go:1.1,2.1 1 1\n"
	coverageRequest(t, handler, "POST", coveragePath, "tenant-a", coveragePayload("go-coverprofile", valid), 200)
	before := coverageRequest(t, handler, "GET", coveragePath, "tenant-a", "", 200).Body.String()
	const xmlStart = `<coverage><packages><package><classes><class filename="private/fixture.py"><lines>`
	const xmlEnd = `</lines></class></classes></package></packages></coverage>`
	for _, test := range []struct {
		name, payload string
		status        int
	}{
		{"missing format", `{"content":"x"}`, 400},
		{"unknown field", `{"format":"cobertura","content":"x","project_id":"x"}`, 400},
		{"trailing JSON", coveragePayload("go-coverprofile", valid) + ` {}`, 400},
		{"invalid content type", `{"format":"cobertura","content":7}`, 400},
		{"unknown format", coveragePayload("unknown", valid), 422},
		{"empty report", coveragePayload("go-coverprofile", "mode: atomic\n"), 422},
		{"invalid blocks", coveragePayload("go-coverprofile", valid+"private/fixture.go:3.1,4.1 1 -1\n"), 422},
		{"conflicting blocks", coveragePayload("go-coverprofile", valid+"private/fixture.go:1.1,2.1 2 1\n"), 422},
		{"statement overflow", coveragePayload("go-coverprofile", valid+"private/fixture.go:3.1,4.1 9223372036854775807 0\n"), 422},
		{"truncated XML", coveragePayload("cobertura", xmlStart), 422},
		{"trailing XML", coveragePayload("cobertura", xmlStart+`<line number="1" hits="1"/>`+xmlEnd+`<extra/>`), 422},
		{"missing XML count", coveragePayload("cobertura", xmlStart+`<line number="1"/>`+xmlEnd), 422},
		{"invalid branch count", coveragePayload("cobertura", xmlStart+`<line number="1" hits="1" branch="true" condition-coverage="200% (2/1)"/>`+xmlEnd), 422},
		{"conflicting branch counts", coveragePayload("cobertura", xmlStart+`<line number="1" hits="1" branch="true" condition-coverage="50% (1/2)"/><line number="1" hits="1" branch="true" condition-coverage="100% (2/2)"/>`+xmlEnd), 422},
		{"oversized report", coveragePayload("go-coverprofile", strings.Repeat("a", (1<<20)+1)), 413},
		{"oversized body", coveragePayload("go-coverprofile", valid) + strings.Repeat(" ", 2<<20), 413},
	} {
		t.Run(test.name, func(t *testing.T) {
			response := coverageRequest(t, handler, "POST", coveragePath, "tenant-a", test.payload, test.status)
			var body map[string]string
			if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || body["error"] == "" || strings.Contains(body["error"], "private/") {
				t.Fatal("invalid import must return a generic JSON error")
			}
		})
	}
	after := coverageRequest(t, handler, "GET", coveragePath, "tenant-a", "", 200).Body.String()
	if before != after {
		t.Fatal("failed imports changed saved evidence")
	}
}

func TestCoverageTenantAndMemberScope(t *testing.T) {
	database, handler := coverageFixture(t)
	payload := coveragePayload("go-coverprofile", "mode: set\nexample/demo.go:1.1,2.1 1 1\n")
	coverageRequest(t, handler, "POST", coveragePath, "tenant-a", payload, 200)
	coverageRequest(t, handler, "GET", coveragePath, "tenant-b", "", 404)
	coverageRequest(t, handler, "POST", coveragePath, "tenant-b", payload, 404)
	coverageRequest(t, handler, "GET", "/api/versions/missing/coverage", "tenant-a", "", 404)
	otherProject := coverageRequest(t, handler, "GET", "/api/versions/coverage-v2/coverage", "tenant-a", "", 200)
	if strings.TrimSpace(otherProject.Body.String()) != "[]" {
		t.Fatal("another project must not see this version's evidence")
	}

	mux := http.NewServeMux()
	server := New(database, AuthOptions{})
	server.registerCoverage(mux)
	mux.HandleFunc("POST /api/auth/login", server.login)
	if err := ProvisionUser(t.Context(), database, "coverage-viewer", fixturePassword, "tenant-a", "", "viewer"); err != nil {
		t.Fatal(err)
	}
	viewer := authLogin(t, mux, "coverage-viewer")
	authRequest(t, mux, "GET", coveragePath, "tenant-a", "", "", 401)
	authRequest(t, mux, "GET", coveragePath, "tenant-a", viewer, "", 200)
	authRequest(t, mux, "POST", coveragePath, "tenant-a", viewer, payload, 403)
	authRequest(t, mux, "GET", coveragePath, "tenant-b", viewer, "", 403)

	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM version_coverage`).Scan(&count); err != nil || count != 1 {
		t.Fatalf("stored coverage rows = %d, error = %v", count, err)
	}
	if _, err := store.ListVersionCoverage(t.Context(), database.DB, "tenant-b", "coverage-v1"); err != sql.ErrNoRows {
		t.Fatalf("cross-tenant report lookup error = %v", err)
	}
}
