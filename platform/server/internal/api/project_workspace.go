package api

import (
	"database/sql"
	"errors"
	"net/http"
	"net/url"
	"strings"

	"platform/server/internal/store"
)

func (s *Server) registerProjectWorkspace(mux *http.ServeMux) {
	mux.HandleFunc("GET /api/projects/{id}", s.tenant(s.getProjectWorkspace))
	mux.HandleFunc("PATCH /api/projects/{id}", s.tenant(s.editProjectWorkspace))
	mux.HandleFunc("POST /api/projects/{id}/environments", s.tenant(s.createProjectEnvironment))
	mux.HandleFunc("PATCH /api/environments/{id}", s.tenant(s.editProjectEnvironment))
}

func (s *Server) getProjectWorkspace(w http.ResponseWriter, r *http.Request, tenantID string) {
	project, err := s.st.GetProjectWorkspace(r.Context(), tenantID, r.PathValue("id"))
	if err != nil {
		failProjectWorkspace(w, err)
		return
	}
	write(w, http.StatusOK, project)
}

func (s *Server) editProjectWorkspace(w http.ResponseWriter, r *http.Request, tenantID string) {
	var edit store.ProjectEdit
	if err := decode(r, &edit); err != nil || !validProjectEdit(edit) {
		fail(w, http.StatusBadRequest, "请填写有效的项目名称、目标地址和项目状态")
		return
	}
	if err := s.st.EditProject(r.Context(), tenantID, r.PathValue("id"), edit); err != nil {
		failProjectWorkspace(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func validProjectEdit(edit store.ProjectEdit) bool {
	if !validWorkspaceTarget(edit.Name, edit.BaseURL) {
		return false
	}
	if edit.Lifecycle != nil {
		switch *edit.Lifecycle {
		case "planning", "active", "completed", "archived":
		default:
			return false
		}
	}
	return true
}

func (s *Server) createProjectEnvironment(w http.ResponseWriter, r *http.Request, tenantID string) {
	edit, ok := decodeEnvironmentEdit(w, r)
	if !ok {
		return
	}
	if edit.Name == nil || edit.BaseURL == nil {
		fail(w, http.StatusBadRequest, "环境名称与地址必填")
		return
	}
	environmentID := id()
	if err := s.st.CreateProjectEnvironment(r.Context(), tenantID, r.PathValue("id"), environmentID, edit); err != nil {
		failProjectWorkspace(w, err)
		return
	}
	write(w, http.StatusCreated, map[string]string{"id": environmentID})
}

func (s *Server) editProjectEnvironment(w http.ResponseWriter, r *http.Request, tenantID string) {
	edit, ok := decodeEnvironmentEdit(w, r)
	if !ok {
		return
	}
	if err := s.st.EditEnvironment(r.Context(), tenantID, r.PathValue("id"), edit); err != nil {
		failProjectWorkspace(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func decodeEnvironmentEdit(w http.ResponseWriter, r *http.Request) (store.EnvironmentEdit, bool) {
	var edit store.EnvironmentEdit
	if err := decode(r, &edit); err != nil || !validWorkspaceTarget(edit.Name, edit.BaseURL) {
		fail(w, http.StatusBadRequest, "名称不能为空，地址须为 HTTP(S) URL")
		return edit, false
	}
	if edit.Source != nil && *edit.Source != "platform" && *edit.Source != "mcp" {
		fail(w, http.StatusBadRequest, "环境来源无效")
		return edit, false
	}
	if edit.Level != nil {
		switch *edit.Level {
		case "", "development", "test", "staging", "production":
		default:
			fail(w, http.StatusBadRequest, "环境级别无效")
			return edit, false
		}
	}
	return edit, true
}

func validWorkspaceTarget(name, baseURL *string) bool {
	if name != nil {
		*name = strings.TrimSpace(*name)
		if *name == "" {
			return false
		}
	}
	if baseURL == nil {
		return true
	}
	*baseURL = strings.TrimSpace(*baseURL)
	parsed, err := url.Parse(*baseURL)
	return err == nil && (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Hostname() != ""
}

func failProjectWorkspace(w http.ResponseWriter, err error) {
	if errors.Is(err, sql.ErrNoRows) {
		fail(w, http.StatusNotFound, "项目、环境或关联版本不存在")
		return
	}
	fail(w, http.StatusInternalServerError, "项目工作台操作失败")
}
