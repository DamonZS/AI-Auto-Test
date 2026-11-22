package api

import (
	"database/sql"
	"net/http"
	"strings"
)

func (s *Server) listRequirements(w http.ResponseWriter, r *http.Request, tid string) {
	rows, _ := s.st.DB.Query(
		"SELECT * FROM requirements WHERE tenant_id = ? ORDER BY created_at DESC", tid)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) createRequirement(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		ProjectID string `json:"project_id"`
		Code      string `json:"code"`
		Title     string `json:"title"`
		Content   string `json:"content"`
	}
	if err := decode(r, &b); err != nil || b.ProjectID == "" || b.Title == "" {
		fail(w, http.StatusBadRequest, "project_id 与 title 必填")
		return
	}
	rid := id()
	if _, err := s.st.DB.Exec(
		"INSERT INTO requirements (id, tenant_id, project_id, code, title, content) VALUES (?,?,?,?,?,?)",
		rid, tid, b.ProjectID, b.Code, b.Title, b.Content); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": rid})
}

func (s *Server) listVersions(w http.ResponseWriter, r *http.Request, tid string) {
	rows, _ := s.st.DB.Query("SELECT * FROM versions WHERE tenant_id = ? ORDER BY created_at DESC", tid)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) createVersion(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		ProjectID string   `json:"project_id"`
		Name      string   `json:"name"`
		CommitRef string   `json:"commit_ref"`
		Impact    []string `json:"impact"`
	}
	if err := decode(r, &b); err != nil || b.ProjectID == "" || b.Name == "" {
		fail(w, http.StatusBadRequest, "project_id 与 name 必填")
		return
	}
	vid := id()
	if _, err := s.st.DB.Exec(
		"INSERT INTO versions (id, tenant_id, project_id, name, commit_ref, impact_json) VALUES (?,?,?,?,?,?)",
		vid, tid, b.ProjectID, b.Name, b.CommitRef, mustJSON(b.Impact)); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": vid})
}

func (s *Server) listEnvironments(w http.ResponseWriter, r *http.Request, tid string) {
	rows, _ := s.st.DB.Query("SELECT * FROM environments WHERE tenant_id = ? ORDER BY created_at DESC", tid)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) createEnvironment(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		ProjectID string `json:"project_id"`
		Name      string `json:"name"`
		BaseURL   string `json:"base_url"`
		Source    string `json:"source"`
	}
	if err := decode(r, &b); err != nil || b.ProjectID == "" || b.BaseURL == "" {
		fail(w, http.StatusBadRequest, "project_id 与 base_url 必填")
		return
	}
	if b.Source == "" {
		b.Source = "platform"
	}
	eid := id()
	if _, err := s.st.DB.Exec(
		"INSERT INTO environments (id, tenant_id, project_id, name, base_url, source) VALUES (?,?,?,?,?,?)",
		eid, tid, b.ProjectID, b.Name, b.BaseURL, b.Source); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": eid})
}

func (s *Server) listCases(w http.ResponseWriter, r *http.Request, tid string) {
	q := "SELECT * FROM cases WHERE tenant_id = ?"
	args := []any{tid}
	if p := r.URL.Query().Get("project_id"); p != "" {
		q += " AND project_id = ?"
		args = append(args, p)
	}
	if st := r.URL.Query().Get("status"); st != "" {
		q += " AND status = ?"
		args = append(args, st)
	}
	rows, _ := s.st.DB.Query(q+" ORDER BY created_at DESC", args...)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) createCase(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		ProjectID      string `json:"project_id"`
		EnvironmentID  string `json:"environment_id"`
		RequirementID  string `json:"requirement_id"`
		Name           string `json:"name"`
		Type           string `json:"type"`
		Scenario       string `json:"scenario"`
		ExpectedSource string `json:"expected_source"`
		GenerationMode string `json:"generation_mode"`
		DatasetID      string `json:"dataset_id"`
		DataRowID      string `json:"data_row_id"`
	}
	if err := decode(r, &b); err != nil || b.ProjectID == "" || b.Name == "" {
		fail(w, http.StatusBadRequest, "project_id 与 name 必填")
		return
	}
	if b.Type == "" {
		b.Type = "ui"
	}
	if b.ExpectedSource == "" {
		b.ExpectedSource = "requirement"
	}
	if b.GenerationMode == "" {
		b.GenerationMode = "manual"
	}
	if err := s.validateDataBinding(tid, b.ProjectID, b.DatasetID, b.DataRowID); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	cid := id()
	_, err := s.st.DB.Exec(`INSERT INTO cases
		(id, tenant_id, project_id, environment_id, requirement_id, name, type, scenario, expected_source, generation_mode, dataset_id, data_row_id)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?)`,
		cid, tid, b.ProjectID, b.EnvironmentID, b.RequirementID, b.Name, b.Type, b.Scenario, b.ExpectedSource, b.GenerationMode, b.DatasetID, b.DataRowID)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.st.DB.Exec("INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario) VALUES (?, ?, 1, ?)", tid, cid, b.Scenario); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": cid})
}

func (s *Server) updateCase(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		Scenario string `json:"scenario"`
	}
	if err := decode(r, &b); err != nil || strings.TrimSpace(b.Scenario) == "" {
		fail(w, http.StatusBadRequest, "scenario 必填")
		return
	}

	cid := r.PathValue("id")
	var status string
	var version int
	err := s.st.DB.QueryRow("SELECT status, version_no FROM cases WHERE id = ? AND tenant_id = ?", cid, tid).Scan(&status, &version)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if status != "draft" {
		fail(w, http.StatusConflict, "只有草稿用例可以编辑")
		return
	}

	tx, err := s.st.DB.Begin()
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err = tx.Exec(`INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario)
		SELECT tenant_id, id, version_no, scenario FROM cases WHERE id = ? AND tenant_id = ?
		ON CONFLICT (tenant_id, case_id, version_no) DO NOTHING`, cid, tid); err == nil {
		_, err = tx.Exec("UPDATE cases SET scenario = ?, version_no = ?, updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", b.Scenario, version+1, cid, tid)
	}
	if err == nil {
		_, err = tx.Exec("INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario) VALUES (?, ?, ?, ?)", tid, cid, version+1, b.Scenario)
	}
	if err == nil {
		err = tx.Commit()
	} else {
		_ = tx.Rollback()
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "case.update", cid)
	write(w, http.StatusOK, map[string]any{"id": cid, "version_no": version + 1, "status": status})
}

func (s *Server) listCaseHistory(w http.ResponseWriter, r *http.Request, tid string) {
	cid := r.PathValue("id")
	var one int
	if err := s.st.DB.QueryRow("SELECT 1 FROM cases WHERE id = ? AND tenant_id = ?", cid, tid).Scan(&one); err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	} else if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if _, err := s.st.DB.Exec(`INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario)
		SELECT tenant_id, id, version_no, scenario FROM cases WHERE id = ? AND tenant_id = ?
		ON CONFLICT (tenant_id, case_id, version_no) DO NOTHING`, cid, tid); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	rows, err := s.st.DB.Query(`SELECT version_no, scenario, created_at FROM case_revisions
		WHERE case_id = ? AND tenant_id = ? ORDER BY version_no`, cid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := rowsToMaps(rows)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, out)
}

func (s *Server) reviewCase(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		Decision string `json:"decision"`
		Reviewer string `json:"reviewer"`
		Scenario string `json:"scenario"`
	}
	if err := decode(r, &b); err != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return
	}
	cid := r.PathValue("id")
	var cur string
	var vno int
	err := s.st.DB.QueryRow("SELECT status, version_no FROM cases WHERE id = ? AND tenant_id = ?", cid, tid).Scan(&cur, &vno)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "用例不存在")
		return
	}
	switch b.Decision {
	case "submit_review":
		_, err = s.st.DB.Exec("UPDATE cases SET status = 'in_review', updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", cid, tid)
	case "approve":
		_, err = s.st.DB.Exec("UPDATE cases SET status = 'approved', reviewer = ?, updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", b.Reviewer, cid, tid)
	case "reject", "revise":
		if b.Scenario == "" {
			fail(w, http.StatusBadRequest, "修订场景不能为空")
			return
		}
		if cur == "in_review" && b.Decision == "reject" {
			_, err = s.st.DB.Exec("UPDATE cases SET status = 'draft', updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", cid, tid)
			break
		}
		tx, txErr := s.st.DB.Begin()
		if txErr != nil {
			fail(w, http.StatusInternalServerError, txErr.Error())
			return
		}
		if _, txErr = tx.Exec(`INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario)
			SELECT tenant_id, id, version_no, scenario FROM cases WHERE id = ? AND tenant_id = ?
			ON CONFLICT (tenant_id, case_id, version_no) DO NOTHING`, cid, tid); txErr == nil {
			_, txErr = tx.Exec("UPDATE cases SET status = 'draft', scenario = ?, version_no = ?, updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", b.Scenario, vno+1, cid, tid)
		}
		if txErr == nil {
			_, txErr = tx.Exec("INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario) VALUES (?, ?, ?, ?)", tid, cid, vno+1, b.Scenario)
		}
		if txErr == nil {
			txErr = tx.Commit()
		} else {
			_ = tx.Rollback()
		}
		if txErr != nil {
			fail(w, http.StatusInternalServerError, txErr.Error())
			return
		}
		write(w, http.StatusOK, map[string]string{"id": cid, "decision": b.Decision})
		return
	case "deprecate":
		_, err = s.st.DB.Exec("UPDATE cases SET status = 'deprecated', updated_at = datetime('now') WHERE id = ? AND tenant_id = ?", cid, tid)
	default:
		fail(w, http.StatusBadRequest, "decision 取值：submit_review / approve / reject / deprecate")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, b.Reviewer, "web", "case."+b.Decision, cid)
	write(w, http.StatusOK, map[string]string{"id": cid, "decision": b.Decision})
}
