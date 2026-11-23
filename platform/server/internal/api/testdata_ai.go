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

type aiDataInput struct {
	ProjectID     string   `json:"project_id"`
	EnvironmentID string   `json:"environment_id"`
	Prompt        string   `json:"prompt"`
	Count         int      `json:"count"`
	Fields        []string `json:"fields"`
}

func (s *Server) generateAIDataSet(w http.ResponseWriter, r *http.Request, tenantID string) {
	var input aiDataInput
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 32<<10))
	if err != nil || decodeMCPSamplingJSON(string(body), &input) != nil || strings.TrimSpace(input.ProjectID) == "" || input.Count < 1 || input.Count > 30 {
		fail(w, http.StatusBadRequest, "请选择项目，count 必须在 1 到 30 之间")
		return
	}
	selected, ok := selectAIDataFields(input.Fields)
	input.Prompt = strings.TrimSpace(input.Prompt)
	if !ok || (len(selected) == 0 && input.Prompt == "") {
		fail(w, http.StatusBadRequest, "请选择有效且不重复的字段，或填写测试数据描述")
		return
	}
	if unsafeAIDataText(input.Prompt) {
		fail(w, http.StatusUnprocessableEntity, "测试数据描述不能包含显式凭据或 URL")
		return
	}
	ctx := r.Context()
	var present int
	err = s.st.DB.QueryRowContext(ctx, `SELECT 1 FROM projects p WHERE p.id = ? AND p.tenant_id = ?
		AND (? = '' OR EXISTS (SELECT 1 FROM environments e WHERE e.id = ? AND e.tenant_id = p.tenant_id AND e.project_id = p.id))`,
		input.ProjectID, tenantID, input.EnvironmentID, input.EnvironmentID).Scan(&present)
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "项目或环境不存在")
		return
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, "读取项目或环境失败")
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
		if key != "" && strings.Contains(input.Prompt, key) {
			fail(w, http.StatusUnprocessableEntity, "测试数据描述不能包含模型凭据")
			return
		}
	}
	modelURL := strings.TrimRight(endpoint.String(), "/")
	if !strings.HasSuffix(modelURL, "/chat/completions") {
		modelURL += "/chat/completions"
	}
	completion, err := requestAIDataCompletion(ctx, settings, modelURL, key, input.Prompt, input.Count, selected)
	if err != nil {
		fail(w, http.StatusBadGateway, "模型调用失败，未保存数据集")
		return
	}
	name, fields, rows, ok := validateAIDataCompletion(completion, key, input.Count, selected)
	if !ok {
		fail(w, http.StatusBadGateway, "模型返回了无效测试数据，未保存数据集")
		return
	}
	datasetID, err := s.saveAIDataSet(ctx, tenantID, input, name, fields, rows)
	if err != nil {
		fail(w, http.StatusInternalServerError, "保存测试数据集失败")
		return
	}
	write(w, http.StatusCreated, map[string]any{"id": datasetID, "name": name, "record_count": len(rows)})
}

func (s *Server) saveAIDataSet(ctx context.Context, tenantID string, input aiDataInput, name string, fields []dataField, rows []map[string]string) (string, error) {
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	datasetID := id()
	if _, err = tx.ExecContext(ctx, `INSERT INTO test_data_sets
		(id, tenant_id, project_id, environment_id, name, fields_json, record_count, generation_source)
		VALUES (?,?,?,?,?,?,?,'ai')`, datasetID, tenantID, input.ProjectID, input.EnvironmentID, name, mustJSON(fields), len(rows)); err != nil {
		return "", err
	}
	for index, row := range rows {
		if _, err = tx.ExecContext(ctx, `INSERT INTO test_data_records
			(id, tenant_id, dataset_id, row_no, values_json) VALUES (?,?,?,?,?)`,
			id(), tenantID, datasetID, index+1, mustJSON(row)); err != nil {
			return "", err
		}
	}
	return datasetID, tx.Commit()
}
