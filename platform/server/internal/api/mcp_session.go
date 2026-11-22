package api

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

const (
	mcpProtocolVersion   = "2025-06-18"
	mcpSessionTTL        = 30 * time.Minute
	mcpSamplingTimeout   = 90 * time.Second
	mcpMaxSessions       = 1024
	mcpMaxSamplingResult = 64 * 1024
)

type mcpIdentity struct {
	tenantID   string
	credential [32]byte
}

func mcpIdentityFromRequest(r *http.Request, tenantID string) mcpIdentity {
	return mcpIdentity{tenantID, sha256.Sum256([]byte(r.Header.Get("X-Agent-Token") + "\x00" + r.Header.Get("Authorization")))}
}

type mcpSession struct {
	server   *Server
	identity mcpIdentity
	expires  time.Time
	sampling bool
	pending  *mcpPendingSample
}

type mcpPendingSample struct {
	id        string
	reply     chan json.RawMessage
	responded bool
}

var mcpSessions = struct {
	sync.Mutex
	entries map[string]*mcpSession
}{entries: make(map[string]*mcpSession)}

func (s *Server) openMCPSession(w http.ResponseWriter, identity mcpIdentity, sampling bool) bool {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()
	now := time.Now()
	for sessionID, session := range mcpSessions.entries {
		if !now.Before(session.expires) && session.pending == nil {
			delete(mcpSessions.entries, sessionID)
		}
	}
	if len(mcpSessions.entries) >= mcpMaxSessions {
		fail(w, http.StatusServiceUnavailable, "MCP 会话已满，请稍后重试")
		return false
	}
	sessionID := uuid.NewString()
	mcpSessions.entries[sessionID] = &mcpSession{s, identity, now.Add(mcpSessionTTL), sampling, nil}
	w.Header().Set("Mcp-Session-Id", sessionID)
	return true
}

func (s *Server) findMCPSession(w http.ResponseWriter, sessionID string, identity mcpIdentity) *mcpSession {
	if sessionID == "" {
		return nil
	}
	mcpSessions.Lock()
	session := mcpSessions.entries[sessionID]
	valid := session != nil && session.server == s && session.identity == identity &&
		(time.Now().Before(session.expires) || session.pending != nil)
	mcpSessions.Unlock()
	if !valid {
		fail(w, http.StatusNotFound, "MCP 会话不存在或已过期")
		return nil
	}
	return session
}

func (session *mcpSession) beginSample() *mcpPendingSample {
	mcpSessions.Lock()
	defer mcpSessions.Unlock()
	if session.pending != nil || !time.Now().Before(session.expires) {
		return nil
	}
	request := &mcpPendingSample{id: uuid.NewString(), reply: make(chan json.RawMessage, 1)}
	session.pending = request
	return request
}

func (session *mcpSession) finishSample(pending *mcpPendingSample) {
	mcpSessions.Lock()
	if session.pending == pending {
		session.pending = nil
	}
	mcpSessions.Unlock()
}

func acceptMCPSamplingReply(w http.ResponseWriter, session *mcpSession, request mcpRequest) {
	var requestID string
	if session == nil || json.Unmarshal(request.ID, &requestID) != nil ||
		(request.Result != nil) == (request.Error != nil) || len(request.Result)+len(request.Error) > mcpMaxSamplingResult {
		fail(w, http.StatusBadRequest, "无效的 Sampling 回包")
		return
	}
	mcpSessions.Lock()
	pending := session.pending
	matched := pending != nil && pending.id == requestID && !pending.responded
	if matched {
		pending.responded = true
		pending.reply <- request.Result
	}
	mcpSessions.Unlock()
	if !matched {
		fail(w, http.StatusBadRequest, "没有匹配的 Sampling 请求")
		return
	}
	w.WriteHeader(http.StatusAccepted)
}

func mcpAcceptsSSE(r *http.Request) bool {
	for _, value := range r.Header.Values("Accept") {
		for _, entry := range strings.Split(value, ",") {
			kind, params, err := mime.ParseMediaType(strings.TrimSpace(entry))
			if err == nil && kind == "text/event-stream" && params["q"] != "0" {
				return true
			}
		}
	}
	return false
}

func writeMCPSamplingEvent(w http.ResponseWriter, message any) error {
	if _, err := fmt.Fprintf(w, "event: message\ndata: %s\n\n", mustJSONString(message)); err != nil {
		return err
	}
	return http.NewResponseController(w).Flush()
}
