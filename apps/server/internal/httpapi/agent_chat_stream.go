package httpapi

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/responses"
)

// Only public answer text is forwarded. Reasoning and raw tool arguments/results
// remain on the server; tool events contain the same minimal evidence as history.
func streamChatResponse(ctx context.Context, sdk openai.Client, params responses.ResponseNewParams, emit func(string)) (*responses.Response, error) {
	stream := sdk.Responses.NewStreaming(ctx, params)
	defer stream.Close()
	var completed *responses.Response
	var streamed strings.Builder
	for stream.Next() {
		event := stream.Current()
		switch event.Type {
		case "response.output_text.delta":
			streamed.WriteString(event.Delta)
			emit(event.Delta)
		case "response.completed":
			response := event.AsResponseCompleted().Response
			completed = &response
		case "response.failed", "response.incomplete", "error":
			return nil, errors.New("AI_UPSTREAM_FAILED")
		}
	}
	if err := stream.Err(); err != nil {
		return nil, err
	}
	if completed == nil {
		return nil, errors.New("AI_UPSTREAM_RESPONSE_INVALID")
	}
	// Some compatible services only send a completed event.
	if streamed.Len() == 0 && completed.OutputText() != "" {
		emit(completed.OutputText())
	}
	return completed, nil
}

func chatStreamError(err error) string {
	switch {
	case errors.Is(err, context.DeadlineExceeded):
		return "AI_REQUEST_TIMEOUT"
	case errors.Is(err, context.Canceled):
		return "AI_REQUEST_CANCELLED"
	case err.Error() == "PROJECT_ACCESS_DENIED", err.Error() == "AGENT_SESSION_NOT_FOUND":
		return err.Error()
	case strings.HasPrefix(err.Error(), "AI_PROVIDER_"), strings.HasPrefix(err.Error(), "AI_UPSTREAM_"), strings.HasPrefix(err.Error(), "AGENT_TOOL_"):
		return strings.SplitN(err.Error(), ":", 2)[0]
	default:
		return "AGENT_SESSION_FAILED"
	}
}

func (s *Server) streamChatTurn(c *gin.Context, pid, sid int32, content string) {
	uid := currentUser(c).ID
	if err := s.checkChatAccess(c.Request.Context(), uid, pid, sid); err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	ctx, cancel := context.WithCancel(c.Request.Context())
	defer cancel()
	c.Header("Content-Type", "application/x-ndjson; charset=utf-8")
	c.Header("Cache-Control", "no-cache, no-transform")
	c.Header("X-Accel-Buffering", "no")
	c.Status(200)
	emit := func(kind string, data gin.H) {
		if ctx.Err() != nil {
			return
		}
		if err := json.NewEncoder(c.Writer).Encode(gin.H{"type": kind, "data": data}); err != nil {
			cancel()
			return
		}
		c.Writer.Flush()
	}
	result, err := s.runChatTurn(ctx, uid, pid, sid, content, c.GetHeader("X-Request-ID"), emit)
	if err != nil {
		// A timeout still needs a terminal frame; the parent request can no longer
		// do database work, but its response writer can report the failure.
		_ = json.NewEncoder(c.Writer).Encode(gin.H{"type": "error", "data": gin.H{"code": chatStreamError(err)}})
		c.Writer.Flush()
		return
	}
	emit("done", result)
}
