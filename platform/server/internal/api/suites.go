package api

import (
	"database/sql"
	"encoding/json"
	"net/http"
	"strings"
)

func (s *Server) listSuites(w http.ResponseWriter, r *http.Request, tid string) {
	projectID := r.URL.Query().Get("project_id")
	rows, err := s.st.DB.QueryContext(r.Context(), `SELECT id, tenant_id, name, created_at, case_ids, max_concurrency, project_id FROM suites
		WHERE tenant_id = ? AND (? = '' OR project_id = ?) ORDER BY created_at DESC, id`, tid, projectID, projectID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := rowsToMaps(rows)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, suite := range out {
		suite["case_ids"] = json.RawMessage(suite["case_ids"].(string))
	}
	write(w, http.StatusOK, out)
}

func (s *Server) createSuite(w http.ResponseWriter, r *http.Request, tid string) {
	var body struct {
		Name      string `json:"name"`
		ProjectID string `json:"project_id"`
	}
	if err := decode(r, &body); err != nil || strings.TrimSpace(body.Name) == "" {
		fail(w, http.StatusBadRequest, "name 必填")
		return
	}
	suiteID := id()
	result, err := s.st.DB.ExecContext(r.Context(), `INSERT INTO suites (id, tenant_id, name, project_id)
		SELECT ?, ?, ?, ? WHERE ? = '' OR EXISTS (SELECT 1 FROM projects WHERE id = ? AND tenant_id = ?)`,
		suiteID, tid, strings.TrimSpace(body.Name), body.ProjectID, body.ProjectID, body.ProjectID, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		fail(w, http.StatusNotFound, "项目不存在")
		return
	}
	write(w, http.StatusOK, map[string]any{"id": suiteID, "name": strings.TrimSpace(body.Name), "project_id": body.ProjectID, "case_ids": []string{}, "max_concurrency": 1})
}

func (s *Server) updateSuiteConcurrency(w http.ResponseWriter, r *http.Request, tid string) {
	var body struct {
		MaxConcurrency int `json:"max_concurrency"`
	}
	if err := decode(r, &body); err != nil || body.MaxConcurrency < 1 {
		fail(w, http.StatusBadRequest, "max_concurrency 必须是正整数")
		return
	}
	suiteID := r.PathValue("id")
	err := s.st.DB.QueryRow(`UPDATE suites SET max_concurrency = ? WHERE id = ? AND tenant_id = ? RETURNING id`,
		body.MaxConcurrency, suiteID, tid).Scan(&suiteID)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "套件不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"id": suiteID, "max_concurrency": body.MaxConcurrency})
}

func (s *Server) runSuite(w http.ResponseWriter, r *http.Request, tid string) {
	suiteID := r.PathValue("id")
	var projectID string
	if err := s.st.DB.QueryRow("SELECT project_id FROM suites WHERE id = ? AND tenant_id = ?", suiteID, tid).Scan(&projectID); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "套件不存在")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	var body struct {
		CaseIDs []string `json:"case_ids"`
	}
	if err := decode(r, &body); err != nil || len(body.CaseIDs) == 0 {
		fail(w, http.StatusBadRequest, "case_ids 必须至少包含一个用例")
		return
	}
	tx, err := s.st.DB.Begin()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer tx.Rollback()
	if _, err = tx.Exec("UPDATE suites SET case_ids = ? WHERE id = ? AND tenant_id = ?", mustJSON(body.CaseIDs), suiteID, tid); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	for _, caseID := range body.CaseIDs {
		var status, caseType string
		if err = tx.QueryRow("SELECT status, type FROM cases WHERE id = ? AND tenant_id = ? AND (? = '' OR project_id = ?)", caseID, tid, projectID, projectID).Scan(&status, &caseType); err != nil {
			if err == sql.ErrNoRows {
				fail(w, http.StatusNotFound, "用例不存在")
			} else {
				fail(w, http.StatusInternalServerError, err.Error())
			}
			return
		}
		if status != "approved" || caseType == "functional" || caseType == "security" {
			fail(w, http.StatusConflict, "只有已定稿的自动化用例可以加入执行批次")
			return
		}
	}
	runID := id()
	var maxConcurrency int
	if err = tx.QueryRow(`INSERT INTO suite_runs (id, tenant_id, suite_id, max_concurrency)
		SELECT ?, tenant_id, id, max_concurrency FROM suites WHERE id = ? AND tenant_id = ? RETURNING max_concurrency`,
		runID, suiteID, tid).Scan(&maxConcurrency); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = enqueueSuiteMembers(tx, tid, runID, body.CaseIDs); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if err = tx.Commit(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]any{"id": runID, "status": "queued", "max_concurrency": maxConcurrency})
}

func enqueueSuiteMembers(tx *sql.Tx, tid, runID string, caseIDs []string) error {
	for _, caseID := range caseIDs {
		result, err := tx.Exec(`INSERT INTO runs (id, tenant_id, case_id, suite_run_id, dataset_id, data_row_id)
			SELECT ?, tenant_id, id, ?, dataset_id, data_row_id FROM cases
			WHERE tenant_id = ? AND id = ? AND status = 'approved' AND type NOT IN ('functional', 'security')`, id(), runID, tid, caseID)
		if err != nil {
			return err
		}
		inserted, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if inserted == 0 {
			return sql.ErrNoRows
		}
	}
	return nil
}

func (s *Server) listSuiteRuns(w http.ResponseWriter, r *http.Request, tid string) {
	suiteID := r.PathValue("id")
	var exists int
	if err := s.st.DB.QueryRow("SELECT 1 FROM suites WHERE id = ? AND tenant_id = ?", suiteID, tid).Scan(&exists); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "套件不存在")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := s.st.DB.Query(`SELECT batch.id, CASE
		WHEN SUM(run.status = 'queued') = COUNT(run.id) THEN 'queued'
		WHEN MAX(run.status IN ('queued', 'running')) THEN 'running'
		WHEN MAX(run.status IN ('failed', 'error')) THEN 'failed'
		WHEN MAX(run.status = 'needs_review' OR run.needs_review = 1) THEN 'needs_review'
		WHEN SUM(run.status = 'passed' OR run.passed = 1) = COUNT(run.id) THEN 'passed'
		ELSE 'needs_review' END AS status
		FROM suite_runs batch JOIN runs run ON run.tenant_id = batch.tenant_id AND run.suite_run_id = batch.id
		WHERE batch.tenant_id = ? AND batch.suite_id = ? GROUP BY batch.id ORDER BY batch.rowid DESC`, tid, suiteID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := rowsToMaps(rows)
	if err == nil {
		err = rows.Err()
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, out)
}

func (s *Server) suiteRunSummary(w http.ResponseWriter, r *http.Request, tid string) {
	runID := r.PathValue("id")
	rows, err := s.st.DB.Query(`SELECT run.case_id, run.status, run.passed, run.needs_review, batch.max_concurrency
		FROM runs run JOIN suite_runs batch ON batch.id = run.suite_run_id AND batch.tenant_id = run.tenant_id
		WHERE run.tenant_id = ? AND run.suite_run_id = ? ORDER BY run.rowid`, tid, runID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	defer rows.Close()
	type result struct {
		CaseID      string `json:"case_id"`
		Status      string `json:"status"`
		Passed      *bool  `json:"passed"`
		NeedsReview bool   `json:"needs_review"`
	}
	results := []result{}
	passed, failed, needsReview := 0, 0, 0
	var maxConcurrency int
	for rows.Next() {
		var item result
		var pass sql.NullBool
		var review int
		if err := rows.Scan(&item.CaseID, &item.Status, &pass, &review, &maxConcurrency); err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		item.NeedsReview = review != 0
		if pass.Valid {
			value := pass.Bool
			item.Passed = &value
		}
		if item.Status == "passed" {
			passed++
		} else if item.Status == "failed" || item.Status == "error" {
			failed++
		}
		if item.NeedsReview || item.Status == "needs_review" {
			needsReview++
		}
		results = append(results, item)
	}
	if err := rows.Err(); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if len(results) == 0 {
		fail(w, http.StatusNotFound, "套件执行记录不存在")
		return
	}
	write(w, http.StatusOK, map[string]any{"id": runID, "total": len(results), "passed": passed, "failed": failed, "needs_review": needsReview, "runs": results, "max_concurrency": maxConcurrency})
}
