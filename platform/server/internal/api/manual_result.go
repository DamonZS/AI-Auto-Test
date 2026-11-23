package api

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"
)

func (s *Server) recordManualResult(w http.ResponseWriter, r *http.Request, tenantID string) {
	var result struct {
		Status    string `json:"status"`
		Report    string `json:"report"`
		VersionID string `json:"version_id"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 16<<10)
	if err := decode(r, &result); err != nil {
		fail(w, http.StatusBadRequest, "请填写核验结果和证据")
		return
	}
	result.Report = strings.TrimSpace(result.Report)
	if result.Report == "" || (result.Status != "passed" && result.Status != "failed") {
		fail(w, http.StatusBadRequest, "请填写核验结果和证据")
		return
	}
	runID := id()
	err := s.st.DB.QueryRowContext(r.Context(), `INSERT INTO runs (id, tenant_id, case_id, version_id, status, passed, report, ended_at)
		SELECT ?, c.tenant_id, c.id, ?, ?, ?, ?, datetime('now') FROM cases c
		WHERE c.id = ? AND c.tenant_id = ? AND c.status = 'approved' AND c.type IN ('functional', 'security')
		AND (? = '' OR EXISTS (SELECT 1 FROM versions v WHERE v.id = ? AND v.tenant_id = c.tenant_id AND v.project_id = c.project_id))
		RETURNING id`, runID, result.VersionID, result.Status, boolInt(result.Status == "passed"), result.Report,
		r.PathValue("id"), tenantID, result.VersionID, result.VersionID).Scan(&runID)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusConflict, "请选择当前项目的版本和已定稿的人工用例")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存人工核验结果失败")
		return
	}
	s.audit(tenantID, authenticatedActor(r), "web", "run.manual_result", runID)
	write(w, http.StatusCreated, map[string]string{"run_id": runID, "status": result.Status})
}
