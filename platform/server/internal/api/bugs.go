package api

import (
	"database/sql"
	"net/http"
	"slices"
	"strings"
)

var bugTransitions = map[string][]string{
	"new":       {"confirmed", "rejected"},
	"reopened":  {"confirmed", "rejected"},
	"confirmed": {"fixing", "rejected"},
	"fixing":    {"fixed"},
	"fixed":     {"verified", "reopened"},
	"verified":  {"closed", "reopened"},
	"closed":    {"reopened"},
}

func (s *Server) listBugs(w http.ResponseWriter, r *http.Request, tid string) {
	query := "SELECT * FROM bugs WHERE tenant_id = ?"
	args := []any{tid}
	filters := r.URL.Query()
	for _, field := range []string{"project_id", "status", "severity"} {
		if value := filters.Get(field); value != "" {
			query += " AND " + field + " = ?"
			args = append(args, value)
		}
	}
	if term := strings.TrimSpace(filters.Get("q")); term != "" {
		query += " AND (instr(lower(title), lower(?)) > 0 OR instr(lower(bug_code), lower(?)) > 0)"
		args = append(args, term, term)
	}
	rows, err := s.st.DB.QueryContext(r.Context(), query+" ORDER BY created_at DESC, rowid DESC", args...)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法加载缺陷列表")
		return
	}
	out, err := rowsToMaps(rows)
	if err != nil || rows.Err() != nil {
		fail(w, http.StatusInternalServerError, "无法加载缺陷列表")
		return
	}
	write(w, http.StatusOK, out)
}

func (s *Server) createBug(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		ProjectID           string `json:"project_id"`
		Title               string `json:"title"`
		Severity            string `json:"severity"`
		Priority            string `json:"priority"`
		Signature           string `json:"signature"`
		Evidence            string `json:"evidence"`
		ReproSteps          string `json:"repro_steps"`
		ExpectedResult      string `json:"expected_result"`
		ActualResult        string `json:"actual_result"`
		Assignee            string `json:"assignee"`
		DiscoveredVersionID string `json:"discovered_version_id"`
	}
	if err := decode(r, &b); err != nil || b.ProjectID == "" || strings.TrimSpace(b.Title) == "" {
		fail(w, http.StatusBadRequest, "project_id 与 title 必填")
		return
	}
	if b.Severity == "" {
		b.Severity = "S3"
	}
	if b.Priority == "" {
		b.Priority = "P2"
	}
	if !validBugSeverity(b.Severity) {
		fail(w, http.StatusBadRequest, "缺陷级别须为 S1 至 S4")
		return
	}
	if !s.checkBugVersion(w, r, tid, b.ProjectID, b.DiscoveredVersionID) {
		return
	}
	ctx := r.Context()
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法创建缺陷")
		return
	}
	defer tx.Rollback()
	bid := id()
	var code string
	err = tx.QueryRowContext(ctx, `INSERT INTO bugs
		(id, tenant_id, project_id, bug_code, title, severity, priority, signature, evidence,
		 repro_steps, expected_result, actual_result, assignee, discovered_version_id)
		SELECT ?, tenant_id, id, ? || (SELECT COALESCE(max(rowid), 0) + 1 FROM bugs WHERE tenant_id = ?),
		 ?, ?, ?, ?, ?, ?, ?, ?, ?, ? FROM projects WHERE id = ? AND tenant_id = ? RETURNING bug_code`,
		bid, "BUG-"+short(tid)+"-", tid, strings.TrimSpace(b.Title), b.Severity, b.Priority, b.Signature, b.Evidence,
		b.ReproSteps, b.ExpectedResult, b.ActualResult, strings.TrimSpace(b.Assignee), b.DiscoveredVersionID, b.ProjectID, tid).Scan(&code)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	if err != nil || saveBugEvent(ctx, tx, tid, bid, "", "new", bugActor(r, ""), "手工创建") != nil || tx.Commit() != nil {
		fail(w, http.StatusInternalServerError, "无法创建缺陷")
		return
	}
	write(w, http.StatusOK, map[string]string{"id": bid, "bug_code": code})
}

func (s *Server) transitionBug(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		To             string  `json:"to"`
		Actor          string  `json:"actor"`
		Note           string  `json:"note"`
		Severity       string  `json:"severity"`
		FixedVersionID *string `json:"fixed_version_id"`
	}
	if err := decode(r, &b); err != nil || b.To == "" {
		fail(w, http.StatusBadRequest, "to 必填")
		return
	}
	bid := r.PathValue("id")
	ctx := r.Context()
	var cur, projectID string
	err := s.st.DB.QueryRowContext(ctx, "SELECT status, project_id FROM bugs WHERE id = ? AND tenant_id = ?", bid, tid).Scan(&cur, &projectID)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "缺陷不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法加载缺陷")
		return
	}
	if b.Severity != "" && !validBugSeverity(b.Severity) {
		fail(w, http.StatusBadRequest, "缺陷级别须为 S1 至 S4")
		return
	}
	severityOnly := cur == b.To && b.Severity != "" && b.FixedVersionID == nil
	if !severityOnly && !slices.Contains(bugTransitions[cur], b.To) {
		fail(w, http.StatusConflict, "不允许从 "+cur+" 转到 "+b.To)
		return
	}
	if b.FixedVersionID != nil {
		if b.To != "fixed" {
			fail(w, http.StatusBadRequest, "仅在标记已修复时可设置修复版本")
			return
		}
		if !s.checkBugVersion(w, r, tid, projectID, *b.FixedVersionID) {
			return
		}
	}
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法更新缺陷")
		return
	}
	defer tx.Rollback()
	var updated string
	err = tx.QueryRowContext(ctx, `UPDATE bugs SET status = ?, reopen_count = reopen_count + ?,
		severity = COALESCE(NULLIF(?, ''), severity), fixed_version_id = COALESCE(?, fixed_version_id),
		updated_at = datetime('now') WHERE id = ? AND tenant_id = ? AND status = ? RETURNING status`,
		b.To, boolInt(cur != b.To && b.To == "reopened"), b.Severity, b.FixedVersionID, bid, tid, cur).Scan(&updated)
	if err == sql.ErrNoRows {
		fail(w, http.StatusConflict, "缺陷状态已变化，请刷新后重试")
		return
	}
	actor := bugActor(r, b.Actor)
	action := b.To
	if severityOnly {
		action = "severity"
	}
	if err != nil || (!severityOnly && saveBugEvent(ctx, tx, tid, bid, cur, b.To, actor, b.Note) != nil) || tx.Commit() != nil {
		fail(w, http.StatusInternalServerError, "无法更新缺陷")
		return
	}
	s.audit(tid, actor, "web", "bug."+action, bid)
	write(w, http.StatusOK, map[string]string{"id": bid, "status": updated})
}

func (s *Server) retestBug(w http.ResponseWriter, r *http.Request, tid string) {
	bid, runID := r.PathValue("id"), id()
	result, err := s.st.DB.ExecContext(r.Context(), `INSERT INTO runs (id, tenant_id, case_id, version_id, dataset_id, data_row_id)
		SELECT ?, r.tenant_id, r.case_id, r.version_id, r.dataset_id, r.data_row_id
		FROM bugs b JOIN runs r ON r.id = b.run_id AND r.tenant_id = b.tenant_id
		JOIN cases c ON c.id = r.case_id AND c.tenant_id = r.tenant_id
		WHERE b.id = ? AND b.tenant_id = ? AND c.status = 'approved'`, runID, bid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法创建复测任务")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		fail(w, http.StatusConflict, "当前工作区没有可供此缺陷复测的定稿用例")
		return
	}
	s.audit(tid, authenticatedActor(r), "web", "bug.retest", bid)
	write(w, http.StatusOK, map[string]string{"run_id": runID, "status": "queued"})
}

func (s *Server) listPatches(w http.ResponseWriter, r *http.Request, tid string) {
	rows, _ := s.st.DB.Query(
		"SELECT * FROM heal_patches WHERE tenant_id = ? ORDER BY created_at DESC", tid)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) decidePatch(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		Decision string `json:"decision"`
		Reviewer string `json:"reviewer"`
	}
	if err := decode(r, &b); err != nil || (b.Decision != "approve" && b.Decision != "reject") {
		fail(w, http.StatusBadRequest, "decision 取值 approve / reject")
		return
	}
	pid := r.PathValue("id")
	res, err := s.st.DB.Exec(
		"UPDATE heal_patches SET status = ? WHERE id = ? AND tenant_id = ? AND status = 'pending'",
		map[string]string{"approve": "approved", "reject": "rejected"}[b.Decision], pid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if n, _ := res.RowsAffected(); n == 0 {
		fail(w, http.StatusConflict, "补丁不存在或已处理")
		return
	}
	s.audit(tid, b.Reviewer, "web", "patch."+b.Decision, pid)
	write(w, http.StatusOK, map[string]string{"id": pid, "status": b.Decision})
}
