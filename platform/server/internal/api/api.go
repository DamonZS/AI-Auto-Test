package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/google/uuid"
	"platform/server/internal/store"
)

type Server struct {
	st           *store.Store
	legacyAuth   bool
	desktopSetup bool
}

type AuthOptions struct {
	InsecureDevAuth bool
	DesktopSetup    bool
}

func New(st *store.Store, options ...AuthOptions) *Server {
	auth := AuthOptions{InsecureDevAuth: os.Getenv("PLATFORM_INSECURE_DEV_AUTH") == "1"}
	if len(options) != 0 {
		auth = options[0]
	}
	return &Server{st: st, legacyAuth: auth.InsecureDevAuth, desktopSetup: auth.DesktopSetup}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET "+desktopSetupPath, s.desktopSetupStatus)
	mux.HandleFunc("POST "+desktopSetupPath, s.registerDesktopOwner)
	mux.HandleFunc("POST /api/auth/login", s.login)
	mux.HandleFunc("POST /api/auth/logout", s.webSession(s.logout))
	mux.HandleFunc("GET /api/auth/service-tokens", s.member(s.listServiceTokens, true))
	mux.HandleFunc("POST /api/auth/service-tokens", s.member(s.issueServiceToken, true))
	mux.HandleFunc("DELETE /api/auth/service-tokens/{id}", s.member(s.revokeServiceToken, true))
	mux.HandleFunc("POST /api/tenants", s.workspaceAccount(s.createTenant))
	mux.HandleFunc("GET /api/tenants", s.workspaceAccount(s.listTenants))
	mux.HandleFunc("GET /api/overview", s.tenant(s.overview))
	mux.HandleFunc("GET /api/settings", s.tenant(s.getSettings))
	mux.HandleFunc("PUT /api/settings", s.owner(s.putSettings))
	mux.HandleFunc("GET /api/agent/settings", s.agent(s.getAgentSettings))
	mux.HandleFunc("POST /mcp", s.service(scopeMCP, s.mcpEndpoint))

	mux.HandleFunc("GET /api/projects", s.tenant(s.listProjects))
	mux.HandleFunc("POST /api/projects", s.tenant(s.createProject))

	mux.HandleFunc("GET /api/requirements", s.tenant(s.listRequirements))
	mux.HandleFunc("POST /api/requirements", s.tenant(s.createRequirement))
	s.registerRequirements(mux)

	mux.HandleFunc("GET /api/versions", s.tenant(s.listVersions))
	mux.HandleFunc("POST /api/versions", s.tenant(s.createVersion))

	mux.HandleFunc("GET /api/environments", s.tenant(s.listEnvironments))
	mux.HandleFunc("POST /api/environments", s.tenant(s.createEnvironment))

	mux.HandleFunc("GET /api/cases", s.tenant(s.listCases))
	mux.HandleFunc("POST /api/cases", s.tenant(s.createCase))
	s.registerCaseGenerationRoutes(mux)
	mux.HandleFunc("GET /api/suites", s.tenant(s.listSuites))
	mux.HandleFunc("POST /api/suites", s.tenant(s.createSuite))
	mux.HandleFunc("PATCH /api/suites/{id}", s.tenant(s.updateSuiteConcurrency))
	mux.HandleFunc("POST /api/suites/{id}/run", s.tenant(s.runSuite))
	mux.HandleFunc("GET /api/suites/{id}/runs", s.tenant(s.listSuiteRuns))
	mux.HandleFunc("GET /api/suites/{id}/schedule", s.tenant(s.getSuiteSchedule))
	mux.HandleFunc("PUT /api/suites/{id}/schedule", s.tenant(s.putSuiteSchedule))
	mux.HandleFunc("DELETE /api/suites/{id}/schedule", s.tenant(s.deleteSuiteSchedule))
	mux.HandleFunc("GET /api/suite-runs/{id}", s.tenant(s.suiteRunSummary))
	mux.HandleFunc("PATCH /api/cases/{id}", s.tenant(s.updateCase))
	mux.HandleFunc("GET /api/cases/{id}/history", s.tenant(s.listCaseHistory))
	mux.HandleFunc("POST /api/cases/{id}/review", s.tenant(s.reviewCase))

	mux.HandleFunc("GET /api/test-data/datasets", s.tenant(s.listDataSets))
	mux.HandleFunc("POST /api/test-data/datasets", s.tenant(s.createDataSet))
	mux.HandleFunc("POST /api/test-data/datasets/{id}/generate", s.tenant(s.generateData))
	mux.HandleFunc("GET /api/test-data/datasets/{id}/records", s.tenant(s.listDataRecords))
	mux.HandleFunc("GET /api/test-data/datasets/{id}/raw-records", s.agent(s.rawDataRecords))

	mux.HandleFunc("GET /api/runs", s.tenant(s.listRuns))
	mux.HandleFunc("GET /api/runs/{id}", s.tenant(s.getRun))
	mux.HandleFunc("POST /api/cases/{id}/run", s.tenant(s.runCase))
	mux.HandleFunc("GET /api/cases/{id}/schedule", s.tenant(s.getCaseSchedule))
	mux.HandleFunc("PUT /api/cases/{id}/schedule", s.tenant(s.putCaseSchedule))
	mux.HandleFunc("DELETE /api/cases/{id}/schedule", s.tenant(s.deleteCaseSchedule))
	mux.HandleFunc("POST /api/runs/{id}/heartbeat", s.agent(s.heartbeat))
	mux.HandleFunc("POST /api/runs/heartbeat", s.agent(s.heartbeat))
	mux.HandleFunc("POST /api/runs/{id}/finish", s.agent(s.finishRun))

	mux.HandleFunc("GET /api/bugs", s.tenant(s.listBugs))
	mux.HandleFunc("POST /api/bugs", s.tenant(s.createBug))
	mux.HandleFunc("POST /api/bugs/{id}/transition", s.tenant(s.transitionBug))

	mux.HandleFunc("GET /api/patches", s.tenant(s.listPatches))
	mux.HandleFunc("POST /api/patches/{id}/decide", s.owner(s.decidePatch))

	mux.HandleFunc("GET /api/trends", s.tenant(s.trends))
	s.registerWorkbench(mux)
	s.registerCoverage(mux)
	return cors(mux)
}

func cors(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == desktopSetupPath {
			w.Header().Set("Cache-Control", "no-store")
			h.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Headers", "Authorization, Content-Type, X-Tenant-Id, X-Agent-Token, Mcp-Session-Id, MCP-Protocol-Version")
		w.Header().Set("Access-Control-Expose-Headers", "Mcp-Session-Id")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusNoContent)
			return
		}
		h.ServeHTTP(w, r)
	})
}

func write(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func fail(w http.ResponseWriter, status int, msg string) {
	write(w, status, map[string]string{"error": msg})
}

func decode(r *http.Request, v any) error {
	defer r.Body.Close()
	return json.NewDecoder(r.Body).Decode(v)
}

func id() string { return strings.ReplaceAll(uuid.NewString(), "-", "")[:12] }

func rowsToMaps(rows interface {
	Next() bool
	Scan(...any) error
	Columns() ([]string, error)
	Close() error
}) ([]map[string]any, error) {
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			switch v := vals[i].(type) {
			case []byte:
				m[c] = string(v)
			default:
				m[c] = v
			}
		}
		out = append(out, m)
	}
	return out, nil
}

func mustJSON(v any) string {
	b, err := json.Marshal(v)
	if err != nil {
		panic(fmt.Sprintf("marshal: %v", err))
	}
	return string(b)
}
