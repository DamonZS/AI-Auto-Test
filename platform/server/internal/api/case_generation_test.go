package api

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"platform/server/internal/store"
)

const generationTestPath = "/api/cases/generate"
const generationTestKey = "fixture-model-key"

func TestCaseGenerationSavesVersionedDraftsAndRequiresApproval(t *testing.T) {
	database, handler, editor, _ := caseGenerationFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct {
			Model     string `json:"model"`
			MaxTokens int    `json:"max_tokens"`
			Messages  []struct{ Role, Content string }
		}
		if r.Method != http.MethodPost || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+generationTestKey {
			t.Error("unexpected upstream method, path or authentication")
		}
		err := json.NewDecoder(r.Body).Decode(&request)
		if err != nil || request.Model != "tenant-model" || request.MaxTokens != 400 || len(request.Messages) != 2 {
			t.Error("unexpected upstream request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		prose := request.Messages[1].Content
		if !strings.Contains(prose, "最初产品行为") || strings.Contains(prose, "最新产品行为") || strings.Contains(prose, "tenant-a") || strings.Contains(prose, "https://a.test") || strings.Contains(prose, generationTestKey) {
			t.Error("prompt must contain only the selected revision's title and prose, plus test type")
		}
		writeCaseGenerationCompletion(w, `{"cases":[{"name":"列表状态","scenario":{"request":{"method":"GET","path":"/items"},"assertions":[{"type":"status_code","expected":200}]}}]}`)
	}))
	t.Cleanup(upstream.Close)
	setCaseGenerationTestModel(t, database, upstream.URL)

	response := authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor,
		`{"project_id":"project-a","requirement_id":"generation-req","version_id":"generation-v1","type":"api"}`, http.StatusCreated)
	var result struct {
		Revision int    `json:"revision_no"`
		Mode     string `json:"generation_mode"`
		Cases    []struct{ ID, Type, Status string }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Revision != 1 || result.Mode != "platform_llm" || len(result.Cases) != 1 || result.Cases[0].Type != "api" || result.Cases[0].Status != "draft" {
		t.Fatalf("generation result = %s (%v)", response.Body, err)
	}
	caseID := result.Cases[0].ID
	var requirement, version, scenario, firstScenario, snapshot, expectedSource string
	var revision int
	err := database.DB.QueryRow(`SELECT c.requirement_id, source.version_id, source.revision_no, c.scenario, history.scenario, req.content, c.expected_source
		FROM cases c JOIN case_generation_sources source ON source.case_id = c.id AND source.tenant_id = c.tenant_id
		JOIN case_revisions history ON history.case_id = c.id AND history.tenant_id = c.tenant_id AND history.version_no = 1
		JOIN requirement_revisions req ON req.requirement_id = source.requirement_id AND req.tenant_id = source.tenant_id AND req.revision_no = source.revision_no
		WHERE c.id = ? AND c.tenant_id = 'tenant-a'`, caseID).Scan(&requirement, &version, &revision, &scenario, &firstScenario, &snapshot, &expectedSource)
	if err != nil || requirement != "generation-req" || version != "generation-v1" || revision != 1 || snapshot != "最初产品行为" || expectedSource != "requirement" || scenario != firstScenario || !strings.Contains(scenario, `"path":"/items"`) {
		t.Fatalf("bad draft provenance or scenario: %q %q %d %q %q %q (%v)", requirement, version, revision, snapshot, expectedSource, scenario, err)
	}
	runPath := "/api/cases/" + caseID + "/run"
	authRequest(t, handler, "POST", runPath, "tenant-a", editor, `{}`, http.StatusConflict)
	authRequest(t, handler, "POST", "/api/cases/"+caseID+"/review", "tenant-a", editor, `{"decision":"submit_review"}`, http.StatusOK)
	authRequest(t, handler, "POST", runPath, "tenant-a", editor, `{}`, http.StatusConflict)
	authRequest(t, handler, "POST", "/api/cases/"+caseID+"/review", "tenant-a", editor, `{"decision":"approve"}`, http.StatusOK)
	authRequest(t, handler, "POST", runPath, "tenant-a", editor, `{}`, http.StatusOK)
}

func TestCaseGenerationRejectsUnauthorizedAndUnusableInputsWithoutWriting(t *testing.T) {
	database, handler, editor, viewer := caseGenerationFixture(t)
	var calls atomic.Int32
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		writeCaseGenerationCompletion(w, `{"cases":[{"name":"可用页面","scenario":{"steps":[{"action":"assert_text","text":"欢迎"}]}},{"name":"错误动作","scenario":{"steps":[{"action":"navigate","url":"https://elsewhere.test"}]}}]}`)
	}))
	t.Cleanup(upstream.Close)
	const request = `{"project_id":"project-a","requirement_id":"generation-req","type":"ui"}`
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", "", request, http.StatusUnauthorized)
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", viewer, request, http.StatusForbidden)
	authRequest(t, handler, "POST", generationTestPath, "tenant-b", editor, request, http.StatusForbidden)
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor, strings.Replace(request, "project-a", "project-b", 1), http.StatusNotFound)
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor, strings.Replace(request, `"type"`, `"version_id":"generation-foreign","type"`, 1), http.StatusNotFound)
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor, strings.Replace(request, `"type"`, `"version_id":"generation-unlinked","type"`, 1), http.StatusUnprocessableEntity)
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor, request, http.StatusUnprocessableEntity)
	setCaseGenerationTestModel(t, database, upstream.URL)
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor, strings.Replace(request, "generation-req", "credential-req", 1), http.StatusUnprocessableEntity)
	if calls.Load() != 0 {
		t.Fatal("invalid input reached the model")
	}
	authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor, request, http.StatusBadGateway)
	if calls.Load() != 1 {
		t.Fatal("expected one model call")
	}
	var count int
	if err := database.DB.QueryRow(`SELECT COUNT(*) FROM cases WHERE generation_mode = 'platform_llm'`).Scan(&count); err != nil || count != 0 {
		t.Fatalf("failed generation saved %d drafts (%v)", count, err)
	}
}

func TestCaseGenerationKeepsLatestRevisionReadBeforeUICompletion(t *testing.T) {
	database, handler, editor, _ := caseGenerationFixture(t)
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil || len(request.Messages) != 2 || !strings.Contains(request.Messages[1].Content, "最新产品行为") {
			t.Error("latest requirement revision was not sent")
		}
		updated := "生成期间修改的产品行为"
		if _, err := database.ReviseRequirement(r.Context(), "tenant-a", "generation-req", nil, nil, &updated); err != nil {
			t.Errorf("concurrent requirement edit failed: %v", err)
		}
		writeCaseGenerationCompletion(w, `{"cases":[{"name":"列表页面","scenario":{"steps":[{"action":"assert_text","text":"项目列表"}]}}]}`)
	}))
	t.Cleanup(upstream.Close)
	setCaseGenerationTestModel(t, database, upstream.URL)
	response := authRequest(t, handler, "POST", generationTestPath, "tenant-a", editor,
		`{"project_id":"project-a","requirement_id":"generation-req","type":"ui"}`, http.StatusCreated)
	var result struct {
		Revision int `json:"revision_no"`
		Cases    []struct{ ID string }
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.Revision != 2 || len(result.Cases) != 1 {
		t.Fatalf("generation result = %s (%v)", response.Body, err)
	}
	var scenario, actor string
	var revision int
	err := database.DB.QueryRow(`SELECT c.scenario, source.revision_no, audit.actor
		FROM cases c JOIN case_generation_sources source ON c.id = source.case_id AND c.tenant_id = source.tenant_id
		JOIN audit_logs audit ON audit.target = c.id AND audit.tenant_id = c.tenant_id AND audit.action = 'case.generate.draft'
		WHERE c.id = ? AND source.version_id = ''`, result.Cases[0].ID).Scan(&scenario, &revision, &actor)
	if err != nil || revision != 2 || actor != "generation-editor" || scenario != `{"steps":[{"action":"navigate","url":"https://a.test"},{"action":"assert_text","text":"项目列表"}]}` {
		t.Fatalf("UI provenance or scenario = %d, %q, %q (%v)", revision, actor, scenario, err)
	}
}

func caseGenerationFixture(t *testing.T) (*store.Store, http.Handler, string, string) {
	t.Helper()
	database := openTestStore(t)
	seedMCPData(t, database)
	if _, err := database.DB.Exec(`INSERT INTO requirements (id, tenant_id, project_id, code, title, content) VALUES
		('generation-req','tenant-a','project-a','R-1','项目列表','最初产品行为'),
		('credential-req','tenant-a','project-a','R-2','模型设置','api_key: fixture-model-key');
		INSERT INTO versions (id, tenant_id, project_id, name) VALUES
		('generation-v1','tenant-a','project-a','1.0'), ('generation-unlinked','tenant-a','project-a','2.0'),
		('generation-foreign','tenant-b','project-b','1.0')`); err != nil {
		t.Fatal(err)
	}
	for _, account := range []struct{ name, role string }{{"generation-editor", "editor"}, {"generation-viewer", "viewer"}} {
		if err := ProvisionUser(t.Context(), database, account.name, fixturePassword, "tenant-a", "", account.role); err != nil {
			t.Fatal(err)
		}
	}
	handler := New(database, AuthOptions{}).Routes()
	editor := authLogin(t, handler, "generation-editor")
	viewer := authLogin(t, handler, "generation-viewer")
	authRequest(t, handler, "PUT", "/api/requirements/generation-req/revisions/1/versions/generation-v1", "tenant-a", editor, "", http.StatusNoContent)
	authRequest(t, handler, "PATCH", "/api/requirements/generation-req", "tenant-a", editor, `{"content":"最新产品行为"}`, http.StatusOK)
	return database, handler, editor, viewer
}

func setCaseGenerationTestModel(t *testing.T, database *store.Store, baseURL string) {
	t.Helper()
	key, err := store.Encrypt(database.MasterKey, generationTestKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`INSERT INTO tenant_settings
		(tenant_id, provider, model, base_url, api_key_ciphertext, max_tokens, max_duration_seconds)
		VALUES ('tenant-a', 'openai-compatible', 'tenant-model', ?, ?, 400, 5)`, baseURL+"/v1/", key); err != nil {
		t.Fatal(err)
	}
}

func writeCaseGenerationCompletion(w http.ResponseWriter, content string) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = io.WriteString(w, `{"choices":[{"message":{"content":`+mustJSON(content)+`}}]}`)
}
