package api

import (
	"strings"
	"testing"
)

func TestBuildMCPSamplingRequestKeepsTenantAndBoundsSource(t *testing.T) {
	input := mcpSamplingInput{
		ProjectID:    "project-a",
		Language:     "go",
		Code:         "func Add(a, b int) int { return a + b }",
		Requirements: "Cover normal and boundary values",
		TestTypes:    []string{"unit"},
	}
	request, err := buildMCPSamplingRequest("tenant-a", input)
	if err != nil {
		t.Fatalf("build sampling request: %v", err)
	}
	if request.TenantID != "tenant-a" {
		t.Fatalf("tenant id was not retained: %q", request.TenantID)
	}
	if request.JSONRPC != "2.0" || request.Method != "sampling/createMessage" {
		t.Fatalf("unexpected sampling request envelope: %#v", request)
	}
	if len(request.Params.Messages) != 1 || !strings.Contains(request.Params.Messages[0].Content[0].Text, input.Code) {
		t.Fatal("sampling prompt does not contain the submitted code")
	}
	if strings.Contains(mustJSONString(request), `"tenant_id"`) {
		t.Fatal("tenant id must remain internal and must not be sent as a non-standard sampling parameter")
	}
}

func TestBuildMCPSamplingRequestRejectsMissingTenantAndOversizedCode(t *testing.T) {
	input := mcpSamplingInput{
		ProjectID:    "project-a",
		Language:     "go",
		Code:         "code",
		Requirements: "requirements",
		TestTypes:    []string{"unit"},
	}
	if _, err := buildMCPSamplingRequest("", input); err == nil {
		t.Fatal("missing tenant id was accepted")
	}
	input.Code = strings.Repeat("x", maxGenerateCodeBytes+1)
	if _, err := buildMCPSamplingRequest("tenant-a", input); err == nil {
		t.Fatal("code exceeding the existing byte limit was accepted")
	}
}

func TestMCPTestCaseDraftFromSamplingResponse(t *testing.T) {
	response := mcpSamplingCreateMessageResult{
		Model: "coding-model",
		Role:  "assistant",
		Content: []mcpSamplingContent{{
			Type: "text",
			Text: `[{"name":"adds positive values","type":"unit","scenario":{"given":"2 and 3","when":"Add","then":"5"}}]`,
		}},
	}
	draft, err := mcpTestCaseDraftFromSamplingResponse("tenant-a", "project-a", response)
	if err != nil {
		t.Fatalf("parse sampling response: %v", err)
	}
	if draft.TenantID != "tenant-a" || draft.ProjectID != "project-a" || draft.Status != "needs_review" {
		t.Fatalf("draft lost tenant/project/review state: %#v", draft)
	}
	if len(draft.Cases) != 1 || draft.Cases[0].Name != "adds positive values" || draft.Cases[0].Type != "unit" {
		t.Fatalf("unexpected generated draft cases: %#v", draft.Cases)
	}
	if strings.Contains(mustJSONString(draft), "func Add") {
		t.Fatal("submitted source code leaked into the generated draft")
	}
}

func TestMCPTestCaseDraftRejectsInvalidSamplingText(t *testing.T) {
	for _, text := range []string{"", "not-json", "[]"} {
		_, err := mcpTestCaseDraftFromSamplingResponse("tenant-a", "project-a", mcpSamplingCreateMessageResult{
			Role:    "assistant",
			Content: []mcpSamplingContent{{Type: "text", Text: text}},
		})
		if err == nil {
			t.Fatalf("invalid model text %q was accepted", text)
		}
	}
}
