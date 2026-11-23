package api

import (
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strconv"
	"strings"
	"unicode/utf8"

	"platform/server/internal/store"
)

func (s *Server) registerRequirementPoints(mux *http.ServeMux) {
	const root = "/api/requirements/{id}/point-analyses"
	mux.HandleFunc("GET "+root, s.tenant(s.listRequirementPointAnalyses))
	mux.HandleFunc("POST "+root, s.tenant(s.parseRequirementPoints))
	mux.HandleFunc("GET "+root+"/{analysisId}", s.tenant(s.getRequirementPointAnalysis))
	mux.HandleFunc("POST "+root+"/{analysisId}/review", s.tenant(s.reviewRequirementPoints))
	mux.HandleFunc("POST "+root+"/{analysisId}/optimize", s.tenant(s.optimizeRequirementPoints))
}

func (s *Server) listRequirementPointAnalyses(w http.ResponseWriter, r *http.Request, tenantID string) {
	revisionNo, err := strconv.Atoi(r.URL.Query().Get("revision_no"))
	if err != nil || revisionNo < 1 {
		fail(w, http.StatusBadRequest, "需求修订号必须为正整数")
		return
	}
	history, err := s.st.ListRequirementPointAnalyses(r.Context(), tenantID, r.PathValue("id"), revisionNo)
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	write(w, http.StatusOK, history)
}

func (s *Server) getRequirementPointAnalysis(w http.ResponseWriter, r *http.Request, tenantID string) {
	analysis, err := s.st.GetRequirementPointAnalysis(r.Context(), tenantID, r.PathValue("id"), r.PathValue("analysisId"))
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	write(w, http.StatusOK, analysis)
}

func (s *Server) parseRequirementPoints(w http.ResponseWriter, r *http.Request, tenantID string) {
	var input struct {
		RevisionNo int `json:"revision_no"`
	}
	if !decodeRequirementPoints(w, r, &input) {
		return
	}
	if input.RevisionNo < 1 {
		fail(w, http.StatusBadRequest, "需求修订号必须为正整数")
		return
	}
	draft := store.RequirementPointAnalysis{ID: id(), RequirementID: r.PathValue("id"), RevisionNo: input.RevisionNo, RoundNo: 1}
	s.generateRequirementPointDraft(w, r, tenantID, draft, requirementPointPrompt{})
}

func (s *Server) optimizeRequirementPoints(w http.ResponseWriter, r *http.Request, tenantID string) {
	previous, err := s.st.GetRequirementPointAnalysis(r.Context(), tenantID, r.PathValue("id"), r.PathValue("analysisId"))
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	if previous.Status != "returned" {
		failRequirementPoints(w, store.ErrRequirementPointConflict)
		return
	}
	draft := store.RequirementPointAnalysis{ID: id(), RequirementID: previous.RequirementID, RevisionNo: previous.RevisionNo,
		RoundNo: previous.RoundNo + 1, ParentAnalysisID: previous.ID}
	s.generateRequirementPointDraft(w, r, tenantID, draft, requirementPointPrompt{PreviousPoints: previous.Points, Feedback: previous.Feedback})
}

func (s *Server) generateRequirementPointDraft(w http.ResponseWriter, r *http.Request, tenantID string, draft store.RequirementPointAnalysis, prompt requirementPointPrompt) {
	ctx := r.Context()
	source, err := s.st.GetRequirementPointSource(ctx, tenantID, draft.RequirementID, draft.RevisionNo)
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	if source.LastRound != draft.RoundNo-1 {
		failRequirementPoints(w, store.ErrRequirementPointConflict)
		return
	}
	if len(source.Title)+len(source.Content) > 32<<10 {
		fail(w, http.StatusUnprocessableEntity, "需求标题和正文不能超过 32 KiB")
		return
	}
	prompt.Title, prompt.Content = source.Title, source.Content
	draft.Points, err = s.generateRequirementPoints(ctx, tenantID, prompt)
	if err != nil {
		var modelErr requirementPointModelError
		if errors.As(err, &modelErr) {
			fail(w, modelErr.status, modelErr.message)
		} else {
			failRequirementPoints(w, err)
		}
		return
	}
	saved, err := s.st.CreateRequirementPointAnalysis(ctx, tenantID, draft)
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	action := "requirement.points.parse"
	if saved.ParentAnalysisID != "" {
		action = "requirement.points.optimize"
	}
	s.audit(tenantID, authenticatedActor(r), "web", action, saved.ID)
	write(w, http.StatusCreated, saved)
}

func (s *Server) reviewRequirementPoints(w http.ResponseWriter, r *http.Request, tenantID string) {
	var input struct {
		Action    string `json:"action"`
		Feedback  string `json:"feedback"`
		VersionID string `json:"version_id"`
	}
	if !decodeRequirementPoints(w, r, &input) {
		return
	}
	input.Feedback = strings.TrimSpace(input.Feedback)
	expected, next := "submitted", ""
	switch input.Action {
	case "submit":
		expected, next = "draft", "submitted"
	case "return":
		next = "returned"
	case "approve":
		next = "approved"
	}
	if next == "" || (next != "approved" && input.VersionID != "") || (next != "returned" && input.Feedback != "") {
		fail(w, http.StatusBadRequest, "评审操作或字段无效")
		return
	}
	if next == "returned" && (input.Feedback == "" || utf8.RuneCountInString(input.Feedback) > 2000) {
		fail(w, http.StatusBadRequest, "退回意见须为 1–2000 个字符")
		return
	}
	ctx := r.Context()
	analysis, err := s.st.GetRequirementPointAnalysis(ctx, tenantID, r.PathValue("id"), r.PathValue("analysisId"))
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	if analysis.Status != expected {
		failRequirementPoints(w, store.ErrRequirementPointConflict)
		return
	}
	if input.VersionID != "" {
		linked, err := s.st.IsRequirementPointVersionLinked(ctx, tenantID, analysis, input.VersionID)
		if err != nil {
			failRequirementPoints(w, err)
			return
		}
		if !linked {
			fail(w, http.StatusUnprocessableEntity, "所选版本未关联此需求修订")
			return
		}
	}
	analysis.Feedback, analysis.VersionID = input.Feedback, input.VersionID
	updated, err := s.st.UpdateRequirementPointReview(ctx, tenantID, analysis, next, authenticatedActor(r))
	if err != nil {
		failRequirementPoints(w, err)
		return
	}
	s.audit(tenantID, authenticatedActor(r), "web", "requirement.points."+input.Action, updated.ID)
	write(w, http.StatusOK, updated)
}

func decodeRequirementPoints(w http.ResponseWriter, r *http.Request, target any) bool {
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 16<<10))
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		fail(w, http.StatusRequestEntityTooLarge, "请求体不能超过 16 KiB")
		return false
	}
	if err != nil || !utf8.Valid(body) || decodeMCPSamplingJSON(string(body), target) != nil {
		fail(w, http.StatusBadRequest, "请求 JSON 无效")
		return false
	}
	return true
}

func failRequirementPoints(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "需求修订或功能点轮次不存在")
	case errors.Is(err, store.ErrRequirementPointConflict):
		fail(w, http.StatusConflict, "功能点轮次已存在或状态已变更，请刷新后重试")
	default:
		fail(w, http.StatusInternalServerError, "处理需求功能点失败")
	}
}
