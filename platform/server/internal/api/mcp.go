package api

import (
	"database/sql"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const maxGenerateCodeBytes = 256 * 1024

type mcpRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      json.RawMessage `json:"id"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params"`
	Result  json.RawMessage `json:"result"`
	Error   json.RawMessage `json:"error"`
}

func (s *Server) mcpEndpoint(w http.ResponseWriter, r *http.Request, tenantID string) {
	for _, origin := range r.Header.Values("Origin") {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") ||
			!strings.EqualFold(parsed.Host, r.Host) || parsed.User != nil || parsed.RequestURI() != "/" || parsed.Fragment != "" {
			fail(w, http.StatusForbidden, "MCP Origin 不受信任")
			return
		}
	}
	identity := mcpIdentityFromRequest(r, tenantID)
	session := s.findMCPSession(w, r.Header.Get("Mcp-Session-Id"), identity)
	if r.Header.Get("Mcp-Session-Id") != "" && session == nil {
		return
	}
	if version := r.Header.Get("MCP-Protocol-Version"); version != "" && version != mcpProtocolVersion {
		fail(w, http.StatusBadRequest, "不支持的 MCP 协议版本")
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2*1024*1024)
	defer r.Body.Close()
	var request mcpRequest
	var extra json.RawMessage
	decoder := json.NewDecoder(r.Body)
	if err := decoder.Decode(&request); err != nil || request.JSONRPC != "2.0" || decoder.Decode(&extra) != io.EOF {
		mcpError(w, request.ID, -32600, "无效的 JSON-RPC 请求")
		return
	}
	if request.Method == "" && (request.Result != nil || request.Error != nil) {
		acceptMCPSamplingReply(w, session, request)
		return
	}
	switch request.Method {
	case "initialize":
		var params struct {
			Capabilities struct {
				Sampling map[string]any `json:"sampling"`
			} `json:"capabilities"`
		}
		if len(request.Params) > 0 && json.Unmarshal(request.Params, &params) != nil {
			mcpError(w, request.ID, -32602, "initialize 参数无效")
			return
		}
		if !s.openMCPSession(w, identity, params.Capabilities.Sampling != nil) {
			return
		}
		mcpWrite(w, request.ID, map[string]any{
			"protocolVersion": mcpProtocolVersion,
			"capabilities":    map[string]any{"tools": map[string]any{}},
			"serverInfo":      map[string]string{"name": "test-platform", "version": "0.1.0"},
		})
	case "notifications/initialized":
		w.WriteHeader(http.StatusAccepted)
	case "ping":
		mcpWrite(w, request.ID, map[string]any{})
	case "tools/list":
		mcpWrite(w, request.ID, map[string]any{"tools": mcpTools()})
	case "tools/call":
		s.mcpCall(w, r, request, identity, session)
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
		{"name": "generate_test_cases_from_code", "description": "使用客户端 MCP Sampling 生成普通功能测试草稿，等待人工审核", "inputSchema": objectSchemaWithRequired(map[string]any{
			"project_id":   map[string]any{"type": "string", "minLength": 1, "maxLength": 128},
			"language":     map[string]any{"type": "string", "minLength": 1, "maxLength": 64},
			"code":         map[string]any{"type": "string", "minLength": 1, "maxLength": maxGenerateCodeBytes},
			"requirements": map[string]any{"type": "string", "minLength": 1, "maxLength": 4096},
			"test_types":   map[string]any{"type": "array", "minItems": 1, "maxItems": 3, "items": map[string]any{"type": "string", "enum": []string{"api", "ui", "functional"}}},
		}, []string{"project_id", "language", "code", "requirements", "test_types"})},
	}
}

func objectSchema(properties map[string]any) map[string]any {
	return map[string]any{"type": "object", "properties": properties}
}

func objectSchemaWithRequired(properties map[string]any, required []string) map[string]any {
	return map[string]any{"type": "object", "properties": properties, "required": required, "additionalProperties": false}
}

func (s *Server) mcpCall(w http.ResponseWriter, r *http.Request, request mcpRequest, identity mcpIdentity, session *mcpSession) {
	var params struct {
		Name      string          `json:"name"`
		Arguments json.RawMessage `json:"arguments"`
	}
	if err := json.Unmarshal(request.Params, &params); err != nil || params.Name == "" {
		mcpError(w, request.ID, -32602, "tools/call 参数无效")
		return
	}
	if params.Name == "generate_test_cases_from_code" {
		s.mcpGenerateTestCasesFromCode(w, r, request.ID, identity, session, params.Arguments)
		return
	}
	var args map[string]any
	if len(params.Arguments) > 0 && json.Unmarshal(params.Arguments, &args) != nil {
		mcpError(w, request.ID, -32602, "tools/call 参数无效")
		return
	}
	tid := identity.tenantID
	var result any
	var err error
	switch params.Name {
	case "list_cases":
		result, err = s.mcpListCases(tid, args)
	case "create_test_task":
		result, err = s.mcpCreateTask(tid, args)
	case "get_run_status", "get_run_report":
		result, err = s.mcpGetRun(tid, args, params.Name == "get_run_report")
	case "list_bugs":
		result, err = s.mcpListBugs(tid)
	case "retest":
		result, err = s.mcpRetest(tid, args)
	default:
		mcpError(w, request.ID, -32601, "工具不存在")
		return
	}
	if err != nil {
		mcpError(w, request.ID, -32000, err.Error())
		return
	}
	mcpWrite(w, request.ID, mcpToolResult(result))
}

func mcpToolResult(result any) map[string]any {
	return map[string]any{"content": []map[string]any{{"type": "text", "text": mustJSONString(result)}}, "structuredContent": result}
}

func mcpToolError(message string) map[string]any {
	return map[string]any{
		"isError": true,
		"content": []map[string]any{{"type": "text", "text": message}},
	}
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
	err := s.st.DB.QueryRow("SELECT project_id, dataset_id, data_row_id FROM cases WHERE id = ? AND tenant_id = ?", caseID, tid).
		Scan(&projectID, &datasetID, &rowID)
	if err == sql.ErrNoRows {
		return nil, errMCP("用例不存在")
	}
	if err != nil {
		return nil, err
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
	queued, err := s.st.DB.Exec(`INSERT INTO runs (id, tenant_id, case_id, version_id, dataset_id, data_row_id)
		SELECT ?, tenant_id, id, ?, ?, ? FROM cases WHERE id = ? AND tenant_id = ?
		AND type NOT IN ('functional', 'security')
		AND (generation_mode NOT IN ('sampling', 'platform_llm') OR status = 'approved')`, runID, versionID, datasetID, rowID, caseID, tid)
	if err != nil {
		return nil, err
	}
	if count, err := queued.RowsAffected(); err != nil {
		return nil, err
	} else if count == 0 {
		return nil, errMCP("用例不存在或 Sampling 用例尚未人工定稿")
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
	} else {
		switch items[0]["status"] {
		case "failed":
			items[0]["repair_hint"] = "对照报告中的失败断言、已审核的用例和需求预期，定位待修复问题，修复后复测。"
		case "error", "needs_review":
			items[0]["repair_hint"] = "先核对报告中的运行环境、用例和断言，再决定是否修复或复测。"
		}
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
