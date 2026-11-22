package api

import (
	"database/sql"
	"net/http"
	"strings"
)

const tenantHeader = "X-Tenant-Id"

func (s *Server) tenant(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
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

// agent 校验设备令牌，令牌即租户绑定（首版：令牌形如 tenant:<id>）。
func (s *Server) agent(next func(http.ResponseWriter, *http.Request, string)) http.HandlerFunc {
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

func (s *Server) createTenant(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Name string `json:"name"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" {
		fail(w, http.StatusBadRequest, "name 必填")
		return
	}
	tid := id()
	if _, err := s.st.DB.Exec("INSERT INTO tenants (id, name) VALUES (?, ?)", tid, body.Name); err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, map[string]string{"id": tid, "name": body.Name})
}

func (s *Server) listTenants(w http.ResponseWriter, r *http.Request) {
	rows, err := s.st.DB.Query("SELECT * FROM tenants ORDER BY created_at")
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
	var body struct {
		Name         string `json:"name"`
		BaseURL      string `json:"base_url"`
		AllowedHosts string `json:"allowed_hosts"`
	}
	if err := decode(r, &body); err != nil || body.Name == "" || body.BaseURL == "" {
		fail(w, http.StatusBadRequest, "name 与 base_url 必填")
		return
	}
	pid := id()
	_, err := s.st.DB.Exec(
		"INSERT INTO projects (id, tenant_id, name, base_url, allowed_hosts) VALUES (?, ?, ?, ?, ?)",
		pid, tid, body.Name, body.BaseURL, body.AllowedHosts)
	if err != nil {
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
