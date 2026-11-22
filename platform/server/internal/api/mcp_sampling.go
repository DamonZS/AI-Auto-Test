package api

import (
	"encoding/json"
	"fmt"
	"strings"
)

const mcpSamplingSourcePersistence = "none"

type mcpSamplingInput struct {
	ProjectID    string
	Language     string
	Code         string
	Requirements string
	TestTypes    []string
}

type mcpSamplingRequest struct {
	JSONRPC  string            `json:"jsonrpc"`
	ID       string            `json:"id"`
	Method   string            `json:"method"`
	Params   mcpSamplingParams `json:"params"`
	TenantID string            `json:"-"`
}

type mcpSamplingParams struct {
	Messages  []mcpSamplingMessage `json:"messages"`
	MaxTokens int                  `json:"maxTokens"`
}

type mcpSamplingMessage struct {
	Role    string               `json:"role"`
	Content []mcpSamplingContent `json:"content"`
}

type mcpSamplingContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}

type mcpSamplingCreateMessageResult struct {
	Model      string               `json:"model"`
	Role       string               `json:"role"`
	Content    []mcpSamplingContent `json:"content"`
	StopReason string               `json:"stopReason"`
}

type mcpTestCaseDraft struct {
	TenantID  string        `json:"-"`
	ProjectID string        `json:"project_id"`
	Status    string        `json:"status"`
	Cases     []mcpTestCase `json:"cases"`
}

type mcpTestCase struct {
	Name     string          `json:"name"`
	Type     string          `json:"type"`
	Scenario json.RawMessage `json:"scenario"`
}

func buildMCPSamplingRequest(tenantID string, input mcpSamplingInput) (mcpSamplingRequest, error) {
	if tenantID == "" {
		return mcpSamplingRequest{}, errMCP("tenant_id 必填")
	}
	if input.ProjectID == "" || input.Language == "" || input.Code == "" || input.Requirements == "" || len(input.TestTypes) == 0 {
		return mcpSamplingRequest{}, errMCP("sampling 输入缺少必填字段")
	}
	if len([]byte(input.Code)) > maxGenerateCodeBytes {
		return mcpSamplingRequest{}, errMCP(fmt.Sprintf("code 超过 %d 字节限制", maxGenerateCodeBytes))
	}
	for _, testType := range input.TestTypes {
		if testType == "" {
			return mcpSamplingRequest{}, errMCP("test_types 必须是非空字符串数组")
		}
	}

	prompt := fmt.Sprintf("Generate test case drafts from the supplied source. Return only a JSON array, with each item containing name, type, and scenario as a JSON object. Do not repeat source code in the response.\nProject: %s\nLanguage: %s\nRequirements: %s\nTest types: %s\nSource code follows:\n%s", input.ProjectID, input.Language, input.Requirements, strings.Join(input.TestTypes, ", "), input.Code)
	return mcpSamplingRequest{
		JSONRPC:  "2.0",
		ID:       "generate-test-cases",
		Method:   "sampling/createMessage",
		TenantID: tenantID,
		Params: mcpSamplingParams{
			Messages:  []mcpSamplingMessage{{Role: "user", Content: []mcpSamplingContent{{Type: "text", Text: prompt}}}},
			MaxTokens: 4096,
		},
	}, nil
}

func mcpTestCaseDraftFromSamplingResponse(tenantID, projectID string, response mcpSamplingCreateMessageResult) (mcpTestCaseDraft, error) {
	if tenantID == "" || projectID == "" || response.Role != "assistant" {
		return mcpTestCaseDraft{}, errMCP("sampling 响应或租户上下文无效")
	}
	var text string
	for _, content := range response.Content {
		if content.Type == "text" && strings.TrimSpace(content.Text) != "" {
			text = strings.TrimSpace(content.Text)
			break
		}
	}
	if text == "" {
		return mcpTestCaseDraft{}, errMCP("sampling 响应不包含模型文本")
	}
	var cases []mcpTestCase
	if err := json.Unmarshal([]byte(text), &cases); err != nil || len(cases) == 0 {
		return mcpTestCaseDraft{}, errMCP("sampling 文本必须是非空测试用例 JSON 数组")
	}
	for _, testCase := range cases {
		if testCase.Name == "" || testCase.Type == "" || len(testCase.Scenario) == 0 || !json.Valid(testCase.Scenario) || testCase.Scenario[0] != '{' {
			return mcpTestCaseDraft{}, errMCP("sampling 生成的用例缺少有效的 name、type 或 scenario 对象")
		}
	}
	return mcpTestCaseDraft{TenantID: tenantID, ProjectID: projectID, Status: "needs_review", Cases: cases}, nil
}
