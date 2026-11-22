package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
)

func (s *Server) runCase(w http.ResponseWriter, r *http.Request, tid string) {
	cid := r.PathValue("id")
	var projectID, scenario, datasetID, dataRowID string
	err := s.st.DB.QueryRow(
		"SELECT project_id, scenario, dataset_id, data_row_id FROM cases WHERE id = ? AND tenant_id = ?", cid, tid,
	).Scan(&projectID, &scenario, &datasetID, &dataRowID)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		VersionID string `json:"version_id"`
		DatasetID string `json:"dataset_id"`
		DataRowID string `json:"data_row_id"`
	}
	_ = decode(r, &body)
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
	rows, _ := s.st.DB.Query(`SELECT r.*, c.name AS case_name, c.scenario
		FROM runs r JOIN cases c ON c.id = r.case_id
		WHERE r.id = ? AND r.tenant_id = ?`, r.PathValue("id"), tid)
	out, _ := rowsToMaps(rows)
	if len(out) == 0 {
		fail(w, http.StatusNotFound, "执行记录不存在")
		return
	}
	write(w, http.StatusOK, out[0])
}

// heartbeat 供执行端领取与续租；返回待执行任务（首版按租户取最早 queued）。
func (s *Server) heartbeat(w http.ResponseWriter, r *http.Request, tid string) {
	var rid string
	err := s.st.DB.QueryRow(`SELECT id FROM runs WHERE tenant_id = ? AND status = 'queued'
		ORDER BY started_at, rowid LIMIT 1`, tid).Scan(&rid)
	if err == sql.ErrNoRows {
		write(w, http.StatusOK, map[string]any{"run": nil})
		return
	}
	_, _ = s.st.DB.Exec("UPDATE runs SET status = 'running' WHERE id = ?", rid)
	rows, _ := s.st.DB.Query(`SELECT r.id AS run_id, r.case_id, r.version_id, r.dataset_id, r.data_row_id,
		c.name, c.type, c.scenario, p.base_url, COALESCE(tdr.values_json, '{}') AS data_json
		FROM runs r JOIN cases c ON c.id = r.case_id JOIN projects p ON p.id = c.project_id
		LEFT JOIN test_data_records tdr ON tdr.id = r.data_row_id AND tdr.dataset_id = r.dataset_id AND tdr.tenant_id = r.tenant_id
		WHERE r.id = ? AND r.tenant_id = ?`, rid, tid)
	out, _ := rowsToMaps(rows)
	if len(out) == 0 {
		fail(w, http.StatusNotFound, "执行记录不存在")
		return
	}
	var data map[string]string
	_ = json.Unmarshal([]byte(out[0]["data_json"].(string)), &data)
	delete(out[0], "data_json")
	out[0]["data"] = data
	write(w, http.StatusOK, map[string]any{"run": out[0]})
}

func (s *Server) finishRun(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		Status      string `json:"status"`
		Passed      bool   `json:"passed"`
		NeedsReview bool   `json:"needs_review"`
		SelfHealed  bool   `json:"self_healed"`
		TokenIn     int    `json:"token_in"`
		TokenOut    int    `json:"token_out"`
		DurationMS  int    `json:"duration_ms"`
		Report      string `json:"report"`
		FallbackReason string `json:"fallback_reason"`
		FailureSigs []struct {
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
	rid := r.PathValue("id")
	_, err := s.st.DB.Exec(`UPDATE runs SET status = ?, passed = ?, needs_review = ?, self_healed = ?,
		token_in = ?, token_out = ?, duration_ms = ?, report = ?, fallback_reason = ?, ended_at = datetime('now')
		WHERE id = ? AND tenant_id = ?`,
		b.Status, boolInt(b.Passed), boolInt(b.NeedsReview), boolInt(b.SelfHealed),
		b.TokenIn, b.TokenOut, b.DurationMS, b.Report, b.FallbackReason, rid, tid)
	if err != nil {
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
