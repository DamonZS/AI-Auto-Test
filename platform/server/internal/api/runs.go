package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

type performanceResult struct {
	Requests  *int     `json:"requests"`
	Failures  int      `json:"failed_requests"`
	ErrorRate float64  `json:"error_rate"`
	RPS       float64  `json:"rps"`
	P50       *float64 `json:"p50_ms"`
	P95       *float64 `json:"p95_ms"`
	P99       *float64 `json:"p99_ms"`
}

func (s *Server) runCase(w http.ResponseWriter, r *http.Request, tid string) {
	cid := r.PathValue("id")
	var projectID, datasetID, dataRowID, generationMode, caseStatus string
	err := s.st.DB.QueryRow(
		"SELECT project_id, dataset_id, data_row_id, generation_mode, status FROM cases WHERE id = ? AND tenant_id = ?", cid, tid,
	).Scan(&projectID, &datasetID, &dataRowID, &generationMode, &caseStatus)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if (generationMode == "sampling" || generationMode == "platform_llm") && caseStatus != "approved" {
		fail(w, http.StatusConflict, "模型生成的用例须人工定稿后运行")
		return
	}
	var body struct {
		VersionID string `json:"version_id"`
		DatasetID string `json:"dataset_id"`
		DataRowID string `json:"data_row_id"`
	}
	_ = decode(r, &body)
	if body.VersionID != "" {
		var one int
		err := s.st.DB.QueryRowContext(r.Context(), "SELECT 1 FROM versions WHERE id = ? AND tenant_id = ? AND project_id = ?", body.VersionID, tid, projectID).Scan(&one)
		if err == sql.ErrNoRows {
			fail(w, http.StatusBadRequest, "版本不存在或不属于当前项目")
			return
		}
		if err != nil {
			fail(w, http.StatusInternalServerError, "无法验证执行版本")
			return
		}
	}
	if body.DatasetID != "" || body.DataRowID != "" {
		datasetID, dataRowID = body.DatasetID, body.DataRowID
	}
	if err := s.validateDataBinding(tid, projectID, datasetID, dataRowID); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	rid := id()
	if _, err := s.st.DB.Exec(
		"INSERT INTO runs (id, tenant_id, case_id, version_id, dataset_id, data_row_id) VALUES (?,?,?,?,?,?)",
		rid, tid, cid, body.VersionID, datasetID, dataRowID); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "run.create", rid)
	write(w, http.StatusOK, map[string]string{"run_id": rid, "status": "queued"})
}

func (s *Server) listRuns(w http.ResponseWriter, r *http.Request, tid string) {
	rows, _ := s.st.DB.Query(`SELECT r.*, c.name AS case_name, c.type AS case_type
		FROM runs r JOIN cases c ON c.id = r.case_id
		WHERE r.tenant_id = ? ORDER BY r.started_at DESC LIMIT 100`, tid)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) getRun(w http.ResponseWriter, r *http.Request, tid string) {
	rows, err := s.st.DB.Query(`SELECT r.*, c.name AS case_name, c.type AS case_type, c.scenario,
		COALESCE(e.steps_json, '[]') AS steps
		FROM runs r JOIN cases c ON c.id = r.case_id
		LEFT JOIN run_step_evidence e ON e.run_id = r.id AND e.tenant_id = r.tenant_id
		WHERE r.id = ? AND r.tenant_id = ?`, r.PathValue("id"), tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := rowsToMaps(rows)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(out) == 0 {
		fail(w, http.StatusNotFound, "执行记录不存在")
		return
	}
	out[0]["steps"] = json.RawMessage(out[0]["steps"].(string))
	out[0]["performance"] = json.RawMessage(out[0]["performance_json"].(string))
	delete(out[0], "performance_json")
	write(w, http.StatusOK, out[0])
}

// heartbeat 供执行端领取与续租；返回待执行任务（首版按租户取最早 queued）。
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request, tid string) {
	rid, err := s.claimQueuedRun(r.Context(), tid)
	if err == sql.ErrNoRows {
		write(w, http.StatusOK, map[string]any{"run": nil})
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法领取执行任务")
		return
	}
	rows, err := s.st.DB.Query(`SELECT r.id AS run_id, r.case_id, r.version_id, r.dataset_id, r.data_row_id,
		c.name, c.type, c.scenario, c.project_id, COALESCE(e.base_url, p.base_url) AS base_url, COALESCE(tdr.values_json, '{}') AS data_json
		FROM runs r JOIN cases c ON c.id = r.case_id AND c.tenant_id = r.tenant_id
		JOIN projects p ON p.id = c.project_id AND p.tenant_id = c.tenant_id
		LEFT JOIN environments e ON e.id = c.environment_id AND e.project_id = c.project_id AND e.tenant_id = c.tenant_id
		LEFT JOIN test_data_records tdr ON tdr.id = r.data_row_id AND tdr.dataset_id = r.dataset_id AND tdr.tenant_id = r.tenant_id
		WHERE r.id = ? AND r.tenant_id = ?`, rid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取执行任务")
		return
	}
	out, err := rowsToMaps(rows)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取执行任务")
		return
	}
	if len(out) == 0 {
		fail(w, http.StatusNotFound, "执行记录不存在")
		return
	}
	var data map[string]string
	_ = json.Unmarshal([]byte(out[0]["data_json"].(string)), &data)
	delete(out[0], "data_json")
	out[0]["data"] = data
	if r.URL.Query().Get("knowledge") == "requirements" && out[0]["type"] == "ui" {
		projectID := out[0]["project_id"].(string)
		if rows, err := s.st.DB.QueryContext(r.Context(), `SELECT id, title, content FROM requirements
			WHERE tenant_id = ? AND project_id = ? AND status = 'open' ORDER BY id`, tid, projectID); err == nil {
			if requirements, err := rowsToMaps(rows); err == nil {
				out[0]["knowledge_context"] = map[string]any{"tenant_id": tid, "project_id": projectID, "requirements": requirements}
			}
		}
	}
	write(w, http.StatusOK, map[string]any{"run": out[0]})
}

func (s *Server) finishRun(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		performanceResult
		Status         string    `json:"status"`
		Passed         bool      `json:"passed"`
		NeedsReview    bool      `json:"needs_review"`
		SelfHealed     bool      `json:"self_healed"`
		TokenIn        int       `json:"token_in"`
		TokenOut       int       `json:"token_out"`
		DurationMS     int       `json:"duration_ms"`
		Report         string    `json:"report"`
		FallbackReason string    `json:"fallback_reason"`
		Steps          []runStep `json:"steps"`
		FailureSigs    []struct {
			Signature string `json:"signature"`
			Title     string `json:"title"`
			Evidence  string `json:"evidence"`
		} `json:"failures"`
		Patch *struct {
			StepSeq     int    `json:"step_seq"`
			OldSelector string `json:"old_selector"`
			NewSelector string `json:"new_selector"`
		} `json:"patch"`
	}
	if err := decode(r, &b); err != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return
	}
	performance := "null"
	if b.Requests != nil {
		if *b.Requests < 0 || b.Failures < 0 || b.Failures > *b.Requests || b.ErrorRate < 0 || b.ErrorRate > 1 || b.RPS < 0 {
			fail(w, http.StatusBadRequest, "性能指标无效")
			return
		}
		performance = mustJSON(b.performanceResult)
	}
	if !validRunSteps(b.Steps) {
		fail(w, http.StatusBadRequest, "步骤证据无效")
		return
	}
	rid := r.PathValue("id")
	tx, err := s.st.DB.BeginTx(r.Context(), nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	err = tx.QueryRow(`UPDATE runs SET status = ?, passed = ?, needs_review = ?, self_healed = ?,
		token_in = ?, token_out = ?, duration_ms = ?, report = ?, fallback_reason = ?, performance_json = ?, ended_at = datetime('now')
		WHERE id = ? AND tenant_id = ? RETURNING id`,
		b.Status, boolInt(b.Passed), boolInt(b.NeedsReview), boolInt(b.SelfHealed),
		b.TokenIn, b.TokenOut, b.DurationMS, b.Report, b.FallbackReason, performance, rid, tid).Scan(&rid)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "执行记录不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(b.Steps) > 0 {
		if _, err := tx.Exec(`INSERT INTO run_step_evidence (run_id, tenant_id, steps_json) VALUES (?,?,?)
			ON CONFLICT(tenant_id, run_id) DO UPDATE SET steps_json = excluded.steps_json`, rid, tid, mustJSON(b.Steps)); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	if err := tx.Commit(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, f := range b.FailureSigs {
		s.upsertBug(tid, rid, f.Signature, f.Title, f.Evidence)
	}
	if b.Patch != nil {
		_, _ = s.st.DB.Exec(`INSERT INTO heal_patches (id, tenant_id, run_id, step_seq, old_selector, new_selector)
			VALUES (?,?,?,?,?,?)`, id(), tid, rid, b.Patch.StepSeq, b.Patch.OldSelector, b.Patch.NewSelector)
	}
	write(w, http.StatusOK, map[string]string{"id": rid})
}

func boolInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// upsertBug：同签名未关闭则关联，已关闭则重新打开，否则新建。
func (s *Server) upsertBug(tid, runID, signature, title, evidence string) {
	var bid, status string
	err := s.st.DB.QueryRow(`SELECT id, status FROM bugs
		WHERE tenant_id = ? AND signature = ? ORDER BY created_at DESC LIMIT 1`,
		tid, signature).Scan(&bid, &status)
	if err == nil {
		if status == "closed" {
			_, _ = s.st.DB.Exec(`UPDATE bugs SET status = 'reopened', reopen_count = reopen_count + 1,
				updated_at = datetime('now') WHERE id = ?`, bid)
			s.captureEvent(tid, bid, status, "reopened", "agent", "复测失败，自动重新打开")
		}
		return
	}
	var projectID string
	_ = s.st.DB.QueryRow(`SELECT c.project_id FROM runs r JOIN cases c ON c.id = r.case_id
		WHERE r.id = ?`, runID).Scan(&projectID)
	var seq int
	_ = s.st.DB.QueryRow("SELECT COALESCE(max(rowid),0)+1 FROM bugs WHERE tenant_id = ?", tid).Scan(&seq)
	code := "BUG-" + short(tid) + "-" + itoa(seq)
	bid = id()
	_, _ = s.st.DB.Exec(`INSERT INTO bugs
		(id, tenant_id, project_id, bug_code, title, signature, run_id, evidence)
		VALUES (?,?,?,?,?,?,?,?)`, bid, tid, projectID, code, title, signature, runID, evidence)
	s.captureEvent(tid, bid, "", "new", "agent", "执行失败自动创建")
}

func (s *Server) captureEvent(tid, bugID, from, to, actor, note string) {
	_, _ = s.st.DB.Exec(`INSERT INTO bug_events (tenant_id, bug_id, from_status, to_status, actor, note)
		VALUES (?,?,?,?,?,?)`, tid, bugID, from, to, actor, note)
}

func (s *Server) audit(tid, actor, channel, action, target string) {
	_, _ = s.st.DB.Exec(`INSERT INTO audit_logs (tenant_id, actor, channel, action, target)
		VALUES (?,?,?,?,?)`, tid, actor, channel, action, target)
}

func short(s string) string {
	if len(s) > 4 {
		return s[:4]
	}
	return s
}

func itoa(i int) string { return fmtInt(i) }

func fmtInt(i int) string {
	if i == 0 {
		return "0"
	}
	var b [20]byte
	p := len(b)
	for i > 0 {
		p--
		b[p] = byte('0' + i%10)
		i /= 10
	}
	return string(b[p:])
}
