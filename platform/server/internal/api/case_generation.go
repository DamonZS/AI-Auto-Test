package api

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"net/http"
	"strings"

	"platform/server/internal/store"
)

type caseGenerationInput struct {
	ProjectID     string `json:"project_id"`
	RequirementID string `json:"requirement_id"`
	VersionID     string `json:"version_id"`
	Type          string `json:"type"`
}

type caseGenerationSource struct {
	RevisionNo int
	Title      string
	Content    string
	BaseURL    string
}

func (s *Server) registerCaseGenerationRoutes(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/cases/generate", s.tenant(s.generateRequirementCases))
}

func (s *Server) generateRequirementCases(w http.ResponseWriter, r *http.Request, tenantID string) {
	var input caseGenerationInput
	defer r.Body.Close()
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 4096))
	if err != nil || decodeMCPSamplingJSON(string(body), &input) != nil ||
		strings.TrimSpace(input.ProjectID) == "" || strings.TrimSpace(input.RequirementID) == "" ||
		(input.Type != "ui" && input.Type != "api") {
		fail(w, http.StatusBadRequest, "请选择项目、需求和有效用例类型（ui 或 api）")
		return
	}
	ctx := r.Context()
	source, err := s.resolveCaseGenerationSource(ctx, tenantID, input)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "需求或版本不存在或不属于当前项目")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取需求修订失败")
		return
	}
	if source.RevisionNo == 0 {
		fail(w, http.StatusUnprocessableEntity, "所选版本没有关联的需求修订")
		return
	}
	prose := source.Title + "\n" + source.Content
	if len(prose) > 32<<10 || caseGenerationPrivateInput.MatchString(prose) {
		fail(w, http.StatusUnprocessableEntity, "请使用不含代码块或凭据且不超过 32 KiB 的需求描述")
		return
	}
	if _, ok := caseGenerationURL(source.BaseURL); input.Type == "ui" && !ok {
		fail(w, http.StatusUnprocessableEntity, "项目首页须为不含凭据、参数或片段的 HTTP(S) 地址")
		return
	}
	settings, err := s.readSettings(tenantID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取租户模型配置失败")
		return
	}
	endpoint, ok := caseGenerationURL(settings.BaseURL)
	if !ok || strings.TrimSpace(settings.Model) == "" || settings.MaxTokens < 1 || settings.MaxDurationSeconds < 1 {
		fail(w, http.StatusUnprocessableEntity, "请先配置租户的 OpenAI 兼容模型")
		return
	}
	var key string
	if settings.APIKeyCiphertext != "" {
		key, err = store.Decrypt(s.st.MasterKey, settings.APIKeyCiphertext)
		if err != nil {
			fail(w, http.StatusInternalServerError, "读取租户模型凭据失败")
			return
		}
		if key != "" && strings.Contains(prose, key) {
			fail(w, http.StatusUnprocessableEntity, "需求描述不能包含模型凭据")
			return
		}
	}
	modelURL := strings.TrimRight(endpoint.String(), "/")
	if !strings.HasSuffix(modelURL, "/chat/completions") {
		modelURL += "/chat/completions"
	}
	drafts, err := generateCasesWithModel(ctx, settings, modelURL, key, input, source)
	if err != nil {
		fail(w, http.StatusBadGateway, "模型调用失败或没有生成可用的测试草稿")
		return
	}
	saved, err := s.saveRequirementCaseDrafts(ctx, tenantID, authenticatedActor(r), input, source.RevisionNo, drafts)
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存测试草稿失败")
		return
	}
	result := map[string]any{
		"project_id": input.ProjectID, "requirement_id": input.RequirementID, "revision_no": source.RevisionNo,
		"generation_mode": "platform_llm", "cases": saved,
	}
	if input.VersionID != "" {
		result["version_id"] = input.VersionID
	}
	write(w, http.StatusCreated, result)
}

func (s *Server) resolveCaseGenerationSource(ctx context.Context, tenantID string, input caseGenerationInput) (caseGenerationSource, error) {
	var source caseGenerationSource
	err := s.st.DB.QueryRowContext(ctx, `SELECT COALESCE(rev.revision_no, 0), COALESCE(rev.title, ''), COALESCE(rev.content, ''), p.base_url
		FROM requirements q JOIN projects p ON p.id = q.project_id AND p.tenant_id = q.tenant_id
		LEFT JOIN versions v ON v.id = ? AND v.tenant_id = p.tenant_id AND v.project_id = p.id
		LEFT JOIN requirement_revisions rev ON rev.tenant_id = q.tenant_id AND rev.requirement_id = q.id
			AND (? = '' OR EXISTS (SELECT 1 FROM requirement_version_links link
				WHERE link.tenant_id = rev.tenant_id AND link.requirement_id = rev.requirement_id
				AND link.revision_no = rev.revision_no AND link.version_id = v.id))
		WHERE q.id = ? AND q.tenant_id = ? AND p.id = ? AND (? = '' OR v.id IS NOT NULL)
		ORDER BY rev.revision_no DESC LIMIT 1`, input.VersionID, input.VersionID,
		input.RequirementID, tenantID, input.ProjectID, input.VersionID).
		Scan(&source.RevisionNo, &source.Title, &source.Content, &source.BaseURL)
	return source, err
}

func (s *Server) saveRequirementCaseDrafts(ctx context.Context, tenantID, actor string, input caseGenerationInput, revision int, drafts []mcpSampledCase) ([]mcpCaseDraft, error) {
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	saved := make([]mcpCaseDraft, 0, len(drafts))
	for _, draft := range drafts {
		caseID := id()
		scenario := string(draft.Scenario)
		if _, err := tx.ExecContext(ctx, `INSERT INTO cases
			(id, tenant_id, project_id, requirement_id, name, type, scenario, status, expected_source, generation_mode)
			VALUES (?, ?, ?, ?, ?, ?, ?, 'draft', 'requirement', 'platform_llm')`,
			caseID, tenantID, input.ProjectID, input.RequirementID, draft.Name, draft.Type, scenario); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO case_revisions (tenant_id, case_id, version_no, scenario)
			VALUES (?, ?, 1, ?)`, tenantID, caseID, scenario); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO case_generation_sources (tenant_id, case_id, requirement_id, revision_no, version_id)
			VALUES (?, ?, ?, ?, ?)`, tenantID, caseID, input.RequirementID, revision, input.VersionID); err != nil {
			return nil, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO audit_logs (tenant_id, actor, channel, action, target)
			VALUES (?, ?, 'web', 'case.generate.draft', ?)`, tenantID, actor, caseID); err != nil {
			return nil, err
		}
		saved = append(saved, mcpCaseDraft{ID: caseID, Name: draft.Name, Type: draft.Type, Status: "draft"})
	}
	return saved, tx.Commit()
}
