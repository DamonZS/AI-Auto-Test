package api

import (
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"platform/server/internal/store"
)

const requirementImportMaxBytes = 1 << 20

type requirementImportRequest struct {
	ProjectID string `json:"project_id"`
	Filename  string `json:"filename"`
	Text      string `json:"text"`
	Code      string `json:"code"`
	Title     string `json:"title"`
}

func (s *Server) registerRequirements(mux *http.ServeMux) {
	mux.HandleFunc("POST /api/requirements/import", s.tenant(s.importRequirements))
	mux.HandleFunc("POST /api/requirements/documents", s.tenant(s.uploadRequirementDocument))
	mux.HandleFunc("PATCH /api/requirements/{id}", s.tenant(s.reviseRequirement))
	mux.HandleFunc("GET /api/requirements/{id}/history", s.tenant(s.requirementHistory))
	mux.HandleFunc("PUT /api/requirements/{id}/revisions/{revision}/versions/{versionId}", s.tenant(s.linkRequirementVersion))
	mux.HandleFunc("GET /api/versions/{id}/requirement-impact", s.tenant(s.versionRequirementImpact))
}

func (s *Server) importRequirements(w http.ResponseWriter, r *http.Request, tenantID string) {
	var request requirementImportRequest
	body := http.MaxBytesReader(w, r.Body, 7*requirementImportMaxBytes)
	defer body.Close()
	payload, err := io.ReadAll(body)
	var limit *http.MaxBytesError
	if errors.As(err, &limit) {
		fail(w, http.StatusRequestEntityTooLarge, "导入文本不能超过 1 MiB")
		return
	}
	if err != nil || !utf8.Valid(payload) || json.Unmarshal(payload, &request) != nil || request.ProjectID == "" {
		fail(w, http.StatusBadRequest, "需要包含 project_id、filename 和 text 的 UTF-8 JSON")
		return
	}
	if len(request.Text) > requirementImportMaxBytes {
		fail(w, http.StatusRequestEntityTooLarge, "导入文本不能超过 1 MiB")
		return
	}
	items, err := parseRequirementImport(request)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	s.saveImportedRequirements(w, r, tenantID, request.ProjectID, items)
}

func (s *Server) saveImportedRequirements(w http.ResponseWriter, r *http.Request, tenantID, projectID string, items []store.RequirementContent) {
	ids := make([]string, len(items))
	for index := range ids {
		ids[index] = id()
	}
	if err := s.st.ImportRequirements(r.Context(), tenantID, projectID, ids, items); err != nil {
		failRequirementStore(w, err)
		return
	}
	write(w, http.StatusCreated, map[string][]string{"ids": ids})
}

func parseRequirementImport(request requirementImportRequest) ([]store.RequirementContent, error) {
	filename := filepath.Base(strings.TrimSpace(request.Filename))
	extension := filepath.Ext(filename)
	text := strings.TrimPrefix(request.Text, "\ufeff")
	if strings.TrimSpace(text) == "" {
		return nil, errors.New("导入文件不能为空")
	}
	switch strings.ToLower(extension) {
	case ".csv":
		return parseRequirementsCSV(text)
	case ".md", ".markdown", ".txt":
		title := strings.TrimSpace(request.Title)
		if title == "" {
			title = strings.TrimSpace(strings.TrimSuffix(filename, extension))
		}
		if title == "" {
			return nil, errors.New("导入需求标题不能为空")
		}
		return []store.RequirementContent{{Code: strings.TrimSpace(request.Code), Title: title, Content: text}}, nil
	default:
		return nil, errors.New("仅支持 CSV、Markdown 和 TXT 文件")
	}
}

func parseRequirementsCSV(text string) ([]store.RequirementContent, error) {
	reader := csv.NewReader(strings.NewReader(text))
	reader.FieldsPerRecord = 3
	header, err := reader.Read()
	if err != nil {
		return nil, errors.New("CSV 表头必须为 code,title,content 或 编号,标题,内容")
	}
	for index, column := range header {
		header[index] = strings.ToLower(strings.TrimSpace(column))
	}
	columns := strings.Join(header, ",")
	if columns != "code,title,content" && columns != "编号,标题,内容" {
		return nil, errors.New("CSV 表头必须为 code,title,content 或 编号,标题,内容")
	}
	var items []store.RequirementContent
	for {
		fields, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("CSV 格式错误：%w", err)
		}
		title := strings.TrimSpace(fields[1])
		if title == "" {
			line, _ := reader.FieldPos(1)
			return nil, fmt.Errorf("CSV 第 %d 行标题不能为空", line)
		}
		items = append(items, store.RequirementContent{Code: strings.TrimSpace(fields[0]), Title: title, Content: fields[2]})
	}
	if len(items) == 0 {
		return nil, errors.New("CSV 至少包含一条需求")
	}
	return items, nil
}

func (s *Server) reviseRequirement(w http.ResponseWriter, r *http.Request, tenantID string) {
	var edit struct {
		Code    *string `json:"code"`
		Title   *string `json:"title"`
		Content *string `json:"content"`
	}
	if err := decode(r, &edit); err != nil || (edit.Code == nil && edit.Title == nil && edit.Content == nil) {
		fail(w, http.StatusBadRequest, "至少提交 code、title 或 content 中的一项")
		return
	}
	if edit.Title != nil {
		*edit.Title = strings.TrimSpace(*edit.Title)
		if *edit.Title == "" {
			fail(w, http.StatusBadRequest, "需求标题不能为空")
			return
		}
	}
	revision, err := s.st.ReviseRequirement(r.Context(), tenantID, r.PathValue("id"), edit.Code, edit.Title, edit.Content)
	if err != nil {
		failRequirementStore(w, err)
		return
	}
	write(w, http.StatusOK, revision)
}

func (s *Server) requirementHistory(w http.ResponseWriter, r *http.Request, tenantID string) {
	history, err := s.st.RequirementHistory(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		failRequirementStore(w, err)
		return
	}
	write(w, http.StatusOK, history)
}

func (s *Server) linkRequirementVersion(w http.ResponseWriter, r *http.Request, tenantID string) {
	revision, err := strconv.Atoi(r.PathValue("revision"))
	if err != nil || revision < 1 {
		fail(w, http.StatusBadRequest, "需求修订号必须为正整数")
		return
	}
	if err := s.st.LinkRequirementVersion(r.Context(), tenantID, r.PathValue("id"), revision, r.PathValue("versionId")); err != nil {
		failRequirementStore(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) versionRequirementImpact(w http.ResponseWriter, r *http.Request, tenantID string) {
	impact, err := s.st.VersionRequirementImpact(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		failRequirementStore(w, err)
		return
	}
	write(w, http.StatusOK, impact)
}

func failRequirementStore(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "项目、需求修订或版本不存在")
		return
	}
	fail(w, http.StatusInternalServerError, "需求数据保存或读取失败")
}
