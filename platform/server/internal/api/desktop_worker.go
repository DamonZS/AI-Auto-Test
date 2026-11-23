package api

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

const (
	desktopWorkerPath = "/api/desktop/worker"
	desktopWorkerFile = "desktop-worker-token.txt"
)

func (s *Server) registerDesktopWorkerRoutes(mux *http.ServeMux) {
	var activation sync.Mutex
	member := s.member(func(w http.ResponseWriter, r *http.Request, tenantID string) {
		activation.Lock()
		defer activation.Unlock()
		status, err := s.desktopWorkerStatus(r.Context(), tenantID)
		if err != nil {
			fail(w, http.StatusInternalServerError, "无法读取桌面 worker 凭据")
			return
		}
		if r.Method == http.MethodPost {
			if status != "inactive" {
				fail(w, http.StatusConflict, "桌面 worker 已配置")
				return
			}
			if !s.activateDesktopWorker(w, r, tenantID) {
				return
			}
			status = "active"
		}
		write(w, http.StatusOK, map[string]string{"status": status})
	}, true)
	localOwner := func(w http.ResponseWriter, r *http.Request) {
		if !s.desktopSetup || s.desktopWorkerDir == "" {
			fail(w, http.StatusNotFound, "桌面 worker 激活未启用")
			return
		}
		if !localSetupRequest(r) {
			fail(w, http.StatusForbidden, "桌面 worker 激活仅允许本机同源访问")
			return
		}
		member(w, r)
	}
	mux.HandleFunc("GET "+desktopWorkerPath, localOwner)
	mux.HandleFunc("POST "+desktopWorkerPath, localOwner)
}

func (s *Server) desktopWorkerStatus(ctx context.Context, tenantID string) (string, error) {
	manual, err := readDesktopWorkerToken(filepath.Join(s.desktopWorkerDir, "worker-token.txt"))
	if err != nil {
		return "", err
	}
	if manual != "" {
		return "external", nil
	}
	managed, err := readDesktopWorkerToken(filepath.Join(s.desktopWorkerDir, desktopWorkerFile))
	if err != nil {
		return "", err
	}
	owner, err := s.desktopWorkerTokenTenant(ctx, managed)
	switch owner {
	case "":
		return "inactive", err
	case tenantID:
		return "active", err
	default:
		return "other_workspace", err
	}
}

func (s *Server) activateDesktopWorker(w http.ResponseWriter, r *http.Request, tenantID string) bool {
	var body struct {
		Token string `json:"token"`
	}
	if err := decodeAuthBody(w, r, &body); err != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return false
	}
	owner, err := s.desktopWorkerTokenTenant(r.Context(), body.Token)
	switch {
	case err != nil:
		fail(w, http.StatusInternalServerError, "无法验证桌面 worker 凭据")
	case owner != tenantID:
		fail(w, http.StatusBadRequest, "需要当前工作区有效的 worker 令牌")
	case storeDesktopWorkerToken(s.desktopWorkerDir, body.Token) != nil:
		fail(w, http.StatusInternalServerError, "无法保存桌面 worker 凭据")
	default:
		return true
	}
	return false
}

func (s *Server) desktopWorkerTokenTenant(ctx context.Context, token string) (string, error) {
	if len(token) != base64.RawURLEncoding.EncodedLen(32) {
		return "", nil
	}
	hash := sha256.Sum256([]byte(token))
	var tenant string
	err := s.st.DB.QueryRowContext(ctx, `SELECT tenant_id FROM auth_tokens
		WHERE token_hash = ? AND scope = ? AND expires_at > ?`, hash[:], scopeWorker, time.Now().Unix()).Scan(&tenant)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	return tenant, err
}

func readDesktopWorkerToken(path string) (string, error) {
	content, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return "", nil
	}
	return strings.TrimSpace(strings.TrimPrefix(string(content), "\uFEFF")), err
}

func storeDesktopWorkerToken(dir, token string) error {
	file, err := os.CreateTemp(dir, ".desktop-worker-token-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, writeErr := file.WriteString(token + "\n")
	if err := file.Close(); err != nil {
		return err
	}
	if writeErr != nil {
		return writeErr
	}
	return os.Rename(file.Name(), filepath.Join(dir, desktopWorkerFile))
}
