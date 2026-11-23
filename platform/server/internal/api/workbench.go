package api

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"
)

func (s *Server) registerWorkbench(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/workbench/templates", s.tenant(func(w http.ResponseWriter, _ *http.Request, _ string) {
		write(w, http.StatusOK, workbenchCatalog)
	}))
	mux.HandleFunc("GET /api/workbench/documents", s.tenant(s.listWorkbenchDocuments))
	mux.HandleFunc("POST /api/workbench/documents", s.tenant(s.createWorkbenchDocument))
	mux.HandleFunc("POST /api/workbench/documents/generate", s.tenant(s.generateWorkbenchDocument))
	mux.HandleFunc("GET /api/workbench/documents/{id}", s.tenant(s.getWorkbenchDocument))
	mux.HandleFunc("PATCH /api/workbench/documents/{id}", s.tenant(s.editWorkbenchDocument))
	mux.HandleFunc("POST /api/workbench/documents/{id}/review", s.tenant(s.reviewWorkbenchDocument))
	mux.HandleFunc("GET /api/workbench/documents/{id}/export", s.tenant(s.exportWorkbenchDocument))
}

type workbenchDocument struct {
	ID        string `json:"id"`
	Kind      string `json:"kind"`
	ProjectID string `json:"project_id"`
	VersionID string `json:"version_id"`
	workbenchContent
	Status     string            `json:"status"`
	Revision   int               `json:"revision"`
	Reviewer   string            `json:"reviewer"`
	ReviewNote string            `json:"review_note"`
	ReviewedAt string            `json:"reviewed_at"`
	CreatedAt  string            `json:"created_at"`
	UpdatedAt  string            `json:"updated_at"`
	Metrics    *workbenchMetrics `json:"metrics,omitempty"`
}

const workbenchColumns = `id, kind, project_id, version_id, content, status, revision,
	reviewer, review_note, reviewed_at, created_at, updated_at, metrics`

func scanWorkbenchDocument(row interface{ Scan(...any) error }) (workbenchDocument, error) {
	var doc workbenchDocument
	var content, metrics string
	if err := row.Scan(&doc.ID, &doc.Kind, &doc.ProjectID, &doc.VersionID, &content, &doc.Status, &doc.Revision,
		&doc.Reviewer, &doc.ReviewNote, &doc.ReviewedAt, &doc.CreatedAt, &doc.UpdatedAt, &metrics); err != nil {
		return doc, err
	}
	if err := json.Unmarshal([]byte(content), &doc.workbenchContent); err != nil {
		return doc, err
	}
	err := json.Unmarshal([]byte(metrics), &doc.Metrics)
	return doc, err
}

func (s *Server) listWorkbenchDocuments(w http.ResponseWriter, r *http.Request, tenantID string) {
	rows, err := s.st.DB.QueryContext(r.Context(), `SELECT `+workbenchColumns+`
		FROM workbench_documents WHERE tenant_id = ? ORDER BY updated_at DESC, id DESC`, tenantID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "无法读取测试产物")
		return
	}
	defer rows.Close()
	documents := []workbenchDocument{}
	for rows.Next() {
		doc, err := scanWorkbenchDocument(rows)
		if err != nil {
			fail(w, http.StatusInternalServerError, "无法读取测试产物")
			return
		}
		documents = append(documents, doc)
	}
	if rows.Err() != nil {
		fail(w, http.StatusInternalServerError, "无法读取测试产物")
		return
	}
	write(w, http.StatusOK, documents)
}

func decodeWorkbench(w http.ResponseWriter, r *http.Request, body any) bool {
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(body); err != nil || decoder.Decode(new(json.RawMessage)) != io.EOF {
		fail(w, http.StatusBadRequest, "请求体无效")
		return false
	}
	return true
}

func (s *Server) createWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string) {
	var body struct {
		Kind      string `json:"kind"`
		ProjectID string `json:"project_id"`
		VersionID string `json:"version_id"`
		workbenchContent
	}
	if !decodeWorkbench(w, r, &body) {
		return
	}
	tmpl := findWorkbenchTemplate(body.Kind)
	if tmpl == nil || body.ProjectID == "" || body.VersionID == "" {
		fail(w, http.StatusBadRequest, "kind、project_id 与 version_id 必须有效")
		return
	}
	if err := body.validate(tmpl); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.insertWorkbenchDraft(w, r, tenantID, body.ProjectID, body.VersionID, body.Kind, body.workbenchContent)
}

func (s *Server) insertWorkbenchDraft(w http.ResponseWriter, r *http.Request, tenantID, projectID, versionID, kind string, content workbenchContent) {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	doc, err := scanWorkbenchDocument(s.st.DB.QueryRowContext(r.Context(), `INSERT INTO workbench_documents
		(id, tenant_id, project_id, version_id, kind, content, created_at, updated_at)
		SELECT ?, project.tenant_id, project.id, version.id, ?, ?, ?, ? FROM versions version
		JOIN projects project ON project.id = version.project_id AND project.tenant_id = version.tenant_id
		WHERE project.tenant_id = ? AND project.id = ? AND version.id = ? RETURNING `+workbenchColumns,
		id(), kind, mustJSON(content), now, now, tenantID, projectID, versionID))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "项目或版本不存在")
	case err != nil:
		fail(w, http.StatusInternalServerError, "无法创建测试产物")
	default:
		write(w, http.StatusCreated, doc)
	}
}

func (s *Server) loadWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string) (workbenchDocument, bool) {
	doc, err := scanWorkbenchDocument(s.st.DB.QueryRowContext(r.Context(), `SELECT `+workbenchColumns+`
		FROM workbench_documents WHERE tenant_id = ? AND id = ?`, tenantID, r.PathValue("id")))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "测试产物不存在")
	case err != nil:
		fail(w, http.StatusInternalServerError, "无法读取测试产物")
	}
	return doc, err == nil
}

func (s *Server) getWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string) {
	if doc, ok := s.loadWorkbenchDocument(w, r, tenantID); ok {
		write(w, http.StatusOK, doc)
	}
}

func (s *Server) editWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string) {
	var body struct {
		Revision int `json:"revision"`
		workbenchContent
	}
	if !decodeWorkbench(w, r, &body) {
		return
	}
	if body.Revision < 1 {
		fail(w, http.StatusBadRequest, "revision 必须为正整数")
		return
	}
	doc, ok := s.loadWorkbenchDocument(w, r, tenantID)
	if !ok {
		return
	}
	if doc.Revision != body.Revision || (doc.Status != "draft" && doc.Status != "rejected") {
		fail(w, http.StatusConflict, "状态或修订号已变化，请重新加载")
		return
	}
	if err := body.validate(findWorkbenchTemplate(doc.Kind)); err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	doc.workbenchContent = body.workbenchContent
	doc.Status, doc.Reviewer, doc.ReviewNote, doc.ReviewedAt, doc.Metrics = "draft", "", "", "", nil
	s.saveWorkbenchDocument(w, r, tenantID, doc, "")
}

func (s *Server) reviewWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string) {
	var body struct {
		Action   string `json:"action"`
		Revision int    `json:"revision"`
		Reviewer string `json:"reviewer"`
		Note     string `json:"note"`
	}
	if !decodeWorkbench(w, r, &body) {
		return
	}
	body.Reviewer = strings.TrimSpace(body.Reviewer)
	if actor := authenticatedActor(r); actor != "" {
		body.Reviewer = actor
	}
	body.Note = strings.TrimSpace(body.Note)
	from, to := "in_review", ""
	switch body.Action {
	case "submit":
		from, to = "draft", "in_review"
	case "approve":
		to = "approved"
	case "reject":
		to = "rejected"
	}
	if to == "" || body.Revision < 1 || (body.Action != "submit" && body.Reviewer == "") ||
		(body.Action == "reject" && body.Note == "") {
		fail(w, http.StatusBadRequest, "审核动作、修订号、签核人或驳回意见无效")
		return
	}
	doc, ok := s.loadWorkbenchDocument(w, r, tenantID)
	if !ok {
		return
	}
	if doc.Revision != body.Revision || doc.Status != from {
		fail(w, http.StatusConflict, "状态或修订号已变化，请重新加载")
		return
	}
	if body.Action == "submit" {
		if missing := doc.missingSections(findWorkbenchTemplate(doc.Kind)); len(missing) > 0 {
			write(w, http.StatusUnprocessableEntity, map[string]any{"error": "请补齐必填内容并确认检查项通过", "missing_sections": missing})
			return
		}
		if doc.Kind == "test_report" || doc.Kind == "daily_report" {
			metrics, err := s.collectWorkbenchMetrics(r.Context(), tenantID, doc)
			if err != nil {
				fail(w, http.StatusInternalServerError, "无法生成报告统计")
				return
			}
			doc.Metrics = metrics
		}
	} else {
		doc.Reviewer, doc.ReviewNote = body.Reviewer, body.Note
		doc.ReviewedAt = time.Now().UTC().Format(time.RFC3339Nano)
	}
	doc.Status = to
	s.saveWorkbenchDocument(w, r, tenantID, doc, body.Action)
}

func (s *Server) saveWorkbenchDocument(w http.ResponseWriter, r *http.Request, tenantID string, doc workbenchDocument, action string) {
	updated, err := s.updateWorkbenchDocument(r.Context(), tenantID, doc, action, authenticatedActor(r))
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusConflict, "状态或修订号已变化，请重新加载")
	case err != nil:
		fail(w, http.StatusInternalServerError, "无法保存测试产物")
	default:
		write(w, http.StatusOK, updated)
	}
}

func (s *Server) updateWorkbenchDocument(ctx context.Context, tenantID string, doc workbenchDocument, action, actor string) (workbenchDocument, error) {
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return doc, err
	}
	defer tx.Rollback()
	updated, err := scanWorkbenchDocument(tx.QueryRowContext(ctx, `UPDATE workbench_documents
		SET content = ?, metrics = ?, status = ?, revision = revision + 1, reviewer = ?, review_note = ?, reviewed_at = ?, updated_at = ?
		WHERE tenant_id = ? AND id = ? AND revision = ? RETURNING `+workbenchColumns,
		mustJSON(doc.workbenchContent), mustJSON(doc.Metrics), doc.Status, doc.Reviewer, doc.ReviewNote,
		doc.ReviewedAt, time.Now().UTC().Format(time.RFC3339Nano), tenantID, doc.ID, doc.Revision))
	if err != nil {
		return doc, err
	}
	if action != "" {
		if actor == "" {
			actor = doc.Reviewer
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs (tenant_id, actor, action, target)
			VALUES (?, ?, ?, ?)`, tenantID, actor, "workbench."+action, doc.ID); err != nil {
			return doc, err
		}
	}
	return updated, tx.Commit()
}
