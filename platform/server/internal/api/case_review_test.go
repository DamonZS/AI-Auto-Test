package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCaseReviewUsesFeedbackAndSessionAcrossRevisions(t *testing.T) {
	database, handler, editor, viewer := caseGenerationFixture(t)
	if err := ProvisionUser(t.Context(), database, "case-reviewer", fixturePassword, "tenant-a", "", "editor"); err != nil {
		t.Fatal(err)
	}
	reviewer := authLogin(t, handler, "case-reviewer")
	completion := `{"cases":[{"name":"项目列表","scenario":{"steps":[{"action":"查看项目列表","expected":"显示项目"}]}}]}`
	feedback := "请补充空列表的预期结果"
	var reviseRequest atomic.Bool
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct {
			Messages []struct{ Content string }
		}
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
			t.Error(err)
		}
		if len(input.Messages) == 2 && strings.Contains(input.Messages[1].Content, feedback) {
			reviseRequest.Store(true)
			writeCaseGenerationCompletion(w, `{"cases":[{"name":"项目列表","scenario":{"steps":[{"action":"查看项目列表","expected":"无项目时显示空列表"}]}}]}`)
			return
		}
		writeCaseGenerationCompletion(w, completion)
	}))
	defer model.Close()
	setCaseGenerationTestModel(t, database, model.URL)
	response := authRequest(t, handler, "POST", "/api/cases/generate", "tenant-a", editor,
		`{"project_id":"project-a","requirement_id":"generation-req","type":"functional"}`, http.StatusCreated)
	var generated struct{ Cases []mcpCaseDraft }
	if err := json.Unmarshal(response.Body.Bytes(), &generated); err != nil || len(generated.Cases) != 1 {
		t.Fatalf("unexpected generation response: %s", response.Body.String())
	}
	path := "/api/cases/" + generated.Cases[0].ID
	const manualResult = `{"status":"passed","report":"空列表显示符合预期","version_id":"generation-v1"}`
	authRequest(t, handler, "POST", path+"/manual-result", "tenant-a", editor, manualResult, http.StatusConflict)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", reviewer, `{"decision":"approve"}`, http.StatusConflict)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", editor, `{"decision":"submit_review"}`, http.StatusOK)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", reviewer, `{"decision":"reject"}`, http.StatusBadRequest)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", reviewer, `{"decision":"reject","feedback":"`+feedback+`","reviewer":"spoof"}`, http.StatusOK)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", editor, `{"decision":"submit_review"}`, http.StatusConflict)
	authRequest(t, handler, "POST", path+"/revise-with-ai", "tenant-a", viewer, `{}`, http.StatusForbidden)
	response = authRequest(t, handler, "POST", path+"/revise-with-ai", "tenant-a", editor, `{}`, http.StatusOK)
	if !reviseRequest.Load() || !strings.Contains(response.Body.String(), `"version_no":2`) || !strings.Contains(response.Body.String(), `"status":"draft"`) {
		t.Fatalf("unexpected revision: %s", response.Body.String())
	}
	authRequest(t, handler, "POST", path+"/revise-with-ai", "tenant-a", editor, `{}`, http.StatusConflict)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", editor, `{"decision":"submit_review"}`, http.StatusOK)
	authRequest(t, handler, "POST", path+"/review", "tenant-a", reviewer, `{"decision":"approve","reviewer":"spoof"}`, http.StatusOK)
	authRequest(t, handler, "POST", path+"/run", "tenant-a", editor, `{}`, http.StatusConflict)
	authRequest(t, handler, "POST", path+"/manual-result", "tenant-a", editor,
		strings.Replace(manualResult, "generation-v1", "generation-foreign", 1), http.StatusConflict)
	response = authRequest(t, handler, "POST", path+"/manual-result", "tenant-a", editor, manualResult, http.StatusCreated)
	var result struct {
		RunID string `json:"run_id"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || result.RunID == "" {
		t.Fatalf("unexpected manual result: %s", response.Body.String())
	}
	var runStatus, runVersion, runReport string
	if err := database.DB.QueryRow(`SELECT status, version_id, report FROM runs WHERE id = ? AND tenant_id = 'tenant-a' AND ended_at IS NOT NULL`, result.RunID).
		Scan(&runStatus, &runVersion, &runReport); err != nil || runStatus != "passed" || runVersion != "generation-v1" || runReport != "空列表显示符合预期" {
		t.Fatalf("unexpected saved run: %q %q %q (%v)", runStatus, runVersion, runReport, err)
	}
	response = authRequest(t, handler, "GET", path+"/history", "tenant-a", editor, "", http.StatusOK)
	var history []struct{ Scenario string }
	if err := json.Unmarshal(response.Body.Bytes(), &history); err != nil || len(history) != 2 ||
		!strings.Contains(history[0].Scenario, "显示项目") || !strings.Contains(history[1].Scenario, "无项目时显示空列表") {
		t.Fatalf("unexpected history: %s", response.Body.String())
	}
	var storedReviewer string
	if err := database.DB.QueryRow(`SELECT reviewer FROM cases WHERE id = ?`, generated.Cases[0].ID).Scan(&storedReviewer); err != nil || storedReviewer != "case-reviewer" {
		t.Fatalf("reviewer = %q, err = %v", storedReviewer, err)
	}
}

func TestCaseGenerationUsesApprovedFunctionalPointsAndWorkerShapes(t *testing.T) {
	database, handler, editor, _ := caseGenerationFixture(t)
	const points = `{"points":[{"title":"列表需求","description":"展示列表","acceptance_criteria":["返回项目清单"]}]}`
	var caseOutput atomic.Value
	caseOutput.Store("")
	model := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var input struct{ Messages []struct{ Content string } }
		if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Messages) != 2 {
			t.Error("missing model input")
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		output := caseOutput.Load().(string)
		if output == "" {
			writeCaseGenerationCompletion(w, points)
			return
		}
		prompt := input.Messages[1].Content
		if !strings.Contains(prompt, "最初产品行为") || !strings.Contains(prompt, "返回项目清单") || strings.Contains(prompt, "最新产品行为") {
			t.Errorf("wrong approved requirement revision or points: %s", prompt)
		}
		writeCaseGenerationCompletion(w, output)
	}))
	defer model.Close()
	setCaseGenerationTestModel(t, database, model.URL)
	call := func(path, body string, status int) *httptest.ResponseRecorder {
		return authRequest(t, handler, "POST", path, "tenant-a", editor, body, status)
	}
	const analysisPath = "/api/requirements/generation-req/point-analyses"
	response := call(analysisPath, `{"revision_no":1}`, http.StatusCreated)
	var analysis struct{ ID string }
	if err := json.Unmarshal(response.Body.Bytes(), &analysis); err != nil || analysis.ID == "" {
		t.Fatalf("unexpected analysis: %s", response.Body.String())
	}
	const generationPath = "/api/cases/generate"
	request := func(kind, version string) string {
		return mustJSON(caseGenerationInput{ProjectID: "project-a", RequirementID: "generation-req", VersionID: version, ReviewID: analysis.ID, Type: kind})
	}
	call(generationPath, request("perf", "generation-v1"), http.StatusNotFound)
	call(analysisPath+"/"+analysis.ID+"/review", `{"action":"submit"}`, http.StatusOK)
	call(analysisPath+"/"+analysis.ID+"/review", `{"action":"approve","version_id":"generation-v1"}`, http.StatusOK)
	call(generationPath, request("perf", "generation-unlinked"), http.StatusUnprocessableEntity)
	for _, sample := range []struct {
		kind, output, stored string
		status               int
	}{
		{"perf", `{"request":{"method":"HEAD","path":"/items"},"concurrency":2,"duration_seconds":5,"max_requests":10,"timeout":1}`, `"duration_seconds":5`, http.StatusCreated},
		{"perf", `{"request":{"method":"GET","path":"/items"},"concurrency":6,"duration_seconds":5,"max_requests":10,"timeout":1}`, "", http.StatusBadGateway},
		{"security", `{"checklist":["authentication","configuration"]}`, `{"checklist":["authentication","configuration"]}`, http.StatusCreated},
		{"security", `{"checklist":["custom"]}`, "", http.StatusBadGateway},
		{"security", `{"checklist":["authentication"],"request":{}}`, "", http.StatusBadGateway},
	} {
		caseOutput.Store(`{"cases":[{"name":"项目列表","scenario":` + sample.output + `}]}`)
		response := call(generationPath, request(sample.kind, "generation-v1"), sample.status)
		if sample.status != http.StatusCreated {
			continue
		}
		var result struct{ Cases []mcpCaseDraft }
		if err := json.Unmarshal(response.Body.Bytes(), &result); err != nil || len(result.Cases) != 1 {
			t.Fatalf("unexpected generation response: %s", response.Body.String())
		}
		var storedScenario, storedReview string
		var revision int
		err := database.DB.QueryRow(`SELECT c.scenario, src.review_id, src.revision_no FROM cases c
			JOIN case_generation_sources src ON src.case_id = c.id AND src.tenant_id = c.tenant_id WHERE c.id = ?`, result.Cases[0].ID).
			Scan(&storedScenario, &storedReview, &revision)
		if err != nil || !strings.Contains(storedScenario, sample.stored) || storedReview != analysis.ID || revision != 1 {
			t.Fatalf("unexpected persisted source or scenario: %q, %q, %d (%v)", storedScenario, storedReview, revision, err)
		}
	}
}

func TestCaseDraftSaveValidatesManualScenarios(t *testing.T) {
	_, handler, editor, _ := caseGenerationFixture(t)
	const casesPath = "/api/cases"
	const invalidChecklist = `{"checklist":["configuration"],"request":{}}`
	create := func(kind, scenario string, status int) *httptest.ResponseRecorder {
		return authRequest(t, handler, "POST", casesPath, "tenant-a", editor,
			mustJSON(map[string]string{"type": kind, "scenario": scenario, "project_id": "project-a", "name": "人工草稿"}), status)
	}
	create("functional", `{"steps":[{"action":"确认默认值"}]}`, http.StatusUnprocessableEntity)
	create("security", invalidChecklist, http.StatusUnprocessableEntity)
	response := create("security", `{"checklist":["configuration"]}`, http.StatusOK)
	var created struct{ ID string }
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil || created.ID == "" {
		t.Fatalf("unexpected response: %s", response.Body.String())
	}
	authRequest(t, handler, "PATCH", casesPath+"/"+created.ID, "tenant-a", editor,
		mustJSON(map[string]string{"scenario": invalidChecklist}), http.StatusUnprocessableEntity)
}
