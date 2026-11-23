package api

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"unicode/utf8"
)

type caseReviewState struct {
	ID              string `json:"id"`
	Status          string `json:"status"`
	Version         int    `json:"version_no"`
	Reviewer        string `json:"reviewer"`
	Feedback        string `json:"review_feedback"`
	FeedbackVersion int    `json:"review_feedback_version"`
	Decision        string `json:"decision,omitempty"`
	Kind            string `json:"-"`
	Scenario        string `json:"-"`
	Name            string `json:"-"`
	BaseURL         string `json:"-"`
	ProjectID       string `json:"-"`
	RequirementID   string `json:"-"`
}

var errCaseReviewConflict = errors.New("case review state changed")

func (s *Server) readCaseReview(w http.ResponseWriter, r *http.Request, tid string) (caseReviewState, bool) {
	current := caseReviewState{ID: r.PathValue("id")}
	err := s.st.DB.QueryRowContext(r.Context(), `SELECT c.status, c.version_no, c.reviewer, c.review_feedback,
		c.review_feedback_version, c.type, c.scenario, c.name, COALESCE(p.base_url, ''), c.project_id, c.requirement_id
		FROM cases c LEFT JOIN projects p ON p.id = c.project_id AND p.tenant_id = c.tenant_id
		WHERE c.id = ? AND c.tenant_id = ?`, current.ID, tid).Scan(&current.Status, &current.Version,
		&current.Reviewer, &current.Feedback, &current.FeedbackVersion, &current.Kind, &current.Scenario,
		&current.Name, &current.BaseURL, &current.ProjectID, &current.RequirementID)
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "用例不存在")
	case err != nil:
		fail(w, http.StatusInternalServerError, "读取用例审核状态失败")
	}
	return current, err == nil
}

func (s *Server) reviewCaseWorkflow(w http.ResponseWriter, r *http.Request, tid string) {
	var input struct {
		Decision string `json:"decision"`
		Feedback string `json:"feedback"`
		Scenario string `json:"scenario"`
	}
	r.Body = http.MaxBytesReader(w, r.Body, 128<<10)
	if decode(r, &input) != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return
	}
	current, ok := s.readCaseReview(w, r, tid)
	if !ok {
		return
	}
	next := current
	next.Decision = input.Decision
	actor := authenticatedActor(r)
	switch input.Decision {
	case "submit_review":
		if current.Status != "draft" {
			fail(w, http.StatusConflict, "只有草稿可以提交审核")
			return
		}
		if current.FeedbackVersion == current.Version {
			fail(w, http.StatusConflict, "请先处理当前版本的退回反馈")
			return
		}
		next.Status, next.Reviewer = "in_review", ""
	case "approve":
		if current.Status != "in_review" {
			fail(w, http.StatusConflict, "只有待审核用例可以定稿")
			return
		}
		next.Status, next.Reviewer = "approved", actor
	case "reject":
		if current.Status != "in_review" {
			fail(w, http.StatusConflict, "只有待审核用例可以退回")
			return
		}
		next.Feedback = strings.TrimSpace(input.Feedback)
		if next.Feedback == "" || utf8.RuneCountInString(next.Feedback) > 2000 || caseGenerationPrivateInput.MatchString(next.Feedback) {
			fail(w, http.StatusBadRequest, "请填写 1 至 2000 字的反馈，且不要包含代码或凭据")
			return
		}
		next.Status, next.Reviewer, next.FeedbackVersion = "draft", actor, current.Version
	case "revise":
		if current.Status != "approved" && current.Status != "draft" {
			fail(w, http.StatusConflict, "只有草稿或已定稿用例可以手工修订")
			return
		}
		if strings.TrimSpace(input.Scenario) == "" {
			fail(w, http.StatusBadRequest, "修订场景不能为空")
			return
		}
		next.Status, next.Scenario, next.Reviewer = "draft", input.Scenario, ""
	case "deprecate":
		next.Status = "deprecated"
	default:
		fail(w, http.StatusBadRequest, "无效的审核操作")
		return
	}
	if input.Decision == "submit_review" || input.Decision == "approve" || input.Decision == "revise" {
		if err := validateCaseDraftScenario(current.Kind, next.Scenario); err != nil {
			fail(w, http.StatusUnprocessableEntity, err.Error())
			return
		}
	}
	if next.Scenario != current.Scenario {
		next.Version++
	}
	s.finishCaseReview(w, r, tid, current, next)
}

func validateCaseDraftScenario(kind, scenario string) error {
	if kind != "functional" && kind != "security" {
		return nil
	}
	_, err := normalizeNonExecutableCaseScenario(kind, scenario)
	return err
}

func (s *Server) reviseCaseDraft(w http.ResponseWriter, r *http.Request, tid string) {
	current, ok := s.readCaseReview(w, r, tid)
	if !ok {
		return
	}
	if current.Status != "draft" || current.Feedback == "" || current.FeedbackVersion != current.Version {
		fail(w, http.StatusConflict, "只有带当前版本退回反馈的草稿可以使用 AI 修订")
		return
	}
	input := caseGenerationInput{Type: current.Kind, ProjectID: current.ProjectID, RequirementID: current.RequirementID}
	if !validCaseDraftType(input.Type) {
		fail(w, http.StatusUnprocessableEntity, "当前用例类型不支持 AI 修订")
		return
	}
	source, err := s.caseRevisionSource(r.Context(), tid, current, input)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取用例生成来源失败")
		return
	}
	source.Scenario, source.Feedback = current.Scenario, current.Feedback
	drafts, ok := s.requestCaseDrafts(w, r.Context(), tid, input, source)
	if !ok {
		return
	}
	if len(drafts) != 1 || string(drafts[0].Scenario) == current.Scenario {
		fail(w, http.StatusBadGateway, "模型没有返回一条包含有效修改的草稿")
		return
	}
	next := current
	next.Version++
	next.Scenario, next.Decision = string(drafts[0].Scenario), "revise_ai"
	s.finishCaseReview(w, r, tid, current, next)
}

func (s *Server) caseRevisionSource(ctx context.Context, tid string, current caseReviewState, input caseGenerationInput) (caseGenerationSource, error) {
	source := caseGenerationSource{Title: current.Name, BaseURL: current.BaseURL}
	err := s.st.DB.QueryRowContext(ctx, `SELECT rev.title, rev.content, src.review_id
		FROM case_generation_sources src JOIN requirement_revisions rev ON rev.tenant_id = src.tenant_id
		AND rev.requirement_id = src.requirement_id AND rev.revision_no = src.revision_no
		WHERE src.tenant_id = ? AND src.case_id = ?`, tid, current.ID).
		Scan(&source.Title, &source.Content, &input.ReviewID)
	if errors.Is(err, sql.ErrNoRows) {
		return source, nil
	}
	if err == nil && input.ReviewID != "" {
		return s.resolveReviewedCaseSource(ctx, tid, input)
	}
	return source, err
}

func (s *Server) finishCaseReview(w http.ResponseWriter, r *http.Request, tid string, current, next caseReviewState) {
	err := s.saveCaseReview(r.Context(), tid, authenticatedActor(r), current, next)
	switch {
	case errors.Is(err, errCaseReviewConflict):
		fail(w, http.StatusConflict, "用例已变化，请刷新后重试")
	case err != nil:
		fail(w, http.StatusInternalServerError, "保存审核或修订失败")
	default:
		write(w, http.StatusOK, next)
	}
}

func (s *Server) saveCaseReview(ctx context.Context, tid, actor string, current, next caseReviewState) error {
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE cases SET status = ?, version_no = ?, scenario = ?, reviewer = ?,
		review_feedback = ?, review_feedback_version = ?, updated_at = datetime('now')
		WHERE id = ? AND tenant_id = ? AND status = ? AND version_no = ? AND review_feedback = ?`,
		next.Status, next.Version, next.Scenario, next.Reviewer, next.Feedback, next.FeedbackVersion,
		current.ID, tid, current.Status, current.Version, current.Feedback)
	if err != nil {
		return err
	}
	if count, err := result.RowsAffected(); err != nil {
		return err
	} else if count != 1 {
		return errCaseReviewConflict
	}
	if next.Version != current.Version {
		if _, err := tx.ExecContext(ctx, `INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario)
			VALUES (?, ?, ?, ?) ON CONFLICT (tenant_id, case_id, version_no) DO NOTHING`, tid, current.ID, current.Version, current.Scenario); err != nil {
			return err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario)
			VALUES (?, ?, ?, ?)`, tid, next.ID, next.Version, next.Scenario); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs (tenant_id, actor, channel, action, target)
		VALUES (?, ?, 'web', ?, ?)`, tid, actor, "case."+next.Decision, next.ID); err != nil {
		return err
	}
	return tx.Commit()
}
