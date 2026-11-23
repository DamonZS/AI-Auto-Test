package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"platform/server/internal/store"
)

func TestRequirementPointsReviewLoop(t *testing.T) {
	database, handler, editor, viewer, other := requirementFixture(t)
	for range 2 {
		if err := store.MigrateRequirementPoints(database.DB); err != nil {
			t.Fatal(err)
		}
	}
	const base = "/api/requirements/legacy-req/point-analyses"
	const source = `{"revision_no":1}`
	request := func(method, path, body string, status int) *httptest.ResponseRecorder {
		return authRequest(t, handler, method, path, "tenant-a", editor, body, status)
	}
	readAnalysis := func(response *httptest.ResponseRecorder) store.RequirementPointAnalysis {
		t.Helper()
		var analysis store.RequirementPointAnalysis
		if err := json.Unmarshal(response.Body.Bytes(), &analysis); err != nil {
			t.Fatalf("decode analysis: %v", err)
		}
		return analysis
	}
	request("GET", base+"?revision_no=1", "", http.StatusOK)
	request("POST", base, source, http.StatusUnprocessableEntity)
	const modelKey = "fixture-requirement-model-key"
	var calls atomic.Int32
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var payload struct {
			Model     string
			MaxTokens int `json:"max_tokens"`
			Messages  []struct{ Role, Content string }
		}
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer "+modelKey ||
			json.NewDecoder(r.Body).Decode(&payload) != nil || payload.Model != "requirement-fixture" || payload.MaxTokens != 1200 || len(payload.Messages) != 2 {
			t.Error("unexpected model request")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		prompt := payload.Messages[1].Content
		if !strings.Contains(prompt, "原始内容") || strings.Contains(prompt, "新内容") || strings.Contains(prompt, modelKey) {
			t.Error("model did not receive the original revision in isolation")
		}
		title := "初始功能"
		criteria := []string{"显示结果"}
		switch calls.Add(1) {
		case 1:
			criteria = []string{" "}
		case 3, 4:
			if !strings.Contains(prompt, "补充停用状态") || !strings.Contains(prompt, "初始功能") {
				t.Error("optimization is missing the previous points or feedback")
			}
			if calls.Load() == 3 {
				w.WriteHeader(http.StatusServiceUnavailable)
				return
			}
			title = "停用状态"
		}
		point := store.RequirementPoint{Title: title, Description: "功能描述", AcceptanceCriteria: criteria}
		write(w, http.StatusOK, map[string]any{"choices": []any{map[string]any{"message": map[string]string{
			"content": mustJSON(map[string]any{"points": []store.RequirementPoint{point}}),
		}}}})
	}))
	defer model.Close()
	ciphertext, err := store.Encrypt(database.MasterKey, modelKey)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := database.DB.Exec(`INSERT INTO tenant_settings
		(tenant_id, provider, model, base_url, api_key_ciphertext, max_steps, max_tokens, max_duration_seconds)
		VALUES ('tenant-a', 'openai-compatible', 'requirement-fixture', ?, ?, 1, 1200, 5)`, model.URL+"/v1", ciphertext); err != nil {
		t.Fatal(err)
	}
	authRequest(t, handler, "POST", base, "tenant-a", viewer, source, http.StatusForbidden)
	authRequest(t, handler, "POST", base, "tenant-b", other, source, http.StatusNotFound)
	request("POST", base, `{"revision_no":99}`, http.StatusNotFound)
	request("POST", base, strings.Repeat(" ", 17<<10), http.StatusRequestEntityTooLarge)
	request("POST", base, source, http.StatusBadGateway)
	first := readAnalysis(request("POST", base, source, http.StatusCreated))
	if first.Status != "draft" || first.RevisionNo != 1 || first.RoundNo != 1 || len(first.Points) != 1 {
		t.Fatalf("unexpected first draft: %#v", first)
	}
	request("POST", base, source, http.StatusConflict)
	firstPath := base + "/" + first.ID
	request("POST", firstPath+"/review", `{"action":"approve"}`, http.StatusConflict)
	request("POST", firstPath+"/review", `{"action":"submit"}`, http.StatusOK)
	request("POST", firstPath+"/review", `{"action":"return","feedback":"  "}`, http.StatusBadRequest)
	returned := readAnalysis(request("POST", firstPath+"/review", `{"action":"return","feedback":" 补充停用状态 "}`, http.StatusOK))
	if returned.Status != "returned" || returned.Feedback != "补充停用状态" || returned.ReviewedBy != "requirements-editor" {
		t.Fatalf("unexpected returned round: %#v", returned)
	}
	request("PATCH", "/api/requirements/legacy-req", `{"content":"新内容"}`, http.StatusOK)
	request("POST", firstPath+"/optimize", "", http.StatusBadGateway)
	if current := readAnalysis(request("GET", firstPath, "", http.StatusOK)); current.Status != "returned" || current.Feedback != returned.Feedback {
		t.Fatalf("failed optimization changed the returned round: %#v", current)
	}
	second := readAnalysis(request("POST", firstPath+"/optimize", "", http.StatusCreated))
	if second.ParentAnalysisID != first.ID || second.RoundNo != 2 || second.RevisionNo != 1 || second.Status != "draft" ||
		len(second.Points) != 1 || second.Points[0].Title != "停用状态" {
		t.Fatalf("unexpected optimized draft: %#v", second)
	}
	request("POST", firstPath+"/optimize", "", http.StatusConflict)
	secondPath := base + "/" + second.ID
	authRequest(t, handler, "GET", secondPath, "tenant-b", other, "", http.StatusNotFound)
	request("POST", secondPath+"/review", `{"action":"submit"}`, http.StatusOK)
	request("POST", secondPath+"/review", `{"action":"approve","version_id":"version-a"}`, http.StatusUnprocessableEntity)
	request("PUT", "/api/requirements/legacy-req/revisions/1/versions/version-a", "", http.StatusNoContent)
	approved := readAnalysis(request("POST", secondPath+"/review", `{"action":"approve","version_id":"version-a"}`, http.StatusOK))
	if approved.Status != "approved" || approved.VersionID != "version-a" || approved.SubmittedAt == "" || approved.ReviewedAt == "" {
		t.Fatalf("unexpected approval: %#v", approved)
	}
	request("POST", secondPath+"/review", `{"action":"return","feedback":"仍需调整"}`, http.StatusConflict)
	request("POST", secondPath+"/optimize", "", http.StatusConflict)
	history := authRequest(t, handler, "GET", base+"?revision_no=1", "tenant-a", viewer, "", http.StatusOK)
	var listing struct {
		Rounds     []store.RequirementPointAnalysis
		VersionIDs []string `json:"version_ids"`
	}
	if json.Unmarshal(history.Body.Bytes(), &listing) != nil || len(listing.Rounds) != 2 || len(listing.VersionIDs) != 1 ||
		listing.VersionIDs[0] != "version-a" || listing.Rounds[0].ID != approved.ID || listing.Rounds[0].Points != nil || listing.Rounds[1].Feedback != returned.Feedback {
		t.Fatalf("unexpected revision history: %s", history.Body)
	}
	if response := request("GET", base+"?revision_no=2", "", http.StatusOK); response.Body.String() != "{\"rounds\":[],\"version_ids\":[]}\n" {
		t.Fatalf("new revision: %s", response.Body)
	}
	var reviewAudits int
	err = database.DB.QueryRow(`SELECT COUNT(*) FROM audit_logs
		WHERE tenant_id = 'tenant-a' AND actor = 'requirements-editor' AND action LIKE 'requirement.points.%'
		AND target IN (?, ?)`, first.ID, second.ID).Scan(&reviewAudits)
	if err != nil || reviewAudits != 6 {
		t.Fatalf("successful analysis and review audit count = %d (%v)", reviewAudits, err)
	}
	if calls.Load() != 4 {
		t.Fatalf("expected 4 model calls, got %d", calls.Load())
	}
}
