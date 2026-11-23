package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
)

func (s *Server) registerBugManagement(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/bugs/{id}", s.tenant(s.getBug))
	mux.HandleFunc("PATCH /api/bugs/{id}", s.tenant(s.updateBug))
}

func (s *Server) getBug(w http.ResponseWriter, r *http.Request, tid string) {
	ctx, bid := r.Context(), r.PathValue("id")
	rows, err := s.st.DB.QueryContext(ctx, `SELECT b.*, COALESCE(p.name, '') AS project_name,
		COALESCE(discovered.name, '') AS discovered_version_name, COALESCE(fixed.name, '') AS fixed_version_name
		FROM bugs b LEFT JOIN projects p ON p.id = b.project_id AND p.tenant_id = b.tenant_id
		LEFT JOIN versions discovered ON discovered.id = b.discovered_version_id AND discovered.tenant_id = b.tenant_id AND discovered.project_id = b.project_id
		LEFT JOIN versions fixed ON fixed.id = b.fixed_version_id AND fixed.tenant_id = b.tenant_id AND fixed.project_id = b.project_id
		WHERE b.id = ? AND b.tenant_id = ?`, bid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法加载缺陷详情")
		return
	}
	bugs, err := rowsToMaps(rows)
	if err != nil || rows.Err() != nil {
		fail(w, http.StatusInternalServerError, "无法加载缺陷详情")
		return
	}
	if len(bugs) == 0 {
		fail(w, http.StatusNotFound, "缺陷不存在")
		return
	}
	rows, err = s.st.DB.QueryContext(ctx, `SELECT id, from_status, to_status, actor, note, at FROM bug_events
		WHERE bug_id = ? AND tenant_id = ? ORDER BY id DESC`, bid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法加载流转记录")
		return
	}
	history, err := rowsToMaps(rows)
	if err != nil || rows.Err() != nil {
		fail(w, http.StatusInternalServerError, "无法加载流转记录")
		return
	}
	bug := bugs[0]
	bug["available_transitions"] = append([]string{}, bugTransitions[bug["status"].(string)]...)
	bug["history"] = history
	write(w, http.StatusOK, bug)
}

func (s *Server) updateBug(w http.ResponseWriter, r *http.Request, tid string) {
	var body struct {
		Title               *string `json:"title"`
		Severity            *string `json:"severity"`
		Priority            *string `json:"priority"`
		Evidence            *string `json:"evidence"`
		ReproSteps          *string `json:"repro_steps"`
		ExpectedResult      *string `json:"expected_result"`
		ActualResult        *string `json:"actual_result"`
		Assignee            *string `json:"assignee"`
		DiscoveredVersionID *string `json:"discovered_version_id"`
	}
	if err := decode(r, &body); err != nil {
		fail(w, http.StatusBadRequest, "无效的缺陷资料")
		return
	}
	if body.Title != nil {
		*body.Title = strings.TrimSpace(*body.Title)
		if *body.Title == "" {
			fail(w, http.StatusBadRequest, "标题不能为空")
			return
		}
	}
	if body.Severity != nil && !validBugSeverity(*body.Severity) {
		fail(w, http.StatusBadRequest, "缺陷级别须为 S1 至 S4")
		return
	}
	if body.Assignee != nil {
		*body.Assignee = strings.TrimSpace(*body.Assignee)
	}
	ctx, bid := r.Context(), r.PathValue("id")
	if body.DiscoveredVersionID != nil && *body.DiscoveredVersionID != "" {
		var projectID string
		err := s.st.DB.QueryRowContext(ctx, "SELECT project_id FROM bugs WHERE id = ? AND tenant_id = ?", bid, tid).Scan(&projectID)
		if err == sql.ErrNoRows {
			fail(w, http.StatusNotFound, "缺陷不存在")
			return
		}
		if err != nil {
			fail(w, http.StatusInternalServerError, "无法加载缺陷")
			return
		}
		if !s.checkBugVersion(w, r, tid, projectID, *body.DiscoveredVersionID) {
			return
		}
	}
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法更新缺陷资料")
		return
	}
	defer tx.Rollback()
	var status string
	err = tx.QueryRowContext(ctx, `UPDATE bugs SET title = COALESCE(?, title), severity = COALESCE(?, severity),
		priority = COALESCE(?, priority), evidence = COALESCE(?, evidence), repro_steps = COALESCE(?, repro_steps),
		expected_result = COALESCE(?, expected_result), actual_result = COALESCE(?, actual_result), assignee = COALESCE(?, assignee),
		discovered_version_id = COALESCE(?, discovered_version_id), updated_at = datetime('now')
		WHERE id = ? AND tenant_id = ? RETURNING status`, body.Title, body.Severity, body.Priority, body.Evidence, body.ReproSteps,
		body.ExpectedResult, body.ActualResult, body.Assignee, body.DiscoveredVersionID, bid, tid).Scan(&status)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "缺陷不存在")
		return
	}
	actor := bugActor(r, "")
	if err != nil || saveBugEvent(ctx, tx, tid, bid, status, status, actor, "更新缺陷资料") != nil || tx.Commit() != nil {
		fail(w, http.StatusInternalServerError, "无法更新缺陷资料")
		return
	}
	s.audit(tid, actor, "web", "bug.update", bid)
	write(w, http.StatusOK, map[string]string{"id": bid})
}

func (s *Server) checkBugVersion(w http.ResponseWriter, r *http.Request, tid, projectID, versionID string) bool {
	if versionID == "" {
		return true
	}
	var exists bool
	err := s.st.DB.QueryRowContext(r.Context(), `SELECT EXISTS(SELECT 1 FROM versions
		WHERE id = ? AND tenant_id = ? AND project_id = ?)`, versionID, tid, projectID).Scan(&exists)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法检查版本")
		return false
	}
	if !exists {
		fail(w, http.StatusBadRequest, "请选择本项目的版本")
	}
	return exists
}

func saveBugEvent(ctx context.Context, tx *sql.Tx, tid, bid, from, to, actor, note string) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO bug_events (tenant_id, bug_id, from_status, to_status, actor, note)
		VALUES (?, ?, ?, ?, ?, ?)`, tid, bid, from, to, actor, note)
	return err
}

func validBugSeverity(severity string) bool {
	return severity == "S1" || severity == "S2" || severity == "S3" || severity == "S4"
}

func bugActor(r *http.Request, fallback string) string {
	if actor := authenticatedActor(r); actor != "" {
		return actor
	}
	if fallback != "" {
		return fallback
	}
	return "user"
}
