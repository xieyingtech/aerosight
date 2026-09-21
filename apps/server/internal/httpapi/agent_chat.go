package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
	"github.com/openai/openai-go/v3/responses"
)

const chatInstructions = "你是 AeroSight 项目 Copilot。先使用平台查询工具核对事实，再用中文回答。明确数据时间和质量；不得把旧告警事件说成案件，不得声称已执行设备或算法操作。需要操作时，只建议用户创建或启动 Task。"

func chatTools() []responses.ToolUnionParam {
	tools := []responses.ToolUnionParam{}
	for _, entry := range []struct{ name, description string }{{"query_devices", "查询当前项目设备、类型、驱动、状态和数据新鲜度"}, {"query_tasks", "查询当前项目 Tasks 及其最近运行状态"}, {"query_issues", "查询当前项目案件、状态、优先级和证据质量"}, {"query_assets", "查询当前项目可用数据资产及版本"}, {"query_tracks", "查询当前项目设备轨迹摘要"}, {"query_map_context", "查询当前项目地图态势摘要"}} {
		properties := map[string]any{}
		switch entry.name {
		case "query_devices":
			properties["deviceIds"] = gin.H{"type": "array", "maxItems": 100, "items": gin.H{"type": "integer", "minimum": 1, "maximum": 2147483647}}
		case "query_tasks", "query_issues":
			properties["limit"] = gin.H{"type": "integer", "minimum": 1, "maximum": 100, "default": 20}
		}
		tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: entry.name, Description: openai.String(entry.description), Strict: openai.Bool(false), Parameters: map[string]any{"type": "object", "properties": properties, "additionalProperties": false}}})
	}
	return tools
}

func (s *Server) chatTimeout(c *gin.Context) {
	timeout := s.cfg.AIRequestTimeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), timeout)
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	c.Next()
}

func (s *Server) checkChatAccess(ctx context.Context, uid, pid, sid int32) error {
	if _, err := s.projectAccess(ctx, s.queries, uid, pid, "agent:use"); err != nil {
		return errors.New("PROJECT_ACCESS_DENIED")
	}
	_, err := s.queries.ReadOpenChatSession(ctx, sqlcgen.ReadOpenChatSessionParams{ID: sid, ProjectID: pid, StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
	if errors.Is(err, sql.ErrNoRows) {
		return errors.New("AGENT_SESSION_NOT_FOUND")
	}
	return err
}

type aiChatClient struct{ client *http.Client }

func (c aiChatClient) Do(req *http.Request) (*http.Response, error) {
	res, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	res.Body = http.MaxBytesReader(nil, res.Body, 4<<20)
	return res, nil
}

func (s *Server) configuredChatClient(ctx context.Context) (openai.Client, string, func(), error) {
	var zero openai.Client
	cleanup := func() {}
	providers, err := s.queries.ReadDefaultChatProvider(ctx)
	if err != nil {
		return zero, "", cleanup, err
	}
	if len(providers) == 0 {
		return zero, "", cleanup, errors.New("AI_PROVIDER_UNAVAILABLE")
	}
	if len(providers) != 1 || providers[0].ProviderType != "openai" {
		return zero, "", cleanup, errors.New("AI_PROVIDER_CONFIGURATION_INVALID")
	}
	provider := providers[0]
	var envelope credentials.Envelope
	if err = json.Unmarshal(provider.CredentialEnvelopeJson, &envelope); err != nil {
		return zero, "", cleanup, err
	}
	var credential struct {
		APIKey string `json:"apiKey"`
	}
	if err = credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("ai-provider", provider.ID, nil), &credential); err != nil {
		return zero, "", cleanup, err
	}
	if credential.APIKey == "" {
		return zero, "", cleanup, errors.New("AI_PROVIDER_CREDENTIAL_UNAVAILABLE")
	}
	base := provider.BaseUrl.String
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	target, err := url.Parse(base)
	if err != nil {
		return zero, "", cleanup, err
	}
	target, addresses, err := s.resolveOutboundURL(ctx, base, []string{target.Hostname()})
	if err != nil {
		return zero, "", cleanup, err
	}
	factory := s.aiHTTPClientFactory
	if factory == nil {
		factory = pinnedAIHTTPClient
	}
	client := factory(target, addresses)
	sdk := openai.NewClient(option.WithAPIKey(credential.APIKey), option.WithBaseURL(strings.TrimRight(base, "/")+"/"), option.WithHTTPClient(aiChatClient{client}), option.WithMaxRetries(0))
	return sdk, provider.ModelID, client.CloseIdleConnections, nil
}

func chatToolEvidence(name string, result gin.H) gin.H {
	items := result["items"].([]gin.H)
	refs := []gin.H{}
	for index, row := range items {
		if index >= 20 {
			break
		}
		ref, ok := row["reference"].(gin.H)
		if !ok {
			continue
		}
		version := row["version"]
		if version == nil {
			version = row["observedAt"]
		}
		if version == nil {
			version = "current"
		}
		versionText := fmt.Sprint(version)
		if number, ok := version.(float64); ok {
			versionText = strconv.FormatFloat(number, 'f', -1, 64)
		}
		refs = append(refs, gin.H{"type": ref["type"], "id": ref["id"], "href": ref["href"], "version": versionText})
	}
	summary := fmt.Sprintf("返回 %d 条项目内记录", len(items))
	if result["truncated"] == true {
		summary = "结果已安全截断"
	}
	return gin.H{"name": name, "status": "succeeded", "summary": summary, "evidenceRefs": refs}
}

func (s *Server) runChatTurn(ctx context.Context, uid, pid, sid int32, content, requestID string, listeners ...func(string, gin.H)) (gin.H, error) {
	var emit func(string, gin.H)
	if len(listeners) > 0 {
		emit = listeners[0]
	}
	if err := s.checkChatAccess(ctx, uid, pid, sid); err != nil {
		return nil, err
	}
	if _, err := s.appendAgentMessage(ctx, uid, pid, sid, "user", content, nil, requestID); err != nil {
		return nil, err
	}
	if emit != nil {
		emit("status", gin.H{"message": "正在分析问题，准备查询项目数据…"})
	}
	history, err := s.queries.RecentChatHistory(ctx, sqlcgen.RecentChatHistoryParams{ID: sid, ProjectID: pid, StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
	if err != nil {
		return nil, err
	}
	sdk, model, cleanup, err := s.configuredChatClient(ctx)
	if err != nil {
		return nil, err
	}
	defer cleanup()
	input := []responses.ResponseInputItemUnionParam{}
	for _, message := range history {
		if message.Content == "" {
			continue
		}
		input = append(input, responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{Role: responses.EasyInputMessageRole(message.Role), Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(message.Content)}}})
	}
	calls := []gin.H{}
	text := ""
	var saved gin.H
	for step := 0; step < 8; step++ {
		if err = s.checkChatAccess(ctx, uid, pid, sid); err != nil {
			return nil, err
		}
		params := responses.ResponseNewParams{Model: model, Instructions: openai.String(chatInstructions), Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input}, Tools: chatTools(), Store: openai.Bool(false), Include: []responses.ResponseIncludable{"reasoning.encrypted_content"}}
		var response *responses.Response
		var e error
		if emit == nil {
			response, e = sdk.Responses.New(ctx, params)
		} else {
			emit("step", gin.H{"step": step})
			response, e = streamChatResponse(ctx, sdk, params, func(delta string) { emit("text", gin.H{"delta": delta}) })
		}
		if e != nil {
			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			return nil, errors.New("AI_UPSTREAM_FAILED")
		}
		if response.Status == "failed" || response.Status == "cancelled" || response.Status == "queued" || response.Status == "in_progress" {
			return nil, errors.New("AI_UPSTREAM_FAILED")
		}
		if response.Status != "completed" && response.Status != "incomplete" {
			return nil, errors.New("AI_UPSTREAM_RESPONSE_INVALID")
		}
		text = response.OutputText()
		toolCount := 0
		stepCalls := []gin.H{}
		// Preserve message phases and encrypted reasoning when replaying stateless
		// Responses context. Hosted tools are not part of this application's toolset.
		for _, item := range response.Output {
			switch item.Type {
			case "message":
				p := item.AsMessage().ToParam()
				input = append(input, responses.ResponseInputItemUnionParam{OfOutputMessage: &p})
			case "reasoning":
				p := item.AsReasoning().ToParam()
				input = append(input, responses.ResponseInputItemUnionParam{OfReasoning: &p})
			case "function_call":
				p := item.AsFunctionCall().ToParam()
				input = append(input, responses.ResponseInputItemUnionParam{OfFunctionCall: &p})
			default:
				return nil, errors.New("AI_UPSTREAM_RESPONSE_INVALID")
			}
		}
		for _, item := range response.Output {
			if item.Type != "function_call" {
				continue
			}
			toolCount++
			call := item.AsFunctionCall()
			if call.CallID == "" {
				return nil, errors.New("AI_UPSTREAM_RESPONSE_INVALID")
			}
			if err = s.checkChatAccess(ctx, uid, pid, sid); err != nil {
				return nil, err
			}
			if emit != nil {
				emit("tool", gin.H{"name": call.Name, "status": "running", "id": call.CallID})
			}
			result, e := s.executeChatReadTool(ctx, uid, pid, call.Name, json.RawMessage(call.Arguments))
			if e != nil {
				if emit != nil {
					failed := gin.H{"name": call.Name, "status": "failed", "summary": "查询未完成"}
					emit("tool", gin.H{"id": call.CallID, "name": call.Name, "status": "failed", "summary": "查询未完成"})
					_, _ = s.appendAgentMessage(ctx, uid, pid, sid, "assistant", text, append(stepCalls, failed), requestID)
				}
				return nil, e
			}
			raw, e := json.Marshal(result)
			if e != nil {
				return nil, e
			}
			input = append(input, responses.ResponseInputItemUnionParam{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{CallID: openai.String(call.CallID), Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: openai.String(string(raw))}}})
			evidence := chatToolEvidence(call.Name, result)
			calls = append(calls, evidence)
			stepCalls = append(stepCalls, evidence)
			if emit != nil {
				evidence["id"] = call.CallID
				emit("tool", evidence)
			}
		}
		if emit != nil && (text != "" || len(stepCalls) > 0) {
			saved, err = s.appendAgentMessage(ctx, uid, pid, sid, "assistant", text, stepCalls, requestID)
			if err != nil {
				return nil, err
			}
		}
		if emit != nil && toolCount > 0 {
			emit("status", gin.H{"message": "正在结合查询结果继续分析…"})
		}
		if toolCount == 0 {
			break
		}
		if emit != nil && step == 7 {
			return nil, errors.New("AGENT_TOOL_STEP_LIMIT")
		}
	}
	if emit != nil {
		if saved == nil || text == "" {
			return nil, errors.New("AI_UPSTREAM_RESPONSE_INVALID")
		}
		saved["content"], saved["modelId"] = text, "openai:"+model
		return saved, nil
	}
	stored := text
	if stored == "" {
		stored = "未生成可用回复。"
	}
	result, err := s.appendAgentMessage(ctx, uid, pid, sid, "assistant", stored, calls, requestID)
	if err != nil {
		return nil, err
	}
	result["content"], result["modelId"] = text, "openai:"+model
	return result, nil
}

func (s *Server) chatTurn(c *gin.Context) {
	pid, err := projectID(c)
	sid, e := strconv.ParseInt(c.Param("sessionId"), 10, 32)
	if err != nil || e != nil || sid <= 0 {
		s.agentSessionFailure(c, errors.New("AGENT_SESSION_NOT_FOUND"))
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		s.agentSessionFailure(c, errors.New("AGENT_MESSAGE_INVALID"))
		return
	}
	content, ok := body["content"].(string)
	if !ok || strings.TrimSpace(content) == "" {
		s.agentSessionFailure(c, errors.New("AGENT_MESSAGE_INVALID"))
		return
	}
	if strings.Contains(c.GetHeader("Accept"), "application/x-ndjson") {
		s.streamChatTurn(c, pid, int32(sid), content)
		return
	}
	result, err := s.runChatTurn(c.Request.Context(), currentUser(c).ID, pid, int32(sid), content, c.GetHeader("X-Request-ID"))
	if err != nil {
		if c.Request.Context().Err() != nil {
			err = c.Request.Context().Err()
		}
		switch {
		case errors.Is(err, context.DeadlineExceeded):
			s.failure(c, 504, "AI_REQUEST_TIMEOUT")
		case errors.Is(err, context.Canceled):
			s.failure(c, 400, "AI_REQUEST_CANCELLED")
		case strings.HasPrefix(err.Error(), "AI_PROVIDER_") || err.Error() == "AI_UPSTREAM_FAILED" || err.Error() == "AI_UPSTREAM_RESPONSE_INVALID" || strings.HasPrefix(err.Error(), "AGENT_TOOL_"):
			s.failure(c, 400, err.Error())
		default:
			s.agentSessionFailure(c, err)
		}
		return
	}
	c.JSON(201, result)
}
