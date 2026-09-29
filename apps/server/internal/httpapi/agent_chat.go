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

const chatInstructions = "巡检影像目标查询、属性筛选和追问前，先用 load_skill 加载 inspection-object-query，再按技能调用 query_objects 并交付稳定筛选结果链接。 你是 AeroSight 项目 Copilot。先使用平台查询工具核对事实，再用中文回答。巡检前使用 query_inspection 检查就绪配置、任务版本、飞行和识别结果。不得把旧告警事件说成案件。所有写工具只生成待确认请求，必须让用户在界面手动点击授权。文字同意不能代替点击。不得伪造资源 ID、预检或飞行审批。启用定时任务、运行任务、提交飞行可能产生真实设备动作，必须清楚说明。平台 API 接受或入队不等于起飞、照片回传、识别或报告完成，要查询实际状态。不能声称未验收的实机链已验证。"

func chatTools() []responses.ToolUnionParam {
	tools := []responses.ToolUnionParam{}
	tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: "load_skill", Description: openai.String("加载平台内置行业 Skill。目标查询、候选筛选和视觉复核前加载 inspection-object-query；返回规则必须用于接下来的操作。"), Strict: openai.Bool(false), Parameters: agentObject(map[string]any{"skillName": agentEnum(objectSkillName)}, "skillName")}})
	tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: "query_objects", Description: openai.String("只读查询真实检测目标。先 load_skill。省略 algorithmRunId 列出当前项目最近算法运行；指定成功检测运行后按实际模型 labels 和 minConfidence 筛选。includeImage 默认 true：经版本和 checksum 校验的整图与最多8张候选裁剪会作为图片输入送给当前 AI Provider。看图后再次调用，selectedDetectionKeys 与 selectionReason 提交保留 ID 和理由（空数组代表零匹配）；服务核验 ID 并返回稳定筛选结果链接。不执行新识别，不认定违规。"), Strict: openai.Bool(false), Parameters: objectQuerySchema()}})
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
	tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: "mutate_issue", Description: openai.String("申请修改当前项目案件。每次调用只创建一项等待用户点击确认的操作，不会立即执行。先查询案件，使用其当前 stateVersion。可添加评论、改状态或标签、分配或取消分配。"), Strict: openai.Bool(false), Parameters: map[string]any{"type": "object", "properties": map[string]any{"issueId": map[string]any{"type": "integer"}, "expectedVersion": map[string]any{"type": "integer"}, "mutation": map[string]any{"type": "object", "properties": map[string]any{"action": map[string]any{"type": "string", "enum": []string{"comment", "status", "labels", "assign", "unassign"}}, "body": map[string]any{"type": "string"}, "status": map[string]any{"type": "string"}, "labels": map[string]any{"type": "array", "items": map[string]any{"type": "string"}}, "assigneeType": map[string]any{"type": "string"}, "assigneeId": map[string]any{"type": "integer"}}, "required": []string{"action"}, "additionalProperties": false}}, "required": []string{"issueId", "expectedVersion", "mutation"}, "additionalProperties": false}}})
	tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: "create_task_draft", Description: openai.String("申请为当前项目已有 Task 创建可编辑草稿版本。需要用户点击授权；不会发布、启用或运行任务。"), Strict: openai.Bool(false), Parameters: map[string]any{"type": "object", "properties": map[string]any{"taskId": map[string]any{"type": "integer", "minimum": 1}}, "required": []string{"taskId"}, "additionalProperties": false}}})
	tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: "query_inspection", Description: openai.String("查询巡检资源：readiness、task_templates（带占位 ID 的配置模板）、validate_task（提供 definition，无副作用校验）、task（工作台及版本）、task_run、flight_operations、flight_plan_options、flight_plan（目录预览，不是起飞预检）、flight_job、observation（照片清单）、photo（必须 observationId 和 assetId，将经校验的照片预览作为图片输入交给模型）、evidence_set、assessment、report、algorithms、algorithm_run。全部限当前项目。照片清单不等于看过图片；photo 图片预览不是算法原图，不能代替完整识别。新飞行模板仍受平台部署能力门控。"), Strict: openai.Bool(false), Parameters: agentInspectionQuerySchema()}})
	for _, spec := range agentWorkflowTools() {
		tools = append(tools, responses.ToolUnionParam{OfFunction: &responses.FunctionToolParam{Name: spec.Name, Description: openai.String(spec.Description + " 必须用户点击授权后才执行。"), Strict: openai.Bool(false), Parameters: spec.Schema}})
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
	var configuredModels []aiModel
	if json.Unmarshal(provider.ModelsJson, &configuredModels) == nil {
		for _, model := range configuredModels {
			if model.ID == provider.ModelID && model.Protocol != "responses" {
				return zero, "", cleanup, errors.New("AI_PROVIDER_PROTOCOL_NOT_IMPLEMENTED")
			}
		}
	}
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

	base := provider.BaseUrl.String
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	target, err := url.Parse(base)
	if err != nil {
		return zero, "", cleanup, err
	}
	target, addresses, err := s.resolveAIURL(ctx, base)
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
	if result["status"] == "failed" {
		return gin.H{"name": name, "status": "failed", "summary": fmt.Sprintf("平台查询失败（HTTP %v），请核对资源或权限。", result["httpStatus"])}
	}
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
	if name == "load_skill" || name == "query_objects" {
		if specific, ok := result["summary"].(string); ok {
			summary = specific
		}
	}
	if result["truncated"] == true {
		summary = "结果已安全截断"
	}
	return gin.H{"name": name, "status": "succeeded", "summary": summary, "evidenceRefs": refs}
}

// An approval completes outside the original chat turn. Start a read-only
// response after the audited write commits so the user receives its outcome.
func (s *Server) appendApprovalFollowup(ctx context.Context, uid, pid, sid int32, receipt, requestID string) error {
	if err := s.checkChatAccess(ctx, uid, pid, sid); err != nil {
		return err
	}
	content := receipt
	aiCtx, cancel := context.WithTimeout(ctx, 12*time.Second)
	defer cancel()
	if history, err := s.queries.RecentChatHistory(aiCtx, sqlcgen.RecentChatHistoryParams{ID: sid, ProjectID: pid, StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}}); err == nil {
		if sdk, model, cleanup, err := s.configuredChatClient(aiCtx); err == nil {
			defer cleanup()
			input := []responses.ResponseInputItemUnionParam{}
			for _, message := range history {
				if message.Content == "" {
					continue
				}
				input = append(input, responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{Role: responses.EasyInputMessageRole(message.Role), Content: responses.EasyInputMessageContentUnionParam{OfString: openai.String(message.Content)}}})
			}
			instructions := "你是 AeroSight 项目 Copilot。用户刚刚点击授权，平台返回了处理结果。只根据以下结果，用中文简短说明成功、失败或待核对状态。入队或接受不等于实际执行完成，不能把失败说成成功。不要再次要求授权，不要提出或执行其他写操作。平台结果：" + receipt
			params := responses.ResponseNewParams{Model: model, Instructions: openai.String(instructions), Input: responses.ResponseNewParamsInputUnion{OfInputItemList: input}, Store: openai.Bool(false)}
			if response, err := sdk.Responses.New(aiCtx, params); err == nil && response.Status == "completed" && strings.TrimSpace(response.OutputText()) != "" {
				content = response.OutputText()
			}
		}
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err := s.appendAgentMessage(ctx, uid, pid, sid, "assistant", content, nil, requestID)
	return err
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
			var result gin.H
			if call.Name == "mutate_issue" {
				result, e = s.proposeIssueWrite(ctx, uid, pid, sid, json.RawMessage(call.Arguments), requestID)
			} else if call.Name == "create_task_draft" {
				result, e = s.proposeTaskDraft(ctx, uid, pid, sid, json.RawMessage(call.Arguments), requestID)
			} else if _, known := agentWorkflowSpec(call.Name); known {
				result, e = s.proposeWorkflowWrite(ctx, uid, pid, sid, call.Name, json.RawMessage(call.Arguments), requestID)
			} else {
				result, e = s.executeChatReadTool(ctx, uid, pid, call.Name, json.RawMessage(call.Arguments))
			}
			if e != nil {
				if emit != nil {
					failed := gin.H{"name": call.Name, "status": "failed", "summary": "工具调用未完成"}
					emit("tool", gin.H{"id": call.CallID, "name": call.Name, "status": "failed", "summary": "工具调用未完成"})
					_, _ = s.appendAgentMessage(ctx, uid, pid, sid, "assistant", text, append(stepCalls, failed), requestID)
				}
				return nil, e
			}
			photo, hasPhoto := result["_image"].(agentPhoto)
			delete(result, "_image")
			photos, _ := result["_images"].([]agentPhoto)
			delete(result, "_images")
			raw, e := json.Marshal(result)
			if e != nil {
				return nil, e
			}
			input = append(input, responses.ResponseInputItemUnionParam{OfFunctionCallOutput: &responses.ResponseInputItemFunctionCallOutputParam{CallID: openai.String(call.CallID), Output: responses.ResponseInputItemFunctionCallOutputOutputUnionParam{OfString: openai.String(string(raw))}}})
			if hasPhoto {
				input = append(input, responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{Role: "user", Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: responses.ResponseInputMessageContentListParam{{OfInputImage: &responses.ResponseInputImageParam{ImageURL: openai.String(photo.dataURL), Detail: "auto"}}}}}})
			}
			for _, preview := range photos {
				input = append(input, responses.ResponseInputItemUnionParam{OfMessage: &responses.EasyInputMessageParam{Role: "user", Content: responses.EasyInputMessageContentUnionParam{OfInputItemContentList: responses.ResponseInputMessageContentListParam{
					{OfInputText: &responses.ResponseInputTextParam{Text: preview.caption}},
					{OfInputImage: &responses.ResponseInputImageParam{ImageURL: openai.String(preview.dataURL), Detail: "auto"}},
				}}}})
			}
			var evidence gin.H
			if agentIsWriteTool(call.Name) {
				evidence = gin.H{"name": call.Name, "status": "confirmation_required", "summary": result["summary"], "approvalId": result["approvalId"]}
			} else {
				evidence = chatToolEvidence(call.Name, result)
			}
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
		case err.Error() == "ISSUE_VERSION_CONFLICT":
			s.failure(c, 409, err.Error())
		default:
			s.agentSessionFailure(c, err)
		}
		return
	}
	c.JSON(201, result)
}
