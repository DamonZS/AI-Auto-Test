package api

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPerfCaseRunsThroughQueueAndFinishHandlers(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	_, err := st.DB.Exec("UPDATE runs SET status = 'passed' WHERE id IN ('run-a', 'run-b')")
	if err != nil {
		t.Fatal(err)
	}
	s := New(st)
	h := s.Routes()
	caseID, runID := createPerfCaseAndRun(t, h, "tenant-a", "project-a")

	response := serveAgent(s, "/api/runs/heartbeat", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("heartbeat status = %d, body = %s", response.Code, response.Body.String())
	}
	var claimed struct {
		Run map[string]any `json:"run"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &claimed); err != nil {
		t.Fatal(err)
	}
	if claimed.Run["run_id"] != runID || claimed.Run["case_id"] != caseID || claimed.Run["type"] != "perf" || claimed.Run["base_url"] != "https://a.test" {
		t.Fatalf("worker received incompatible performance task: %#v", claimed.Run)
	}
	var scenario map[string]any
	if err := json.Unmarshal([]byte(claimed.Run["scenario"].(string)), &scenario); err != nil {
		t.Fatalf("worker scenario is not JSON: %v", err)
	}
	request := scenario["request"].(map[string]any)
	if request["method"] != "GET" || request["path"] != "/health" {
		t.Fatalf("worker received unexpected scenario: %#v", scenario)
	}

	result := []byte(`{"status":"passed","passed":true,"needs_review":false,"duration_ms":1250,"report":"完成 10 个请求，失败 0 个。P95 12.5ms","fallback_reason":"Laya 请求失败：DecisionProviderError"}`)
	response = serveAgent(s, "/api/runs/"+runID+"/finish", "tenant-a", result)
	if response.Code != http.StatusOK {
		t.Fatalf("finish status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serveTenant(h, http.MethodGet, "/api/runs/"+runID, "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("get run status = %d, body = %s", response.Code, response.Body.String())
	}
	var finished map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &finished); err != nil {
		t.Fatal(err)
	}
	if finished["status"] != "passed" || finished["report"] != "完成 10 个请求，失败 0 个。P95 12.5ms" || finished["fallback_reason"] != "Laya 请求失败：DecisionProviderError" {
		t.Fatalf("unexpected finished run: %#v", finished)
	}
}

func TestPerfQueueAndRunLookupAreTenantScoped(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	_, err := st.DB.Exec("UPDATE runs SET status = 'passed' WHERE id IN ('run-a', 'run-b')")
	if err != nil {
		t.Fatal(err)
	}
	s := New(st)
	h := s.Routes()
	_, runA := createPerfCaseAndRun(t, h, "tenant-a", "project-a")
	_, runB := createPerfCaseAndRun(t, h, "tenant-b", "project-b")

	response := serveAgent(s, "/api/runs/heartbeat", "tenant-a", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("tenant-a heartbeat status = %d, body = %s", response.Code, response.Body.String())
	}
	var claimedA struct {
		Run map[string]any `json:"run"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &claimedA); err != nil {
		t.Fatal(err)
	}
	if claimedA.Run["run_id"] != runA || claimedA.Run["type"] != "perf" {
		t.Fatalf("tenant-a claimed unexpected run: %#v", claimedA.Run)
	}

	response = serveAgent(s, "/api/runs/heartbeat", "tenant-b", nil)
	if response.Code != http.StatusOK {
		t.Fatalf("tenant-b heartbeat status = %d, body = %s", response.Code, response.Body.String())
	}
	var claimedB struct {
		Run map[string]any `json:"run"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &claimedB); err != nil {
		t.Fatal(err)
	}
	if claimedB.Run["run_id"] != runB || claimedB.Run["type"] != "perf" {
		t.Fatalf("tenant-b claimed unexpected run: %#v", claimedB.Run)
	}

	response = serveTenant(h, http.MethodGet, "/api/runs/"+runB, "tenant-a", nil)
	if response.Code != http.StatusNotFound {
		t.Fatalf("cross-tenant run lookup status = %d, want 404, body = %s", response.Code, response.Body.String())
	}
}

func createPerfCaseAndRun(t *testing.T, h http.Handler, tenantID, projectID string) (string, string) {
	t.Helper()
	body := []byte(`{"project_id":"` + projectID + `","name":"health endpoint perf","type":"perf","scenario":"{\"request\":{\"method\":\"GET\",\"path\":\"/health\"},\"concurrency\":2,\"duration_seconds\":3,\"max_requests\":10,\"timeout\":1}"}`)
	response := serveTenant(h, http.MethodPost, "/api/cases", tenantID, body)
	if response.Code != http.StatusOK {
		t.Fatalf("create performance case status = %d, body = %s", response.Code, response.Body.String())
	}
	var createdCase map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &createdCase); err != nil {
		t.Fatal(err)
	}

	response = serveTenant(h, http.MethodPost, "/api/cases/"+createdCase["id"]+"/run", tenantID, []byte(`{}`))
	if response.Code != http.StatusOK {
		t.Fatalf("queue performance run status = %d, body = %s", response.Code, response.Body.String())
	}
	var createdRun map[string]string
	if err := json.Unmarshal(response.Body.Bytes(), &createdRun); err != nil {
		t.Fatal(err)
	}
	return createdCase["id"], createdRun["run_id"]
}

func serveTenant(h http.Handler, method, path, tenantID string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(method, path, bytes.NewReader(body))
	req.Header.Set(tenantHeader, tenantID)
	response := httptest.NewRecorder()
	h.ServeHTTP(response, req)
	return response
}

func serveAgent(s *Server, path, tenantID string, body []byte) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("X-Agent-Token", "tenant:"+tenantID)
	response := httptest.NewRecorder()
	next := s.heartbeat
	if strings.HasSuffix(path, "/finish") {
		parts := strings.Split(path, "/")
		req.SetPathValue("id", parts[len(parts)-2])
		next = s.finishRun
	}
	s.agent(next).ServeHTTP(response, req)
	return response
}
