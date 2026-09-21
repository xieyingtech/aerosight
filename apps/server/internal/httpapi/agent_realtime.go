package httpapi

import (
	"aerosight/server/internal/agent"
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/gorilla/websocket"
)

type realtimeConfig struct{ Protocol, Model string }

// A compact dispatcher avoids ambiguous voice-tool selection while execution
// still goes through exactly the same scoped read tools as text chat.
var realtimeResources = map[string]string{"devices": "query_devices", "tasks": "query_tasks", "issues": "query_issues", "assets": "query_assets", "tracks": "query_tracks", "map": "query_map_context"}

func stepRealtimeSession() gin.H {
	tools := []gin.H{{"type": "function", "function": gin.H{"name": "query_project", "description": "查询当前项目的数据。支持设备、任务、案件、资产、轨迹和地图。", "parameters": gin.H{"type": "object", "properties": gin.H{"resource": gin.H{"type": "string", "enum": []string{"devices", "tasks", "issues", "assets", "tracks", "map"}, "description": "查询的数据类型，设备选devices，任务选tasks"}}, "required": []string{"resource"}, "additionalProperties": false}}}}
	return gin.H{"modalities": []string{"text", "audio"}, "voice": "linjiajiejie",
		"instructions":       "你是 AeroSight 项目智能体。用户询问项目情况时必须调用 query_project 查询真实数据。调用工具前不要说话，获得工具结果后再用中文简短回答。不能执行设备或算法操作。以工具返回的数据时间和质量为准，空结果说明暂无记录。",
		"input_audio_format": "pcm16", "output_audio_format": "pcm16", "input_audio_transcription": gin.H{"model": "whisper-1"},
		"turn_detection": gin.H{"type": "server_vad", "prefix_padding_ms": 500, "silence_duration_ms": 600}, "tools": tools, "tool_choice": "auto"}
}

func realtimeReadTool(item stepRealtimeItem) (string, error) {
	if item.Name != "query_project" {
		return "", errors.New("AGENT_TOOL_NOT_ALLOWED")
	}
	var args map[string]json.RawMessage
	if json.Unmarshal([]byte(item.Arguments), &args) != nil || len(args) != 1 {
		return "", errors.New("AGENT_TOOL_ARGUMENTS_INVALID")
	}
	var resource string
	if json.Unmarshal(args["resource"], &resource) != nil || realtimeResources[resource] == "" {
		return "", errors.New("AGENT_TOOL_ARGUMENTS_INVALID")
	}
	return realtimeResources[resource], nil
}

func realtimeURL(base, model string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimRight(base, "/"))
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || strings.TrimSpace(model) == "" {
		return nil, errors.New("AI_REALTIME_PROVIDER_REQUIRED")
	}
	u.Path += "/realtime"
	u.RawQuery = url.Values{"model": {model}}.Encode()
	return u, nil
}

func (s *Server) connectRealtime(ctx context.Context) (*websocket.Conn, realtimeConfig, error) {
	config := realtimeConfig{}
	providers, err := s.queries.ReadDefaultChatProvider(ctx)
	if err != nil {
		return nil, config, err
	}
	if len(providers) != 1 {
		return nil, config, errors.New("AI_REALTIME_PROVIDER_REQUIRED")
	}
	p := providers[0]
	config = realtimeConfig{Protocol: p.RealtimeProtocol, Model: p.RealtimeModelID}
	if config.Protocol != "stepfun" || strings.TrimSpace(config.Model) == "" {
		return nil, config, errors.New("AI_REALTIME_PROVIDER_REQUIRED")
	}
	base := p.BaseUrl.String
	target, err := realtimeURL(base, config.Model)
	if err != nil {
		return nil, config, err
	}
	if s.realtimeConnect != nil {
		conn, err := s.realtimeConnect(ctx)
		return conn, config, err
	}
	var envelope credentials.Envelope
	var secret struct {
		APIKey string `json:"apiKey"`
	}
	if json.Unmarshal(p.CredentialEnvelopeJson, &envelope) != nil || credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("ai-provider", p.ID, nil), &secret) != nil || secret.APIKey == "" {
		return nil, config, errors.New("AI_PROVIDER_CREDENTIAL_UNAVAILABLE")
	}
	target, addresses, err := s.resolveOutboundURL(ctx, target.String(), []string{target.Hostname()})
	if err != nil {
		return nil, config, err
	}
	transport := pinnedAIHTTPClient(target, addresses).Transport.(*http.Transport)
	target.Scheme = "wss"
	dialer := websocket.Dialer{HandshakeTimeout: 15 * time.Second, NetDialContext: transport.DialContext}
	conn, response, err := dialer.DialContext(ctx, target.String(), http.Header{"Authorization": {"Bearer " + secret.APIKey}})
	if err != nil {
		if response != nil && response.Body != nil {
			response.Body.Close()
		}
		return nil, config, errors.New("AI_REALTIME_CONNECT_FAILED")
	}
	return conn, config, nil
}

type realtimeMessage struct {
	ID        int32   `json:"id"`
	Role      string  `json:"role"`
	Content   string  `json:"content"`
	ToolCalls []gin.H `json:"toolCalls"`
	CreatedAt any     `json:"createdAt"`
	dirty     bool
}

// Allocate message IDs in conversation order, then update their final text.
// ASR completion can arrive after the assistant has already started speaking.
type realtimeConversation struct {
	s                *Server
	uid, pid, sid    int32
	requestID        string
	config           realtimeConfig
	items            map[string]*realtimeMessage
	order            []string
	calls            map[string]bool
	toolCount        int
	continueResponse bool
	emit             func(string, any) error
	write            func(any) error
}

func (r *realtimeConversation) message(ctx context.Context, id, role string) (*realtimeMessage, error) {
	if m := r.items[id]; m != nil {
		return m, nil
	}
	if id == "" || len(r.items) >= 500 {
		return nil, errors.New("AI_REALTIME_LIMIT")
	}
	saved, err := r.s.appendAgentMessage(ctx, r.uid, r.pid, r.sid, role, "", nil, r.requestID)
	if err != nil {
		return nil, err
	}
	m := &realtimeMessage{ID: saved["id"].(int32), Role: role, ToolCalls: []gin.H{}, CreatedAt: saved["createdAt"]}
	r.items[id] = m
	r.order = append(r.order, id)
	return m, nil
}

func (r *realtimeConversation) publish(m *realtimeMessage) error {
	m.dirty = true
	text, _ := agent.SanitizeChatMessage(m.Content, nil)
	copy := *m
	copy.Content = text
	return r.emit("message", &copy)
}

func (r *realtimeConversation) save(ctx context.Context, m *realtimeMessage) error {
	if !m.dirty {
		return nil
	}
	access, err := r.s.projectAccess(ctx, r.s.queries, r.uid, r.pid, "agent:use")
	if err != nil {
		return errors.New("PROJECT_ACCESS_DENIED")
	}
	text, calls := agent.SanitizeChatMessage(m.Content, m.ToolCalls)
	encoded, _ := json.Marshal(calls)
	audit := database.AuditContext{ProjectID: r.pid, TeamID: access.TeamID, ActorUserID: r.uid, RequestID: r.requestID, Action: "agent_message.transcribe", ResourceType: "agent_session", ResourceID: strconv.Itoa(int(r.sid)), Input: gin.H{"role": m.Role}, PolicyResult: gin.H{"permission": "agent:use", "retention": "minimal"}}
	_, err = database.AuditedWrite(ctx, r.s.db, audit, r.s.authorizeWrite(r.uid, r.pid, access.TeamID, "agent:use", false), func(w *database.WriteTx) (gin.H, error) {
		if _, e := w.Queries.LockChatSession(ctx, sqlcgen.LockChatSessionParams{ID: r.sid, ProjectID: r.pid, StartedByUserID: sql.NullInt32{Int32: r.uid, Valid: true}}); e != nil {
			return nil, e
		}
		n, e := w.Queries.UpdateRealtimeChatMessage(ctx, sqlcgen.UpdateRealtimeChatMessageParams{ID: m.ID, SessionID: r.sid, Content: text, ToolCallsJson: encoded})
		if e == nil && n != 1 {
			e = errors.New("AGENT_SESSION_NOT_FOUND")
		}
		return gin.H{"id": m.ID}, e
	})
	if err == nil {
		m.dirty = false
	}
	return err
}

type stepRealtimeItem struct {
	ID, Type, Role, Name, Arguments string
	CallID                          string `json:"call_id"`
	Content                         []struct{ Type, Text, Transcript string }
}
type stepRealtimeEvent struct {
	Type                    string `json:"type"`
	ItemID                  string `json:"item_id"`
	Delta, Transcript, Text string
	Item                    stepRealtimeItem
	Response                struct {
		Status string
		Output []stepRealtimeItem
	}
}

func (r *realtimeConversation) handle(ctx context.Context, e stepRealtimeEvent) error {
	switch e.Type {
	case "session.created":
		return r.write(gin.H{"type": "session.update", "session": stepRealtimeSession()})
	case "session.updated":
		return r.emit("ready", gin.H{"model": r.config.Model, "sampleRate": 24000})
	case "input_audio_buffer.speech_started":
		r.toolCount = 0
		if _, err := r.message(ctx, e.ItemID, "user"); err != nil {
			return err
		}
		return r.emit("interrupted", gin.H{})
	case "input_audio_buffer.speech_stopped":
		return r.emit("status", gin.H{"message": "正在理解你的话…"})
	case "conversation.item.created", "response.output_item.added":
		if e.Item.Type == "message" && (e.Item.Role == "user" || e.Item.Role == "assistant") {
			_, err := r.message(ctx, e.Item.ID, e.Item.Role)
			return err
		}
	case "conversation.item.input_audio_transcription.delta", "conversation.item.input_audio_transcription.completed", "conversation.item.input_audio_transcription.failed", "response.audio_transcript.delta", "response.audio_transcript.done", "response.text.delta", "response.text.done":
		role := "assistant"
		if strings.HasPrefix(e.Type, "conversation.") {
			role = "user"
		}
		m, err := r.message(ctx, e.ItemID, role)
		if err != nil {
			return err
		}
		switch {
		case strings.HasSuffix(e.Type, ".delta"):
			m.Content += e.Delta
		case strings.HasSuffix(e.Type, ".failed"):
			m.Content = "（这段语音未能识别，请重说）"
		case e.Transcript != "":
			m.Content = e.Transcript
		case e.Text != "":
			m.Content = e.Text
		}
		if len(m.Content) > 80000 {
			return errors.New("AI_REALTIME_LIMIT")
		}
		if err = r.publish(m); err != nil {
			return err
		}
		if !strings.HasSuffix(e.Type, ".delta") {
			return r.save(ctx, m)
		}
	case "response.audio.delta":
		// Audio is relayed transiently; it is never retained in the database.
		return r.emit("audio", gin.H{"itemId": e.ItemID, "delta": e.Delta})
	case "response.output_item.done":
		if e.Item.Type == "function_call" {
			return r.tool(ctx, e.Item)
		}
	case "response.done":
		if e.Response.Status == "failed" {
			return errors.New("AI_REALTIME_UPSTREAM_FAILED")
		}
		// StepFun also documents calls delivered only in the final output list.
		for _, item := range e.Response.Output {
			if item.Type == "function_call" && e.Response.Status != "cancelled" {
				if item.ID == "" {
					item.ID = item.CallID
				}
				if err := r.tool(ctx, item); err != nil {
					return err
				}
			}
		}
		if r.continueResponse && e.Response.Status != "cancelled" {
			r.continueResponse = false
			return r.write(gin.H{"type": "response.create"})
		}
		r.continueResponse = false
		return r.emit("status", gin.H{"message": "正在聆听，可以继续说话"})
	case "conversation.item.truncated":
		if m := r.items[e.ItemID]; m != nil && !strings.HasSuffix(m.Content, "（已打断）") {
			m.Content += "\n\n（已打断）"
			if err := r.publish(m); err != nil {
				return err
			}
			return r.save(ctx, m)
		}
	case "error":
		return errors.New("AI_REALTIME_UPSTREAM_FAILED")
	}
	return nil
}

func (r *realtimeConversation) tool(ctx context.Context, item stepRealtimeItem) error {
	if item.CallID == "" {
		return errors.New("AI_REALTIME_UPSTREAM_FAILED")
	}
	if r.calls[item.CallID] {
		return nil
	}
	r.calls[item.CallID] = true
	r.toolCount++
	if r.toolCount > 8 {
		return errors.New("AGENT_TOOL_STEP_LIMIT")
	}
	if err := r.s.checkChatAccess(ctx, r.uid, r.pid, r.sid); err != nil {
		return err
	}
	name, err := realtimeReadTool(item)
	if err != nil {
		return err
	}
	m, err := r.message(ctx, item.ID, "assistant")
	if err != nil {
		return err
	}
	m.ToolCalls = []gin.H{{"name": name, "status": "running"}}
	if err = r.publish(m); err != nil {
		return err
	}
	toolCtx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	result, err := r.s.executeChatReadTool(toolCtx, r.uid, r.pid, name, json.RawMessage(`{}`))
	if err != nil {
		m.ToolCalls = []gin.H{{"name": name, "status": "failed", "summary": "查询未完成"}}
		_ = r.publish(m)
		_ = r.save(ctx, m)
		return err
	}
	m.ToolCalls = []gin.H{chatToolEvidence(name, result)}
	if err = r.publish(m); err != nil {
		return err
	}
	if err = r.save(ctx, m); err != nil {
		return err
	}
	raw, _ := json.Marshal(result)
	if err = r.write(gin.H{"type": "conversation.item.create", "item": gin.H{"type": "function_call_output", "call_id": item.CallID, "output": string(raw)}}); err != nil {
		return err
	}
	// Wait for response.done: several tools can belong to one response.
	r.continueResponse = true
	return nil
}

func (s *Server) realtimeChat(c *gin.Context) {
	// WebSockets cannot send the normal CSRF header. Require an exact trusted
	// Origin in addition to the authenticated session, including in development.
	if c.GetHeader("Origin") != s.cfg.PublicOrigin {
		s.failure(c, 403, "ORIGIN_DENIED")
		return
	}
	pid, err := projectID(c)
	sid, e := strconv.ParseInt(c.Param("sessionId"), 10, 32)
	if err != nil || e != nil || sid <= 0 {
		s.agentSessionFailure(c, errors.New("AGENT_SESSION_NOT_FOUND"))
		return
	}
	uid := currentUser(c).ID
	if err = s.checkChatAccess(c.Request.Context(), uid, pid, int32(sid)); err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	if _, loaded := s.realtimeUsers.LoadOrStore(uid, true); loaded {
		s.failure(c, 409, "AI_REALTIME_ALREADY_CONNECTED")
		return
	}
	defer s.realtimeUsers.Delete(uid)
	upgrader := websocket.Upgrader{CheckOrigin: func(r *http.Request) bool { return r.Header.Get("Origin") == s.cfg.PublicOrigin }}
	client, err := upgrader.Upgrade(c.Writer, c.Request, nil)
	if err != nil {
		return
	}
	defer client.Close()
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Minute)
	defer cancel()
	emit := func(kind string, data any) error {
		_ = client.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return client.WriteJSON(gin.H{"type": kind, "data": data})
	}
	upstream, config, err := s.connectRealtime(ctx)
	if err != nil {
		_ = emit("error", gin.H{"code": realtimeError(err)})
		return
	}
	defer upstream.Close()
	write := func(v any) error {
		_ = upstream.SetWriteDeadline(time.Now().Add(5 * time.Second))
		return upstream.WriteJSON(v)
	}
	r := &realtimeConversation{config: config, s: s, uid: uid, pid: pid, sid: int32(sid), requestID: c.GetHeader("X-Request-ID"), items: map[string]*realtimeMessage{}, calls: map[string]bool{}, emit: emit, write: write}
	// On disconnect retain partial transcripts, without keeping raw audio.
	defer func() {
		saveCtx, done := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer done()
		for _, id := range r.order {
			_ = r.save(saveCtx, r.items[id])
		}
	}()
	type frame struct {
		kind int
		data []byte
		err  error
	}
	read := func(conn *websocket.Conn, limit int64) <-chan frame {
		ch := make(chan frame, 16)
		conn.SetReadLimit(limit)
		go func() {
			defer close(ch)
			for {
				kind, b, e := conn.ReadMessage()
				select {
				case ch <- frame{kind, b, e}:
				case <-ctx.Done():
					return
				}
				if e != nil {
					return
				}
			}
		}()
		return ch
	}
	clientFrames, upstreamFrames := read(client, 64<<10), read(upstream, 2<<20)
	ticker := time.NewTicker(20 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			_ = emit("error", gin.H{"code": "AI_REALTIME_TIMEOUT"})
			return
		case <-ticker.C:
			if err = s.checkChatAccess(ctx, uid, pid, int32(sid)); err != nil {
				_ = emit("error", gin.H{"code": realtimeError(err)})
				return
			}
			if err = emit("heartbeat", gin.H{}); err != nil {
				return
			}
		case f, ok := <-upstreamFrames:
			if !ok || f.err != nil {
				_ = emit("error", gin.H{"code": "AI_REALTIME_DISCONNECTED"})
				return
			}
			var event stepRealtimeEvent
			if json.Unmarshal(f.data, &event) != nil {
				_ = emit("error", gin.H{"code": "AI_REALTIME_UPSTREAM_FAILED"})
				return
			}
			if err = r.handle(ctx, event); err != nil {
				_ = emit("error", gin.H{"code": realtimeError(err)})
				return
			}
		case f, ok := <-clientFrames:
			if !ok || f.err != nil {
				return
			}
			if f.kind == websocket.BinaryMessage {
				if len(f.data) == 0 || len(f.data) > 24000 || len(f.data)%2 != 0 {
					_ = emit("error", gin.H{"code": "AI_REALTIME_AUDIO_INVALID"})
					return
				}
				err = write(gin.H{"type": "input_audio_buffer.append", "audio": base64.StdEncoding.EncodeToString(f.data)})
			} else {
				var command struct {
					Type       string `json:"type"`
					ItemID     string `json:"itemId"`
					AudioEndMS int    `json:"audioEndMs"`
				}
				if json.Unmarshal(f.data, &command) != nil {
					return
				}
				switch command.Type {
				case "stop":
					for _, id := range r.order {
						if err = r.save(ctx, r.items[id]); err != nil {
							_ = emit("error", gin.H{"code": realtimeError(err)})
							return
						}
					}
					_ = emit("done", gin.H{})
					return
				case "truncate":
					if m := r.items[command.ItemID]; m != nil && m.Role == "assistant" && command.AudioEndMS >= 0 && command.AudioEndMS <= 900000 {
						err = write(gin.H{"type": "conversation.item.truncate", "item_id": command.ItemID, "content_index": 0, "audio_end_ms": command.AudioEndMS})
					}
				default:
					_ = emit("error", gin.H{"code": "AI_REALTIME_COMMAND_INVALID"})
					return
				}
			}
			if err != nil {
				return
			}
		}
	}
}

func realtimeError(err error) string {
	if strings.HasPrefix(err.Error(), "AI_REALTIME_") {
		return err.Error()
	}
	return chatStreamError(err)
}
