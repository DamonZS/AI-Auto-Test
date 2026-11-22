package api

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"platform/server/internal/store"
)

const (
	scopeWeb       = "web"
	scopeWorker    = "worker"
	scopeMCP       = "mcp"
	sessionTTL     = 8 * time.Hour
	serviceTTL     = 90 * 24 * time.Hour
	passwordRounds = 600_000
)

type authCredential struct {
	id         string
	scope      string
	identityID string
	tenantID   string
}

type serviceCredential struct {
	ID        string    `json:"id"`
	Scope     string    `json:"scope"`
	ExpiresAt time.Time `json:"expires_at"`
}

func ProvisionUser(ctx context.Context, st *store.Store, username, password, tenantID, tenantName, role string) error {
	username = strings.TrimSpace(username)
	if username == "" || len(username) > 128 || len(password) < 12 || len(password) > 1024 {
		return errors.New("需要有效用户名与 12–1024 字节的密码")
	}
	if strings.TrimSpace(tenantID) == "" || (role != "owner" && role != "editor" && role != "viewer") {
		return errors.New("需要租户 ID 和 owner、editor 或 viewer 角色")
	}
	if tenantName != "" && (strings.TrimSpace(tenantName) == "" || len(tenantName) > 128) {
		return errors.New("工作区名称应为 1–128 字节")
	}
	salt := make([]byte, 16)
	rand.Read(salt)
	hash, err := passwordHash(password, salt)
	if err != nil {
		return err
	}
	tx, err := st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if tenantName != "" {
		if _, err := tx.ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, tenantName); err != nil {
			return err
		}
	}
	var tenantExists int
	if err := tx.QueryRowContext(ctx, "SELECT 1 FROM tenants WHERE id = ?", tenantID).Scan(&tenantExists); err != nil {
		return fmt.Errorf("读取配置目标租户: %w", err)
	}
	var identityID string
	if err := tx.QueryRowContext(ctx, `INSERT INTO auth_identities (id, username, password_salt, password_hash)
		VALUES (?, ?, ?, ?) ON CONFLICT (username) DO UPDATE SET
		password_salt = excluded.password_salt, password_hash = excluded.password_hash RETURNING id`,
		id(), username, salt, hash).Scan(&identityID); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO tenant_memberships (identity_id, tenant_id, role) VALUES (?, ?, ?)
		ON CONFLICT (identity_id, tenant_id) DO UPDATE SET role = excluded.role`, identityID, tenantID, role); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, "DELETE FROM auth_tokens WHERE identity_id = ?", identityID); err != nil {
		return err
	}
	return tx.Commit()
}

func (s *Server) createWorkspace(ctx context.Context, tenantID, name, identityID string) error {
	tx, err := s.st.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, "INSERT INTO tenants (id, name) VALUES (?, ?)", tenantID, name); err != nil {
		return err
	}
	if identityID != "" {
		if _, err := tx.ExecContext(ctx, "INSERT INTO tenant_memberships (identity_id, tenant_id, role) VALUES (?, ?, 'owner')", identityID, tenantID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func passwordHash(password string, salt []byte) ([]byte, error) {
	return pbkdf2.Key(sha256.New, password, salt, passwordRounds, sha256.Size)
}

func decodeAuthBody(w http.ResponseWriter, r *http.Request, target any) error {
	body := http.MaxBytesReader(w, r.Body, 8192)
	defer body.Close()
	decoder := json.NewDecoder(body)
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		return errors.New("请求体必须是一个 JSON 值")
	}
	return nil
}

func authFailure(w http.ResponseWriter) {
	w.Header().Set("WWW-Authenticate", "Bearer")
	fail(w, http.StatusUnauthorized, "凭据无效或已过期")
}

func (s *Server) authenticate(w http.ResponseWriter, r *http.Request, scope string) (authCredential, bool) {
	w.Header().Set("Cache-Control", "no-store")
	var secret string
	if header := r.Header.Get("Authorization"); header != "" {
		if fields := strings.Fields(header); len(fields) == 2 && strings.EqualFold(fields[0], "Bearer") {
			secret = fields[1]
		}
	} else if scope != scopeWeb {
		secret = r.Header.Get("X-Agent-Token")
	}
	if len(secret) != base64.RawURLEncoding.EncodedLen(32) {
		authFailure(w)
		return authCredential{}, false
	}
	hash := sha256.Sum256([]byte(secret))
	var credential authCredential
	err := s.st.DB.QueryRowContext(r.Context(), `SELECT id, scope, COALESCE(identity_id, ''), COALESCE(tenant_id, '')
		FROM auth_tokens WHERE token_hash = ? AND expires_at > ?`, hash[:], time.Now().Unix()).
		Scan(&credential.id, &credential.scope, &credential.identityID, &credential.tenantID)
	switch {
	case err == sql.ErrNoRows:
		authFailure(w)
	case err != nil:
		fail(w, http.StatusInternalServerError, "认证服务不可用")
	case credential.scope != scope:
		fail(w, http.StatusForbidden, "凭据范围不匹配")
	default:
		return credential, true
	}
	return authCredential{}, false
}

func (s *Server) webSession(next func(http.ResponseWriter, *http.Request, authCredential)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if credential, ok := s.authenticate(w, r, scopeWeb); ok {
			next(w, r, credential)
		}
	}
}

func (s *Server) workspaceAccount(next func(http.ResponseWriter, *http.Request, authCredential)) http.HandlerFunc {
	if s.legacyAuth {
		return func(w http.ResponseWriter, r *http.Request) { next(w, r, authCredential{}) }
	}
	return s.webSession(next)
}

func newSecret() (string, [sha256.Size]byte) {
	raw := make([]byte, 32)
	rand.Read(raw)
	secret := base64.RawURLEncoding.EncodeToString(raw)
	return secret, sha256.Sum256([]byte(secret))
}

func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	var body struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}
	if err := decodeAuthBody(w, r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return
	}
	username := strings.TrimSpace(body.Username)
	if username == "" || len(username) > 128 || body.Password == "" || len(body.Password) > 1024 {
		fail(w, http.StatusBadRequest, "用户名或密码格式无效")
		return
	}
	var identityID string
	salt, expected := make([]byte, 16), make([]byte, sha256.Size)
	err := s.st.DB.QueryRowContext(r.Context(), "SELECT id, password_salt, password_hash FROM auth_identities WHERE username = ?", username).
		Scan(&identityID, &salt, &expected)
	if err != nil && err != sql.ErrNoRows {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	actual, hashErr := passwordHash(body.Password, salt)
	if hashErr != nil || subtle.ConstantTimeCompare(actual, expected) != 1 || err == sql.ErrNoRows {
		authFailure(w)
		return
	}
	secret, hash := newSecret()
	expires := time.Now().UTC().Truncate(time.Second).Add(sessionTTL)
	result, err := s.st.DB.ExecContext(r.Context(), `INSERT INTO auth_tokens (id, token_hash, scope, identity_id, expires_at)
		SELECT ?, ?, ?, id, ? FROM auth_identities WHERE id = ? AND password_hash = ?`,
		id(), hash[:], scopeWeb, expires.Unix(), identityID, expected)
	if err != nil {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		authFailure(w)
		return
	}
	write(w, http.StatusOK, map[string]any{"token": secret, "expires_at": expires})
}

func (s *Server) logout(w http.ResponseWriter, r *http.Request, credential authCredential) {
	if _, err := s.st.DB.ExecContext(r.Context(), "DELETE FROM auth_tokens WHERE id = ?", credential.id); err != nil {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) issueServiceToken(w http.ResponseWriter, r *http.Request, tenantID string) {
	var body struct {
		Scope string `json:"scope"`
	}
	if err := decodeAuthBody(w, r, &body); err != nil || (body.Scope != scopeWorker && body.Scope != scopeMCP) {
		fail(w, http.StatusBadRequest, "scope 必须为 worker 或 mcp")
		return
	}
	secret, hash := newSecret()
	credential := serviceCredential{id(), body.Scope, time.Now().UTC().Truncate(time.Second).Add(serviceTTL)}
	if _, err := s.st.DB.ExecContext(r.Context(), `INSERT INTO auth_tokens (id, token_hash, scope, tenant_id, expires_at)
		VALUES (?, ?, ?, ?, ?)`, credential.ID, hash[:], credential.Scope, tenantID, credential.ExpiresAt.Unix()); err != nil {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	write(w, http.StatusCreated, struct {
		serviceCredential
		Token string `json:"token"`
	}{credential, secret})
}

func (s *Server) listServiceTokens(w http.ResponseWriter, r *http.Request, tenantID string) {
	rows, err := s.st.DB.QueryContext(r.Context(), "SELECT id, scope, expires_at FROM auth_tokens WHERE tenant_id = ? ORDER BY expires_at, id", tenantID)
	if err != nil {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	defer rows.Close()
	credentials := []serviceCredential{}
	for rows.Next() {
		var credential serviceCredential
		var expires int64
		if err := rows.Scan(&credential.ID, &credential.Scope, &expires); err != nil {
			fail(w, http.StatusInternalServerError, "认证服务不可用")
			return
		}
		credential.ExpiresAt = time.Unix(expires, 0).UTC()
		credentials = append(credentials, credential)
	}
	if err := rows.Err(); err != nil {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	write(w, http.StatusOK, credentials)
}

func (s *Server) revokeServiceToken(w http.ResponseWriter, r *http.Request, tenantID string) {
	result, err := s.st.DB.ExecContext(r.Context(), "DELETE FROM auth_tokens WHERE tenant_id = ? AND id = ?", tenantID, r.PathValue("id"))
	if err != nil {
		fail(w, http.StatusInternalServerError, "认证服务不可用")
		return
	}
	if count, _ := result.RowsAffected(); count == 0 {
		fail(w, http.StatusNotFound, "服务令牌不存在")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
