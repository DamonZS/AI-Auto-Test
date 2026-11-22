package api

import (
	"bytes"
	"database/sql"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"platform/server/internal/store"
)

const maxCoverageContent = 1 << 20

func (s *Server) registerCoverage(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/versions/{id}/coverage", s.tenant(s.getVersionCoverage))
	mux.HandleFunc("POST /api/versions/{id}/coverage", s.tenant(s.importVersionCoverage))
}

func (s *Server) getVersionCoverage(w http.ResponseWriter, r *http.Request, tenantID string) {
	snapshots, err := store.ListVersionCoverage(r.Context(), s.st.DB, tenantID, r.PathValue("id"))
	writeVersionCoverage(w, snapshots, err)
}

func (s *Server) importVersionCoverage(w http.ResponseWriter, r *http.Request, tenantID string) {
	input, ok := readCoverageInput(w, r)
	if !ok {
		return
	}
	var units, branches store.CoverageCount
	var err error
	switch input.Format {
	case store.CoverageGo:
		units, err = parseGoCoverage(input.Content)
	case store.CoverageCobertura:
		units, branches, err = parseCoberturaCoverage(input.Content)
	default:
		fail(w, http.StatusUnprocessableEntity, "仅支持 Go coverprofile 和 Cobertura XML")
		return
	}
	if err != nil {
		fail(w, http.StatusUnprocessableEntity, "覆盖率报告格式或计数无效")
		return
	}
	snapshot, err := s.st.SaveVersionCoverage(r.Context(), tenantID, r.PathValue("id"), input.Format, units, branches)
	writeVersionCoverage(w, snapshot, err)
}

type coverageInput struct {
	Format  string `json:"format"`
	Content string `json:"content"`
}

func readCoverageInput(w http.ResponseWriter, r *http.Request) (coverageInput, bool) {
	var input coverageInput
	body, err := io.ReadAll(http.MaxBytesReader(w, r.Body, 2*maxCoverageContent))
	var tooLarge *http.MaxBytesError
	if errors.As(err, &tooLarge) {
		fail(w, http.StatusRequestEntityTooLarge, "请求体超过 2 MiB")
		return input, false
	}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.DisallowUnknownFields()
	if err != nil || decoder.Decode(&input) != nil || decoder.Decode(new(json.RawMessage)) != io.EOF || input.Format == "" || input.Content == "" {
		fail(w, http.StatusBadRequest, "请提交包含 format 和 content 的 JSON")
		return input, false
	}
	if len(input.Content) > maxCoverageContent {
		fail(w, http.StatusRequestEntityTooLarge, "覆盖率报告超过 1 MiB")
		return input, false
	}
	return input, true
}

func writeVersionCoverage(w http.ResponseWriter, result any, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		fail(w, http.StatusNotFound, "版本不存在")
	case err != nil:
		fail(w, http.StatusInternalServerError, "无法访问版本覆盖率")
	default:
		write(w, http.StatusOK, result)
	}
}
