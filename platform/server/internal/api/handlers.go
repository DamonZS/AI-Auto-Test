package api

import (
	"context"
	"database/sql"
	"net/http"
	"strings"

	"platform/server/internal/store"
)

const tenantHeader = "X-Tenant-Id"

type authenticatedActorKey struct{}

func authenticatedActor(r *http.Request) string {
	username, _ := r.Context().Value(authenticatedActorKey{}).(string)
	return username
}

func (s *Server) tenant(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return s.tenantRole(next, false)
}

func (s *Server) owner(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return s.tenantRole(next, true)
}

func (s *Server) tenantRole(next func(http.ResponseWriter, *http.Request, string), ownerOnly bool) http.HandlerFunc {
	if !s.legacyAuth {
		return s.member(next, ownerOnly)
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tid := r.Header.Get(tenantHeader)
		if tid == "" {
			fail(w, http.StatusBadRequest, "缺少 X-Tenant-Id")
			return
		}
		var one int
		err := s.st.DB.QueryRow("SELECT 1 FROM tenants WHERE id = ?", tid).Scan(&one)
		if err == sql.ErrNoRows {
			fail(w, http.StatusNotFound, "租户不存在")
			return
		}
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
		next(w, r, tid)
	}
}

func (s *Server) member(next func(http.ResponseWriter, *http.Request, string), ownerOnly bool) http.HandlerFunc {
	return s.webSession(func(w http.ResponseWriter, r *http.Request, credential authCredential) {
		tenantID := r.Header.Get(tenantHeader)
		if tenantID == "" {
			fail(w, http.StatusBadRequest, "缺少 X-Tenant-Id")
			return
		}
		var role, username string
		err := s.st.DB.QueryRowContext(r.Context(), `SELECT membership.role, identity.username
			FROM tenant_memberships AS membership JOIN auth_identities AS identity ON identity.id = membership.identity_id
			WHERE membership.identity_id = ? AND membership.tenant_id = ?`, credential.identityID, tenantID).Scan(&role, &username)
		switch err {
		case sql.ErrNoRows:
			fail(w, http.StatusForbidden, "无权访问此工作区")
		case nil:
			readOnly := r.Method == http.MethodGet || r.Method == http.MethodHead
			if (ownerOnly && role != "owner") || (!readOnly && role == "viewer") {
				fail(w, http.StatusForbidden, "工作区角色权限不足")
				return
			}
			next(w, r.WithContext(context.WithValue(r.Context(), authenticatedActorKey{}, username)), tenantID)
		default:
			fail(w, http.StatusInternalServerError, "认证服务不可用")
		}
	})
}

func (s *Server) agent(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	return s.service(scopeWorker, next)
}

func (s *Server) service(scope string, next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
	if !s.legacyAuth {
		return func(w http.ResponseWriter, r *http.Request) {
			credential, ok := s.authenticate(w, r, scope)
			if ok {
				next(w, r, credential.tenantID)
			}
		}
	}
	return func(w http.ResponseWriter, r *http.Request) {
		tok := r.Header.Get("X-Agent-Token")
		tid, ok := strings.CutPrefix(tok, "tenant:")
		if !ok || tid == "" {
			fail(w, http.StatusUnauthorized, "设备令牌无效")
			return
		}
		var one int
		if err := s.st.DB.QueryRow("SELECT 1 FROM tenants WHERE id = ?", tid).Scan(&one); err != nil {
			fail(w, http.StatusUnauthorized, "设备令牌无效")
			return
		}
		next(w, r, tid)
	}
}

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request, credential authCredential) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decodeAuthBody(w, r, &body); err != nil || strings.TrimSpace(body.Name) == "" || len(body.Name) > 128 {
		fail(w, http.StatusBadRequest, "name 必填且不能超过 128 字节")
		return
	}
	tid := id()
	if err := s.createWorkspace(r.Context(), tid, body.Name, credential.identityID); err != nil {
		fail(w, http.StatusInternalServerError, "创建工作区失败")
		return
	}
	write(w, http.StatusOK, map[string]string{"id": tid, "name": body.Name})
}

func (s *Server) listTenants(w http.ResponseWriter, r *http.Request, credential authCredential) {
	var rows *sql.Rows
	var err error
	if s.legacyAuth {
		rows, err = s.st.DB.QueryContext(r.Context(), "SELECT * FROM tenants ORDER BY created_at, id")
	} else {
		rows, err = s.st.DB.QueryContext(r.Context(), `SELECT tenants.*, tenant_memberships.role FROM tenants
			JOIN tenant_memberships ON tenant_memberships.tenant_id = tenants.id
			WHERE tenant_memberships.identity_id = ? ORDER BY tenants.created_at, tenants.id`, credential.identityID)
	}
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, err := rowsToMaps(rows)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, out)
}

func (s *Server) listProjects(w http.ResponseWriter, r *http.Request, tid string) {
	rows, err := s.st.DB.Query("SELECT * FROM projects WHERE tenant_id = ? ORDER BY created_at", tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	out, _ := rowsToMaps(rows)
	write(w, http.StatusOK, out)
}

func (s *Server) createProject(w http.ResponseWriter, r *http.Request, tid string) {
	var body store.ProjectEdit
	if err := decode(r, &body); err != nil || body.Name == nil || body.BaseURL == nil || !validProjectEdit(body) {
		fail(w, http.StatusBadRequest, "请填写有效的项目名称、目标地址和项目状态")
		return
	}
	pid := id()
	if err := s.st.CreateProject(r.Context(), tid, pid, body); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": pid})
}

func (s *Server) overview(w http.ResponseWriter, r *http.Request, tid string) {
	count := func(q string, args ...any) int {
		var n int
		_ = s.st.DB.QueryRow(q, args...).Scan(&n)
		return n
	}
	write(w, http.StatusOK, map[string]int{
		"projects":          count("SELECT count(*) FROM projects WHERE tenant_id = ?", tid),
		"requirements":      count("SELECT count(*) FROM requirements WHERE tenant_id = ?", tid),
		"cases":             count("SELECT count(*) FROM cases WHERE tenant_id = ?", tid),
		"approved":          count("SELECT count(*) FROM cases WHERE tenant_id = ? AND status = 'approved'", tid),
		"runs":              count("SELECT count(*) FROM runs WHERE tenant_id = ?", tid),
		"open_bugs":         count("SELECT count(*) FROM bugs WHERE tenant_id = ? AND status NOT IN ('closed','rejected')", tid),
		"patches":           count("SELECT count(*) FROM heal_patches WHERE tenant_id = ? AND status = 'pending'", tid),
		"test_data_sets":    count("SELECT count(*) FROM test_data_sets WHERE tenant_id = ?", tid),
		"test_data_records": count("SELECT COALESCE(sum(record_count), 0) FROM test_data_sets WHERE tenant_id = ?", tid),
	})
}
