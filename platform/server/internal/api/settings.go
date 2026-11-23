package api

import (
	"database/sql"
	"net/http"

	"platform/server/internal/store"
)

type settingsInput struct {
	Provider           string  `json:"provider"`
	Model              string  `json:"model"`
	BaseURL            string  `json:"base_url"`
	APIKey             string  `json:"api_key"`
	MaxSteps           int     `json:"max_steps"`
	MaxTokens          int     `json:"max_tokens"`
	MaxDurationSeconds int     `json:"max_duration_seconds"`
	PlaywrightBrowser  *string `json:"playwright_browser"`
	EgoEnabled         bool    `json:"ego_enabled"`
	LayaEnabled        bool    `json:"laya_enabled"`
	JevEnabled         bool    `json:"jev_enabled"`
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request, tid string) {
	settings, err := s.readSettings(tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	write(w, http.StatusOK, settingsResponse(settings, false))
}

func (s *Server) putSettings(w http.ResponseWriter, r *http.Request, tid string) {
	var input settingsInput
	if err := decode(r, &input); err != nil {
		fail(w, http.StatusBadRequest, "请求体无效")
		return
	}
	hasModel, hasBaseURL := input.Model != "", input.BaseURL != ""
	if input.Provider == "" || hasModel != hasBaseURL || (!hasModel && input.APIKey != "") {
		fail(w, http.StatusBadRequest, "provider 必填，配置模型时须同时提供 model 和 base_url")
		return
	}
	if input.MaxSteps < 1 || input.MaxTokens < 1 || input.MaxDurationSeconds < 1 {
		fail(w, http.StatusBadRequest, "预算必须为正数")
		return
	}
	if input.PlaywrightBrowser != nil {
		switch *input.PlaywrightBrowser {
		case "", "chromium", "firefox", "webkit", "chrome", "msedge":
		default:
			fail(w, http.StatusBadRequest, "不支持的 Playwright 浏览器")
			return
		}
	}
	current, err := s.readSettings(tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	if input.PlaywrightBrowser == nil {
		input.PlaywrightBrowser = &current.PlaywrightBrowser
	}
	ciphertext := current.APIKeyCiphertext
	if input.APIKey != "" {
		ciphertext, err = store.Encrypt(s.st.MasterKey, input.APIKey)
		if err != nil {
			fail(w, http.StatusInternalServerError, err.Error())
			return
		}
	}
	_, err = s.st.DB.Exec(`INSERT INTO tenant_settings
		(tenant_id, provider, model, base_url, api_key_ciphertext, max_steps, max_tokens,
		max_duration_seconds, playwright_browser, ego_enabled, laya_enabled, jev_enabled, updated_at)
		VALUES (?,?,?,?,?,?,?,?,?,?,?,?,datetime('now'))
		ON CONFLICT(tenant_id) DO UPDATE SET provider=excluded.provider, model=excluded.model,
		base_url=excluded.base_url, api_key_ciphertext=excluded.api_key_ciphertext,
		max_steps=excluded.max_steps, max_tokens=excluded.max_tokens,
		max_duration_seconds=excluded.max_duration_seconds, playwright_browser=excluded.playwright_browser,
		ego_enabled=excluded.ego_enabled,
		laya_enabled=excluded.laya_enabled, jev_enabled=excluded.jev_enabled,
		updated_at=datetime('now')`, tid, input.Provider, input.Model, input.BaseURL, ciphertext,
		input.MaxSteps, input.MaxTokens, input.MaxDurationSeconds, *input.PlaywrightBrowser, boolInt(input.EgoEnabled),
		boolInt(input.LayaEnabled), boolInt(input.JevEnabled))
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	s.audit(tid, "", "web", "settings.update", tid)
	write(w, http.StatusOK, settingsResponse(settingsInputToRecord(tid, input, ciphertext), false))
}

func (s *Server) getAgentSettings(w http.ResponseWriter, r *http.Request, tid string) {
	settings, err := s.readSettings(tid)
	if err != nil {
		fail(w, http.StatusInternalServerError, err.Error())
		return
	}
	apiKey, err := store.Decrypt(s.st.MasterKey, settings.APIKeyCiphertext)
	if settings.APIKeyCiphertext != "" && err != nil {
		fail(w, http.StatusInternalServerError, "API key 解密失败")
		return
	}
	response := settingsResponse(settings, true)
	response["api_key"] = apiKey
	write(w, http.StatusOK, response)
}

type settingsRecord struct {
	TenantID           string
	Provider           string
	Model              string
	BaseURL            string
	APIKeyCiphertext   string
	MaxSteps           int
	MaxTokens          int
	MaxDurationSeconds int
	PlaywrightBrowser  string
	EgoEnabled         int
	LayaEnabled        int
	JevEnabled         int
}

func (s *Server) readSettings(tid string) (settingsRecord, error) {
	var value settingsRecord
	err := s.st.DB.QueryRow(`SELECT tenant_id, provider, model, base_url, api_key_ciphertext,
		max_steps, max_tokens, max_duration_seconds, playwright_browser, ego_enabled, laya_enabled, jev_enabled
		FROM tenant_settings WHERE tenant_id = ?`, tid).Scan(&value.TenantID, &value.Provider,
		&value.Model, &value.BaseURL, &value.APIKeyCiphertext, &value.MaxSteps, &value.MaxTokens,
		&value.MaxDurationSeconds, &value.PlaywrightBrowser, &value.EgoEnabled, &value.LayaEnabled, &value.JevEnabled)
	if err == sql.ErrNoRows {
		return settingsRecord{TenantID: tid, Provider: "openai-compatible", MaxSteps: 40, MaxTokens: 20000, MaxDurationSeconds: 600}, nil
	}
	return value, err
}

func settingsInputToRecord(tid string, input settingsInput, ciphertext string) settingsRecord {
	return settingsRecord{TenantID: tid, Provider: input.Provider, Model: input.Model, BaseURL: input.BaseURL,
		APIKeyCiphertext: ciphertext, MaxSteps: input.MaxSteps, MaxTokens: input.MaxTokens,
		MaxDurationSeconds: input.MaxDurationSeconds, PlaywrightBrowser: *input.PlaywrightBrowser, EgoEnabled: boolInt(input.EgoEnabled),
		LayaEnabled: boolInt(input.LayaEnabled), JevEnabled: boolInt(input.JevEnabled)}
}

func settingsResponse(value settingsRecord, includeKey bool) map[string]any {
	response := map[string]any{
		"tenant_id": value.TenantID, "provider": value.Provider, "model": value.Model,
		"base_url": value.BaseURL, "api_key_configured": value.APIKeyCiphertext != "",
		"max_steps": value.MaxSteps, "max_tokens": value.MaxTokens,
		"max_duration_seconds": value.MaxDurationSeconds,
		"playwright_browser":   value.PlaywrightBrowser,
		"ego_enabled":          value.EgoEnabled == 1, "laya_enabled": value.LayaEnabled == 1,
		"jev_enabled": value.JevEnabled == 1,
	}
	if includeKey {
		response["api_key_configured"] = value.APIKeyCiphertext != ""
	}
	return response
}
