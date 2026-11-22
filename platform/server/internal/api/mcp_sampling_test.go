package api

import (
	"bufio"
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestMCPSamplingHTTPDraftReviewAndRun(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	if _, err := st.DB.Exec(`UPDATE runs SET status = 'passed'`); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(New(st).Routes())
	t.Cleanup(server.Close)
	const source = `func sourceOnlyMarker() { return }`
	const privateModel = "private-model-response-marker"
	session := initializeSamplingHTTP(t, server, "tenant-a", true)
	stream := samplingHTTP(t, server, "tenant-a", session, generationCall(source))
	t.Cleanup(func() { _ = stream.Body.Close() })
	if stream.StatusCode != http.StatusOK || !strings.HasPrefix(stream.Header.Get("Content-Type"), "text/event-stream") {
		t.Fatalf("expected tool POST SSE, got HTTP %d %q", stream.StatusCode, stream.Header.Get("Content-Type"))
	}
	scanner := bufio.NewScanner(stream.Body)
	scanner.Buffer(make([]byte, 4096), 2*1024*1024)
	challenge := readSamplingEvent(t, scanner)
	params := challenge["params"].(map[string]any)
	message := params["messages"].([]any)[0].(map[string]any)
	content := message["content"].(map[string]any)
	if challenge["method"] != "sampling/createMessage" || content["type"] != "text" ||
		strings.Count(content["text"].(string), source) != 1 || strings.Contains(mustJSONString(challenge), "tenant-a") {
		t.Fatal("sampling event has the wrong protocol shape or disclosed tenant metadata")
	}
	answer := map[string]any{"jsonrpc": "2.0", "id": challenge["id"], "result": map[string]any{
		"model": privateModel, "role": "assistant", "stopReason": "endTurn",
		"content": map[string]any{"type": "text", "text": `[
			{"name":"Health status","type":"api","scenario":{"request":{"method":"GET","path":"/health"},"assertions":[{"type":"status_code","expected":200}]}},
			{"name":"Homepage title","type":"ui","scenario":{"steps":[{"action":"assert_text","text":"Welcome"}]}}
		]`},
	}}
	assertSamplingAck(t, samplingHTTP(t, server, "tenant-b", session, answer), http.StatusNotFound)
	assertSamplingAck(t, samplingRequestHTTP(t, server, "/mcp", "tenant-a", session, answer, "Bearer different-mcp-credential"), http.StatusNotFound)
	otherSession := initializeSamplingHTTP(t, server, "tenant-a", true)
	assertSamplingAck(t, samplingHTTP(t, server, "tenant-a", otherSession, answer), http.StatusBadRequest)
	answer["id"] = "wrong-id"
	assertSamplingAck(t, samplingHTTP(t, server, "tenant-a", session, answer), http.StatusBadRequest)
	answer["id"] = challenge["id"]
	assertSamplingAck(t, samplingHTTP(t, server, "tenant-a", session, answer), http.StatusAccepted)
	assertSamplingAck(t, samplingHTTP(t, server, "tenant-a", session, answer), http.StatusBadRequest)
	final := readSamplingEvent(t, scanner)
	if final["id"] != "generate" || strings.Contains(mustJSONString(final), source) || strings.Contains(mustJSONString(final), privateModel) {
		t.Fatalf("final result wrong or echoed input: %#v", final)
	}
	if scanner.Scan() || scanner.Err() != nil {
		t.Fatalf("stream did not end cleanly: %v", scanner.Err())
	}
	generated := final["result"].(map[string]any)["structuredContent"].(map[string]any)
	if generated["status"] != "needs_review" || generated["generated"] != true {
		t.Fatalf("expected generated review drafts: %#v", generated)
	}
	cases := generated["draft"].(map[string]any)["cases"].([]any)
	if len(cases) != 2 {
		t.Fatalf("expected API and UI drafts: %#v", cases)
	}
	for index, entry := range cases {
		draft := entry.(map[string]any)
		caseID := draft["id"].(string)
		call := func(name string, args map[string]any) map[string]any {
			return readSamplingJSON(t, samplingHTTP(t, server, "tenant-a", session, map[string]any{
				"jsonrpc": "2.0", "id": name, "method": "tools/call", "params": map[string]any{"name": name, "arguments": args},
			}), http.StatusOK)
		}
		var scenario, revision, mode, state string
		if err := st.DB.QueryRow(`SELECT c.scenario, v.scenario, c.generation_mode, c.status FROM cases c
			JOIN case_revisions v ON v.case_id = c.id AND v.tenant_id = c.tenant_id AND v.version_no = 1
			WHERE c.id = ? AND c.tenant_id = 'tenant-a'`, caseID).Scan(&scenario, &revision, &mode, &state); err != nil {
			t.Fatal(err)
		}
		if scenario != revision || mode != "sampling" || state != "draft" || strings.Contains(scenario, source) || strings.Contains(scenario, privateModel) {
			t.Fatalf("wrong draft metadata or scenario %q (%s %s)", scenario, mode, state)
		}
		if reply := call("create_test_task", map[string]any{"case_id": caseID}); reply["error"] == nil {
			t.Fatal("MCP queued an unapproved draft")
		}
		assertSamplingAck(t, samplingRequestHTTP(t, server, "/api/cases/"+caseID+"/review", "tenant-a", "", map[string]string{"decision": "submit_review"}), http.StatusOK)
		if reply := call("create_test_task", map[string]any{"case_id": caseID}); reply["error"] == nil {
			t.Fatal("MCP queued a draft still in review")
		}
		assertSamplingAck(t, samplingRequestHTTP(t, server, "/api/cases/"+caseID+"/review", "tenant-a", "", map[string]string{"decision": "approve", "reviewer": "human"}), http.StatusOK)
		queued := call("create_test_task", map[string]any{"case_id": caseID})
		runID := queued["result"].(map[string]any)["structuredContent"].(map[string]any)["run_id"].(string)
		claimed := readSamplingJSON(t, samplingRequestHTTP(t, server, "/api/runs/heartbeat", "tenant-a", "", nil), http.StatusOK)["run"].(map[string]any)
		if claimed["run_id"] != runID || claimed["scenario"] != scenario || claimed["type"] != []string{"api", "ui"}[index] {
			t.Fatalf("existing worker received wrong scenario: %#v", claimed)
		}
		passed := call("get_run_status", map[string]any{"run_id": runID})
		if result := passed["result"].(map[string]any)["structuredContent"].(map[string]any); result["status"] != "running" || result["report"] != nil {
			t.Fatalf("existing status poll changed: %#v", result)
		}
		assertSamplingAck(t, samplingRequestHTTP(t, server, "/api/runs/"+runID+"/finish", "tenant-a", "", map[string]any{"status": "failed", "report": "Functional assertion failed"}), http.StatusOK)
		reported := call("get_run_report", map[string]any{"run_id": runID})
		if result := reported["result"].(map[string]any)["structuredContent"].(map[string]any); result["report"] != "Functional assertion failed" || result["repair_hint"] == nil {
			t.Fatalf("failed run lost evidence or hint: %#v", result)
		}
	}
	var audits int
	if err := st.DB.QueryRow(`SELECT count(*) FROM audit_logs WHERE tenant_id = 'tenant-a'
		AND channel = 'mcp' AND action = 'test_cases.generate_from_code.draft'
		AND actor <> '' AND instr(target, ?) = 0 AND instr(target, ?) = 0`, source, privateModel).Scan(&audits); err != nil || audits != 2 {
		t.Fatalf("expected two safe draft audits, count=%d err=%v", audits, err)
	}
}

func TestMCPSamplingHTTPRejectsUnsupportedClientAndSourceEcho(t *testing.T) {
	st := openTestStore(t)
	seedMCPData(t, st)
	server := httptest.NewServer(New(st).Routes())
	t.Cleanup(server.Close)
	const source = "privateSourceMarker"
	unsupported := initializeSamplingHTTP(t, server, "tenant-a", false)
	assertSamplingToolError(t, readSamplingJSON(t, samplingHTTP(t, server, "tenant-a", unsupported, generationCall(source)), http.StatusOK), source)
	session := initializeSamplingHTTP(t, server, "tenant-a", true)
	assertSamplingToolError(t, readSamplingJSON(t, samplingHTTP(t, server, "tenant-b", initializeSamplingHTTP(t, server, "tenant-b", true), generationCall(source)), http.StatusOK), source)
	assertSamplingToolError(t, readSamplingJSON(t, samplingHTTP(t, server, "tenant-a", session, generationCall(strings.Repeat("x", maxGenerateCodeBytes+1))), http.StatusOK), source)
	for _, sample := range []string{
		`[{"name":"privateSourceMarker","type":"api","scenario":{"request":{"method":"GET","path":"/health"},"assertions":[{"type":"status_code","expected":200}]}}]`,
		`[{"name":"Probe","type":"api","scenario":{"request":{"method":"POST","path":"/health"},"assertions":[{"type":"status_code","expected":200}]}}]`,
		`[{"name":"External","type":"api","scenario":{"request":{"method":"GET","path":"//external.invalid/"},"assertions":[{"type":"status_code","expected":200}]}}]`,
		"",
	} {
		stream := samplingHTTP(t, server, "tenant-a", session, generationCall(source))
		t.Cleanup(func() { _ = stream.Body.Close() })
		scanner := bufio.NewScanner(stream.Body)
		challenge := readSamplingEvent(t, scanner)
		answer := map[string]any{"jsonrpc": "2.0", "id": challenge["id"], "result": map[string]any{
			"role": "assistant", "model": "test-model", "content": map[string]any{"type": "text", "text": sample},
		}}
		if sample == "" {
			delete(answer, "result")
			answer["error"] = map[string]any{"code": -32000, "message": source}
		}
		assertSamplingAck(t, samplingHTTP(t, server, "tenant-a", session, answer), http.StatusAccepted)
		assertSamplingToolError(t, readSamplingEvent(t, scanner), source)
		_ = stream.Body.Close()
	}
	var drafts int
	if err := st.DB.QueryRow(`SELECT count(*) FROM cases WHERE generation_mode = 'sampling'`).Scan(&drafts); err != nil || drafts != 0 {
		t.Fatalf("unsupported or invalid generation persisted drafts, count=%d err=%v", drafts, err)
	}
}

func generationCall(source string) map[string]any {
	return map[string]any{"jsonrpc": "2.0", "id": "generate", "method": "tools/call", "params": map[string]any{
		"name": "generate_test_cases_from_code", "arguments": map[string]any{
			"project_id": "project-a", "language": "go", "code": source, "requirements": "Verify the health endpoint and welcome page", "test_types": []string{"api", "ui"},
		},
	}}
}

func initializeSamplingHTTP(t *testing.T, server *httptest.Server, tenant string, capable bool) string {
	t.Helper()
	capabilities := map[string]any{}
	if capable {
		capabilities["sampling"] = map[string]any{}
	}
	response := samplingHTTP(t, server, tenant, "", map[string]any{
		"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"protocolVersion": "2025-06-18", "capabilities": capabilities},
	})
	session := response.Header.Get("Mcp-Session-Id")
	readSamplingJSON(t, response, http.StatusOK)
	if session == "" {
		t.Fatal("initialize did not issue a session ID")
	}
	assertSamplingAck(t, samplingHTTP(t, server, tenant, session, map[string]any{
		"jsonrpc": "2.0", "method": "notifications/initialized",
	}), http.StatusAccepted)
	return session
}

func samplingHTTP(t *testing.T, server *httptest.Server, tenant, session string, body any) *http.Response {
	t.Helper()
	return samplingRequestHTTP(t, server, "/mcp", tenant, session, body)
}

func TestMCPSessionIsBoundToIssuedCredential(t *testing.T) {
	database, _ := authFixture(t)
	if err := ProvisionUser(t.Context(), database, "sampling-owner", fixturePassword, "alpha", "", "owner"); err != nil {
		t.Fatal(err)
	}
	handler := New(database, AuthOptions{}).Routes()
	owner := authLogin(t, handler, "sampling-owner")
	issue := func() string {
		response := authRequest(t, handler, "POST", "/api/auth/service-tokens", "alpha", owner, `{"scope":"mcp"}`, http.StatusCreated)
		var issued struct{ Token string }
		if err := json.Unmarshal(response.Body.Bytes(), &issued); err != nil || issued.Token == "" {
			t.Fatal("missing MCP credential")
		}
		return "Bearer " + issued.Token
	}
	first, second := issue(), issue()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	initialize := map[string]any{"jsonrpc": "2.0", "id": 1, "method": "initialize", "params": map[string]any{"capabilities": map[string]any{"sampling": map[string]any{}}}}
	response := samplingRequestHTTP(t, server, "/mcp", "alpha", "", initialize, first)
	session := response.Header.Get("Mcp-Session-Id")
	readSamplingJSON(t, response, http.StatusOK)
	if session == "" {
		t.Fatal("missing MCP session")
	}
	list := map[string]any{"jsonrpc": "2.0", "id": 2, "method": "tools/list"}
	readSamplingJSON(t, samplingRequestHTTP(t, server, "/mcp", "alpha", session, list, second), http.StatusNotFound)
	readSamplingJSON(t, samplingRequestHTTP(t, server, "/mcp", "alpha", session, list, first), http.StatusOK)
}

func samplingRequestHTTP(t *testing.T, server *httptest.Server, path, tenant, session string, body any, authorization ...string) *http.Response {
	t.Helper()
	request, err := http.NewRequest(http.MethodPost, server.URL+path, bytes.NewBufferString(mustJSONString(body)))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("X-Agent-Token", "tenant:"+tenant)
	request.Header.Set("X-Tenant-Id", tenant)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("Accept", "application/json, text/event-stream")
	if len(authorization) > 0 {
		request.Header.Set("Authorization", authorization[0])
	}
	if session != "" {
		request.Header.Set("Mcp-Session-Id", session)
		request.Header.Set("MCP-Protocol-Version", "2025-06-18")
	}
	client := *server.Client()
	client.Timeout = 5 * time.Second
	response, err := client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	return response
}

func assertSamplingAck(t *testing.T, response *http.Response, status int) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil || response.StatusCode != status || (status == http.StatusAccepted && len(body) != 0) {
		t.Fatalf("expected HTTP %d, got HTTP %d %q err=%v", status, response.StatusCode, body, err)
	}
}

func readSamplingJSON(t *testing.T, response *http.Response, status int) map[string]any {
	t.Helper()
	defer response.Body.Close()
	var result map[string]any
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil || response.StatusCode != status {
		t.Fatalf("expected JSON HTTP %d, got HTTP %d %#v err=%v", status, response.StatusCode, result, err)
	}
	return result
}

func readSamplingEvent(t *testing.T, scanner *bufio.Scanner) map[string]any {
	t.Helper()
	for scanner.Scan() {
		if payload, ok := strings.CutPrefix(scanner.Text(), "data: "); ok {
			var event map[string]any
			if err := json.Unmarshal([]byte(payload), &event); err != nil || !scanner.Scan() || scanner.Text() != "" {
				t.Fatalf("invalid SSE event: %v", err)
			}
			return event
		}
	}
	t.Fatalf("missing SSE event: %v", scanner.Err())
	return nil
}

func assertSamplingToolError(t *testing.T, reply map[string]any, source string) {
	t.Helper()
	result, _ := reply["result"].(map[string]any)
	if result["isError"] != true || strings.Contains(mustJSONString(reply), source) {
		t.Fatalf("wrong error or source disclosed: %#v", reply)
	}
}
