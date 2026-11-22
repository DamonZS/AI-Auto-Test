package api

import (
	"database/sql"
	"net/http"
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
	q := "SELECT * FROM bugs WHERE tenant_id = ?"
	args := []any{tid}
	if st := r.URL.Query().Get("status"); st != "" {
		q += " AND status = ?"
		args = append(args, st)
	}
	if sv := r.URL.Query().Get("severity"); sv != "" {
		q += " AND severity = ?"
		args = append(args, sv)
	}
	rows, _ := s.st.DB.Query(q+" ORDER BY created_at DESC", args...)
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) createBug(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		ProjectID string `json:"project_id"`
		Title     string `json:"title"`
		Severity  string `json:"severity"`
		Priority  string `json:"priority"`
		Signature string `json:"signature"`
		Evidence  string `json:"evidence"`
	}
	if err := decode(r, &b); err != nil || b.ProjectID == "" || b.Title == "" {
		fail(w, http.StatusBadRequest, "project_id 与 title 必填")
		return
	}
	if b.Severity == "" {
		b.Severity = "S3"
	}
	if b.Priority == "" {
		b.Priority = "P2"
	}
	var seq int
	_ = s.st.DB.QueryRow("SELECT COALESCE(max(rowid),0)+1 FROM bugs WHERE tenant_id = ?", tid).Scan(&seq)
	bid := id()
	code := "BUG-" + short(tid) + "-" + itoa(seq)
	_, err := s.st.DB.Exec(`INSERT INTO bugs
		(id, tenant_id, project_id, bug_code, title, severity, priority, signature, evidence)
		VALUES (?,?,?,?,?,?,?,?,?)`,
		bid, tid, b.ProjectID, code, b.Title, b.Severity, b.Priority, b.Signature, b.Evidence)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.captureEvent(tid, bid, "", "new", "user", "手工创建")
	write(w, http.StatusOK, map[string]string{"id": bid, "bug_code": code})
}

func (s *Server) transitionBug(w http.ResponseWriter, r *http.Request, tid string) {
	var b struct {
		To       string `json:"to"`
		Actor    string `json:"actor"`
		Note     string `json:"note"`
		Severity string `json:"severity"`
	}
	if err := decode(r, &b); err != nil || b.To == "" {
		fail(w, http.StatusBadRequest, "to 必填")
		return
	}
	bid := r.PathValue("id")
	var cur string
	err := s.st.DB.QueryRow("SELECT status FROM bugs WHERE id = ? AND tenant_id = ?", bid, tid).Scan(&cur)
	if err == sql.ErrNoRows {
		fail(w, http.StatusNotFound, "缺陷不存在")
		return
	}
	allowed := false
	for _, next := range bugTransitions[cur] {
		if next == b.To {
			allowed = true
			break
		}
	}
	if !allowed {
		fail(w, http.StatusConflict, "不允许从 "+cur+" 转到 "+b.To)
		return
	}
	_, err = s.st.DB.Exec(`UPDATE bugs SET status = ?, reopen_count = CASE WHEN ? = 'reopened' THEN reopen_count + 1 ELSE reopen_count END,
		severity = COALESCE(NULLIF(?, ''), severity), updated_at = datetime('now') WHERE id = ? AND tenant_id = ?`,
		b.To, b.To, b.Severity, bid, tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.captureEvent(tid, bid, cur, b.To, b.Actor, b.Note)
	s.audit(tid, b.Actor, "web", "bug."+b.To, bid)
	write(w, http.StatusOK, map[string]string{"id": bid, "status": b.To})
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
