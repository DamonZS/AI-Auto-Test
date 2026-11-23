package api

import (
	"errors"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"platform/server/internal/store"
)

const requirementDocumentMaxBytes = 10 << 20

func (s *Server) uploadRequirementDocument(w http.ResponseWriter, r *http.Request, tenantID string) {
	projectID := r.URL.Query().Get("project_id")
	var project string
	if err := s.st.DB.QueryRowContext(r.Context(), "SELECT id FROM projects WHERE id = ? AND tenant_id = ?", projectID, tenantID).Scan(&project); err != nil {
		failRequirementStore(w, err)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, requirementDocumentMaxBytes+(64<<10))
	filename, data, err := readRequirementUpload(r)
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) || len(data) > requirementDocumentMaxBytes {
		fail(w, http.StatusRequestEntityTooLarge, "文件不能超过 10 MiB")
		return
	}
	if err != nil {
		fail(w, http.StatusBadRequest, "请选择一个需求说明书文件")
		return
	}
	items, err := parseRequirementDocument(filename, data)
	if err != nil {
		fail(w, http.StatusBadRequest, err.Error())
		return
	}
	if r.URL.Query().Get("preview") == "1" {
		write(w, http.StatusOK, map[string]any{"items": items})
		return
	}
	s.saveImportedRequirements(w, r, tenantID, projectID, items)
}

func readRequirementUpload(r *http.Request) (string, []byte, error) {
	reader, err := r.MultipartReader()
	if err != nil {
		return "", nil, err
	}
	part, err := reader.NextPart()
	if err != nil {
		return "", nil, err
	}
	if part.FormName() != "file" || part.FileName() == "" {
		return "", nil, errors.New("file 字段缺少文件")
	}
	data, err := io.ReadAll(io.LimitReader(part, requirementDocumentMaxBytes+1))
	return part.FileName(), data, err
}

func parseRequirementDocument(filename string, data []byte) ([]store.RequirementContent, error) {
	filename = filepath.Base(filename)
	extension := strings.ToLower(filepath.Ext(filename))
	title := strings.TrimSpace(filename[:len(filename)-len(extension)])
	var items []store.RequirementContent
	var err error
	switch extension {
	case ".xlsx":
		items, err = parseRequirementXLSX(data)
	case ".pdf", ".docx":
		var text string
		text, err = extractRequirementDocumentText(extension, data)
		if err == nil {
			items = []store.RequirementContent{{Title: title, Content: text}}
		}
	case ".csv", ".md", ".markdown", ".txt":
		if !utf8.Valid(data) {
			return nil, errors.New("CSV、Markdown 和 TXT 文件须使用 UTF-8 编码")
		}
		items, err = parseRequirementImport(requirementImportRequest{Filename: filename, Text: string(data)})
	default:
		return nil, errors.New("请选择 PDF、DOCX、XLSX、CSV、Markdown 或 TXT 文件")
	}
	if err != nil {
		return nil, err
	}
	if len(items) == 0 || len(items) > 500 {
		return nil, errors.New("每份文件须包含 1 至 500 条需求")
	}
	var total int
	for index := range items {
		item := &items[index]
		if item.Title == "" {
			item.Title = title
		}
		if item.Title == "" {
			return nil, errors.New("需求标题不能为空")
		}
		total += len(item.Code) + len(item.Title) + len(item.Content)
	}
	if total > requirementImportMaxBytes {
		return nil, errors.New("提取的需求文本超过 1 MiB，请拆分文件")
	}
	return items, nil
}
