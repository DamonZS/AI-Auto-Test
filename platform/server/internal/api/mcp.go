package api

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"net/http"
)

const maxGenerateCodeBytes = 256 * 1024

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      any             `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
}

func (s *Server) mcpEndpoint(w http.ResponseWriter, r *http.Request, tenantID string) {
	var request mcpRequest
	if err := decode(r, &request); err != nil || request.JSONRPC != "2.0" {
		mcpError(w, request.ID, -32600, "无效的 JSON-RPC 请求")
		return
	}
	switch request.Method {
	case "initialize":
		mcpWrite(w, request.ID, map[string]any{
			"protocolVersion": "2025-06-18",
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "test-platform", "version": "0.1.0"},
		})
	case "tools/list":
		mcpWrite(w, request.ID, map[string]any{"tools": mcpTools()})
	case "tools/call":
		s.mcpCall(w, request.ID, tenantID, request.Params)
	default:
		mcpError(w, request.ID, -32601, "方法不存在")
	}
}

func mcpTools() []map[string]any {
	return []map[string]any{
		{"name": "list_cases", "description": "列出当前租户的测试用例", "inputSchema": objectSchema(map[string]any{"project_id": map[string]string{"type": "string"}})},
		{"name": "create_test_task", "description": "创建异步测试任务并返回 run_id", "inputSchema": objectSchema(map[string]any{"case_id": map[string]string{"type": "string"}, "version_id": map[string]string{"type": "string"}, "dataset_id": map[string]string{"type": "string"}, "data_row_id": map[string]string{"type": "string"}})},
		{"name": "get_run_status", "description": "查询执行状态和进度", "inputSchema": objectSchema(map[string]any{"run_id": map[string]string{"type": "string"}})},
		{"name": "get_run_report", "description": "查询执行报告", "inputSchema": objectSchema(map[string]any{"run_id": map[string]string{"type": "string"}})},
		{"name": "list_bugs", "description": "列出缺陷", "inputSchema": objectSchema(map[string]any{})},
		{"name": "retest", "description": "按原缺陷关联的用例创建复测任务", "inputSchema": objectSchema(map[string]any{"bug_id": map[string]string{"type": "string"}})},
		{"name": "generate_test_cases_from_code", "description": "提交代码测试用例草稿占位请求，等待人工审核", "inputSchema": objectSchemaWithRequired(map[string]any{
			"project_id":   map[string]any{"type": "string", "minLength": 1},
			"language":     map[string]any{"type": "string", "minLength": 1},
			"code":         map[string]any{"type": "string", "minLength": 1, "maxLength": maxGenerateCodeBytes},
			"requirements": map[string]any{"type": "string", "minLength": 1},
			"test_types":   map[string]any{"type": "array", "minItems": 1, "items": map[string]any{"type": "string", "minLength": 1}},
		}, []string{"project_id", "language", "code", "requirements", "test_types"})},
	}
}

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties}
}

func objectSchemaWithRequired(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func (s *Server) mcpCall(w http.ResponseWriter, id any, tid string, raw json.RawMessage) {
	var params struct {
		Name      string         `json:"name"`
		Arguments map[string]any `json:"arguments"`
	}
	if err := json.Unmarshal(raw, &params); err != nil || params.Name == "" {
		mcpError(w, id, -32602, "tools/call 参数无效")
		return
	}
	var result any
	var err error
	switch params.Name {
	case "list_cases":
		result, err = s.mcpListCases(tid, params.Arguments)
	case "create_test_task":
		result, err = s.mcpCreateTask(tid, params.Arguments)
	case "get_run_status", "get_run_report":
		result, err = s.mcpGetRun(tid, params.Arguments, params.Name == "get_run_report")
	case "list_bugs":
		result, err = s.mcpListBugs(tid)
	case "retest":
		result, err = s.mcpRetest(tid, params.Arguments)
	case "generate_test_cases_from_code":
		result, err = s.mcpGenerateTestCasesFromCode(tid, params.Arguments)
	default:
		mcpError(w, id, -32601, "工具不存在")
		return
	}
	if err != nil {
		mcpError(w, id, -32000, err.Error())
		return
	}
	mcpWrite(w, id, map[string]any{"content": []map[string]any{{"type": "text", "text": mustJSONString(result)}}, "structuredContent": result})
}

func validateGenerateTestCasesArgs(args map[string]any) error {
	projectID, ok := args["project_id"].(string)
	if !ok || projectID == "" {
		return errMCP("project_id 必填且必须是非空字符串")
	}
	language, ok := args["language"].(string)
	if !ok || language == "" {
		return errMCP("language 必填且必须是非空字符串")
	}
	code, ok := args["code"].(string)
	if !ok || code == "" {
		return errMCP("code 必填且必须是非空字符串")
	}
	if len([]byte(code)) > maxGenerateCodeBytes {
		return errMCP(fmt.Sprintf("code 超过 %d 字节限制", maxGenerateCodeBytes))
	}
	requirements, ok := args["requirements"].(string)
	if !ok || requirements == "" {
		return errMCP("requirements 必填且必须是非空字符串")
	}
	testTypes, ok := args["test_types"].([]any)
	if !ok || len(testTypes) == 0 {
		return errMCP("test_types 必填且必须是非空数组")
	}
	for _, value := range testTypes {
		item, ok := value.(string)
		if !ok || item == "" {
			return errMCP("test_types 必须是非空字符串数组")
		}
	}
	return nil
}

func (s *Server) mcpGenerateTestCasesFromCode(tid string, args map[string]any) (any, error) {
	if err := validateGenerateTestCasesArgs(args); err != nil {
		return nil, err
	}
	projectID := args["project_id"].(string)
	var one int
	if err := s.st.DB.QueryRow("SELECT 1 FROM projects WHERE id = ? AND tenant_id = ?", projectID, tid).Scan(&one); err == sql.ErrNoRows {
		return nil, errMCP("项目不存在或不属于当前租户")
	} else if err != nil {
		return nil, err
	}
	s.audit(tid, "", "mcp", "test_cases.generate_from_code.request", projectID)
	return map[string]any{
		"status":        "needs_review",
		"accepted":      false,
		"generated":     false,
		"project_id":    projectID,
		"language":      args["language"],
		"test_types":    args["test_types"],
		"draft":         map[string]any{"status": "needs_review", "cases": []any{}},
		"review_reason": "当前 MCP 入口没有可用的 sampling 双向会话，未生成测试用例草稿",
	}, nil
}

func (s *Server) mcpListCases(tid string, args map[string]any) (any, error) {
	query := "SELECT id, project_id, name, type, scenario, status, version_no FROM cases WHERE tenant_id = ?"
	values := []any{tid}
	if projectID, ok := args["project_id"].(string); ok && projectID != "" {
		query += " AND project_id = ?"
		values = append(values, projectID)
	}
	rows, err := s.st.DB.Query(query+" ORDER BY updated_at DESC", values...)
	if err != nil {
		return nil, err
	}
	return rowsToMaps(rows)
}

func (s *Server) mcpCreateTask(tid string, args map[string]any) (any, error) {
	caseID, _ := args["case_id"].(string)
	if caseID == "" {
		return nil, errMCP("case_id 必填")
	}
	var projectID, datasetID, rowID string
	err := s.st.DB.QueryRow("SELECT project_id, dataset_id, data_row_id FROM cases WHERE id = ? AND tenant_id = ?", caseID, tid).Scan(&projectID, &datasetID, &rowID)
	if err == sql.ErrNoRows {
		return nil, errMCP("用例不存在")
	}
	if value, ok := args["dataset_id"].(string); ok && value != "" {
		datasetID = value
	}
	if value, ok := args["data_row_id"].(string); ok && value != "" {
		rowID = value
	}
	if err := s.validateDataBinding(tid, projectID, datasetID, rowID); err != nil {
		return nil, err
	}
	runID := id()
	versionID, _ := args["version_id"].(string)
	if _, err := s.st.DB.Exec(`INSERT INTO runs (id, tenant_id, case_id, version_id, dataset_id, data_row_id)
		VALUES (?,?,?,?,?,?)`, runID, tid, caseID, versionID, datasetID, rowID); err != nil {
		return nil, err
	}
	s.audit(tid, "mcp", "mcp", "run.create", runID)
	return map[string]string{"run_id": runID, "status": "queued"}, nil
}

func (s *Server) mcpGetRun(tid string, args map[string]any, report bool) (any, error) {
	runID, _ := args["run_id"].(string)
	if runID == "" {
		return nil, errMCP("run_id 必填")
	}
	rows, err := s.st.DB.Query(`SELECT r.id, r.case_id, r.status, r.passed, r.needs_review,
		r.self_healed, r.token_in, r.token_out, r.duration_ms, r.report, r.started_at, r.ended_at,
		c.name AS case_name FROM runs r JOIN cases c ON c.id = r.case_id
		WHERE r.id = ? AND r.tenant_id = ?`, runID, tid)
	if err != nil {
		return nil, err
	}
	items, err := rowsToMaps(rows)
	if err != nil || len(items) == 0 {
		return nil, errMCP("执行记录不存在")
	}
	if !report {
		delete(items[0], "report")
	}
	return items[0], nil
}

func (s *Server) mcpListBugs(tid string) (any, error) {
	rows, err := s.st.DB.Query(`SELECT bug_code, title, severity, priority, status, run_id, reopen_count, updated_at
		FROM bugs WHERE tenant_id = ? ORDER BY updated_at DESC`, tid)
	if err != nil {
		return nil, err
	}
	return rowsToMaps(rows)
}

func (s *Server) mcpRetest(tid string, args map[string]any) (any, error) {
	bugID, _ := args["bug_id"].(string)
	if bugID == "" {
		return nil, errMCP("bug_id 必填")
	}
	var caseID string
	err := s.st.DB.QueryRow(`SELECT r.case_id FROM bugs b JOIN runs r ON r.id = b.run_id
		WHERE b.id = ? AND b.tenant_id = ?`, bugID, tid).Scan(&caseID)
	if err == sql.ErrNoRows {
		return nil, errMCP("缺陷没有可复测的执行关联")
	}
	return s.mcpCreateTask(tid, map[string]any{"case_id": caseID})
}

func mustJSONString(value any) string {
	data, _ := json.Marshal(value)
	return string(data)
}

func errMCP(message string) error { return &mcpErrorValue{message: message} }

type mcpErrorValue struct{ message string }

func (e *mcpErrorValue) Error() string { return e.message }

func mcpWrite(w http.ResponseWriter, id any, result any) {
	write(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "result": result})
}

func mcpError(w http.ResponseWriter, id any, code int, message string) {
	write(w, http.StatusOK, map[string]any{"jsonrpc": "2.0", "id": id, "error": map[string]any{"code": code, "message": message}})
}
