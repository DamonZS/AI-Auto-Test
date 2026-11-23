package api

import (
	"context"
	"crypto/rand"
	"net"
	"net/http"
	"net/netip"
	"strings"
)

const desktopSetupPath = "/api/auth/setup"

type desktopRegistration struct {
	WorkspaceName string `json:"workspace_name"`
	Username      string `json:"username"`
	Password      string `json:"password"`
}

func (s *Server) desktopSetupStatus(w http.ResponseWriter, r *http.Request) {
	var available bool
	if s.desktopSetup {
		if !localSetupRequest(r) {
			fail(w, http.StatusForbidden, "桌面首次注册仅允许本机同源访问")
			return
		}
		if err := s.st.DB.QueryRowContext(r.Context(), "SELECT NOT EXISTS (SELECT 1 FROM auth_identities)").Scan(&available); err != nil {
			fail(w, http.StatusInternalServerError, "无法查询桌面首次注册状态")
			return
		}
	}
	write(w, http.StatusOK, map[string]bool{"available": available})
}

func (s *Server) registerDesktopOwner(w http.ResponseWriter, r *http.Request) {
	if !s.desktopSetup {
		fail(w, http.StatusNotFound, "桌面首次注册未启用")
		return
	}
	if !localSetupRequest(r) {
		fail(w, http.StatusForbidden, "桌面首次注册仅允许本机同源访问")
		return
	}
	var registration desktopRegistration
	if err := decodeAuthBody(w, r, &registration); err != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return
	}
	registration.Username = strings.TrimSpace(registration.Username)
	registration.WorkspaceName = strings.TrimSpace(registration.WorkspaceName)
	if !registration.valid() {
		fail(w, http.StatusBadRequest, "工作区名称和用户名须为 1–128 字节，密码须为 12–1024 字节")
		return
	}
	workspaceID, err := s.createFirstDesktopWorkspace(r.Context(), registration)
	switch {
	case err != nil:
		fail(w, http.StatusInternalServerError, "无法创建桌面账号和工作区")
	case workspaceID == "":
		fail(w, http.StatusConflict, "已有账号，请登录")
	default:
		write(w, http.StatusCreated, map[string]string{"workspace_id": workspaceID})
	}
}

func (registration desktopRegistration) valid() bool {
	return registration.Username != "" && len(registration.Username) <= 128 &&
		registration.WorkspaceName != "" && len(registration.WorkspaceName) <= 128 &&
		len(registration.Password) >= 12 && len(registration.Password) <= 1024
}

func localSetupRequest(r *http.Request) bool {
	local, ok := r.Context().Value(http.LocalAddrContextKey).(*net.TCPAddr)
	remote, err := netip.ParseAddrPort(r.RemoteAddr)
	if !ok || !local.IP.IsLoopback() || err != nil || !remote.Addr().Unmap().IsLoopback() || !loopbackAuthority(r.Host) {
		return false
	}
	origins := r.Header.Values("Origin")
	if len(origins) == 0 {
		return r.Method != http.MethodPost
	}
	scheme := "http://"
	if r.TLS != nil {
		scheme = "https://"
	}
	return len(origins) == 1 && origins[0] == scheme+r.Host
}

func loopbackAuthority(host string) bool {
	endpoint, err := netip.ParseAddrPort(host)
	if err != nil {
		endpoint, err = netip.ParseAddrPort(host + ":80")
	}
	return err == nil && endpoint.Addr().Zone() == "" && endpoint.Addr().Unmap().IsLoopback()
}

func (s *Server) createFirstDesktopWorkspace(ctx context.Context, registration desktopRegistration) (string, error) {
	salt := make([]byte, 16)
	rand.Read(salt)
	hash, err := passwordHash(registration.Password, salt)
	if err != nil {
		return "", err
	}
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	identityID := id()
	result, err := tx.ExecContext(ctx, `INSERT INTO auth_identities (id, username, password_salt, password_hash)
		SELECT ?, ?, ?, ? WHERE NOT EXISTS (SELECT 1 FROM auth_identities)`, identityID, registration.Username, salt, hash)
	if err != nil {
		return "", err
	}
	if count, err := result.RowsAffected(); err != nil || count == 0 {
		return "", err
	}
	workspaceID := id()
	if _, err := tx.ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", workspaceID, registration.WorkspaceName); err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, "INSERT INTO tenant_memberships (identity_id, tenant_id, role) VALUES (?, ?, 'owner')", identityID, workspaceID); err != nil {
		return "", err
	}
	return workspaceID, tx.Commit()
}
