package httpapi

import (
	"aerosight/server/internal/agent"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

func (s *Server) agentSessionFailure(c *gin.Context, err error) {
	code, status := "AGENT_SESSION_FAILED", 400
	switch err.Error() {
	case "PROJECT_ACCESS_DENIED":
		code, status = err.Error(), 403
	case "AGENT_SESSION_NOT_FOUND":
		code, status = err.Error(), 404
	case "AGENT_MESSAGE_INVALID":
		code = err.Error()
	}
	s.failure(c, status, code)
}

func (s *Server) agentSessionRoutes() {
	g := s.router.Group("/api/projects/:id/agent-sessions", s.requireUser, s.timeout)
	g.GET("", s.listAgentSessions)
	g.POST("", s.createAgentSession)
}

func (s *Server) listAgentSessions(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.agentSessionFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{Isolation: sql.LevelRepeatableRead, ReadOnly: true})
	if err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	defer tx.Rollback()
	q := s.queries.WithTx(tx)
	if _, err = s.projectAccess(ctx, q, uid, pid, "agent:use"); err != nil {
		s.agentSessionFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	user := sql.NullInt32{Int32: uid, Valid: true}
	sessions, err := q.ListChatSessions(ctx, sqlcgen.ListChatSessionsParams{ProjectID: pid, StartedByUserID: user})
	if err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	rows := make([]gin.H, 0, len(sessions))
	ids := make([]int32, 0, len(sessions))
	positions := map[int32]int{}
	for _, session := range sessions {
		positions[session.ID] = len(rows)
		ids = append(ids, session.ID)
		rows = append(rows, gin.H{"id": session.ID, "status": session.Status, "summary": nullable(session.Summary), "createdAt": timestamp(session.CreatedAt), "messages": []gin.H{}})
	}
	if len(ids) > 0 {
		messages, e := q.ListChatMessages(ctx, sqlcgen.ListChatMessagesParams{ProjectID: pid, StartedByUserID: user, SessionIds: ids})
		if e != nil {
			s.agentSessionFailure(c, e)
			return
		}
		for _, message := range messages {
			index := positions[message.SessionID]
			rows[index]["messages"] = append(rows[index]["messages"].([]gin.H), gin.H{"id": message.ID, "sessionId": message.SessionID, "role": message.Role, "content": message.Content, "toolCalls": message.ToolCallsJson, "createdAt": timestamp(message.CreatedAt)})
		}
	}
	if err = tx.Commit(); err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	c.JSON(200, rows)
}

func (s *Server) createAgentSession(c *gin.Context) {
	pid, err := projectID(c)
	if err != nil {
		s.agentSessionFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	uid := currentUser(c).ID
	ctx := c.Request.Context()
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "agent:use")
	if err != nil {
		s.agentSessionFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "agent_session.create", ResourceType: "agent_session", Input: gin.H{}, PolicyResult: gin.H{"permission": "agent:use"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false), func(w *database.WriteTx) (gin.H, error) {
		id, e := w.Queries.CreateChatSession(ctx, sqlcgen.CreateChatSessionParams{ProjectID: pid, StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
		return gin.H{"id": id}, e
	})
	if err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	c.JSON(201, result)
}

// Chat orchestration calls this for both roles. Each append independently
// reauthorizes and locks the user's open session; callers cannot select a role
// through a public message-storage endpoint.
func (s *Server) appendAgentMessage(ctx context.Context, uid, pid, sid int32, role, content string, toolCalls any, requestID string) (gin.H, error) {
	if role != "user" && role != "assistant" {
		return nil, errors.New("AGENT_MESSAGE_INVALID")
	}
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "agent:use")
	if err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	text, calls := agent.SanitizeChatMessage(content, toolCalls)
	encoded, err := json.Marshal(calls)
	if err != nil {
		return nil, err
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: requestID, Action: "agent_message.append", ResourceType: "agent_session", ResourceID: strconv.Itoa(int(sid)), Input: gin.H{"role": role}, PolicyResult: gin.H{"permission": "agent:use", "retention": "minimal"}}
	return database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false), func(w *database.WriteTx) (gin.H, error) {
		_, e := w.Queries.LockChatSession(ctx, sqlcgen.LockChatSessionParams{ID: sid, ProjectID: pid, StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("AGENT_SESSION_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		message, e := w.Queries.AppendChatMessage(ctx, sqlcgen.AppendChatMessageParams{SessionID: sid, Role: role, Content: text, ToolCallsJson: encoded})
		return gin.H{"id": message.ID, "createdAt": timestamp(message.CreatedAt)}, e
	})
}
