package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"bytes"
	"context"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/png"
	"net/http"
	"net/url"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type agentWorkflowTool struct {
	Name, Description, Permission string
	Schema                        map[string]any
}

func agentObject(properties map[string]any, required ...string) map[string]any {
	keys := make([]any, len(required))
	for i, key := range required {
		keys[i] = key
	}
	return map[string]any{"type": "object", "properties": properties, "required": keys, "additionalProperties": false}
}
func agentID() map[string]any {
	return map[string]any{"type": "integer", "minimum": 1, "maximum": 2147483647}
}
func agentText() map[string]any {
	return map[string]any{"type": "string", "minLength": 1, "maxLength": 200}
}
func agentEnum(values ...string) map[string]any {
	v := make([]any, len(values))
	for i, x := range values {
		v[i] = x
	}
	return map[string]any{"type": "string", "enum": v}
}
func agentJSON() map[string]any { return map[string]any{"type": "object"} }

// A fixed operation catalogue, never an arbitrary HTTP/SQL tool. The same
// business handlers used by the platform enforce their resource and safety gates.
func agentWorkflowTools() []agentWorkflowTool {
	return []agentWorkflowTool{
		{"sync_flight_resources", "申请同步司空飞行记录与媒体目录。只更新平台资源，不会启动飞行；需连接器管理权限。", "device:configure", agentObject(map[string]any{"connectorId": agentID()}, "connectorId")},
		{"create_inspection_task", "申请创建停用的巡检任务及草稿。definition 使用 AeroSight Task JSON；创建不会发布、启用或运行。", "mission:operate", agentObject(map[string]any{"definition": agentJSON()}, "definition")},
		{"save_task_draft", "申请保存任务草稿配置。先查询任务工作台获得 versionId 和 expectedRevision。", "mission:operate", agentObject(map[string]any{"taskId": agentID(), "versionId": agentID(), "expectedRevision": agentID(), "definition": agentJSON()}, "taskId", "versionId", "expectedRevision", "definition")},
		{"publish_task", "申请发布指定任务草稿版本，发布不等于运行；必须使用查询到的版本和修订。", "mission:operate", agentObject(map[string]any{"taskId": agentID(), "versionId": agentID(), "expectedRevision": agentID()}, "taskId", "versionId", "expectedRevision")},
		{"set_task_state", "申请启用或停用任务。启用定时任务可能允许后续自动触发，停用不代表取消正在运行的任务。", "mission:operate", agentObject(map[string]any{"taskId": agentID(), "status": agentEnum("active", "disabled"), "expectedVersionId": agentID()}, "taskId", "status", "expectedVersionId")},
		{"run_task", "申请手动运行已发布任务，可能触发真实设备或飞行。先查询并固定 published version ID 和 inputs；等待用户点击授权。", "mission:operate", agentObject(map[string]any{"taskId": agentID(), "expectedVersionId": agentID(), "inputs": agentJSON()}, "taskId", "expectedVersionId", "inputs")},
		{"control_task_run", "申请任务审批、暂停、恢复、取消或紧急停止。必须先查询 stateVersion；业务取消不代表飞机已停止。", "mission:operate", agentObject(map[string]any{"taskRunId": agentID(), "action": agentEnum("approve", "pause", "resume", "cancel", "emergency_stop"), "expectedVersion": map[string]any{"type": "integer", "minimum": 0}, "reason": map[string]any{"type": "string", "minLength": 1, "maxLength": 1000}}, "taskRunId", "action", "expectedVersion", "reason")},
		{"submit_flight", "申请向司空提交一次 immediate 飞行任务。必须已有真实预检通过、匹配的飞行审批和航线；本工具不会创建或绕过安全审批。API 接受不等于飞机起飞。", "mission:operate", agentObject(map[string]any{"connectorId": agentID(), "taskRunId": agentID(), "approvalRequestId": agentText(), "waylineResourceId": agentID(), "request": agentJSON()}, "connectorId", "taskRunId", "approvalRequestId", "waylineResourceId", "request")},
		{"control_flight", "申请司空 flight-task-status 或 flight-task-resume 操作，保留既有飞行预检、审批和功能开关。", "mission:operate", agentObject(map[string]any{"connectorId": agentID(), "taskRunId": agentID(), "approvalRequestId": agentText(), "targetResourceId": agentID(), "action": agentEnum("flight-task-status", "flight-task-resume"), "request": agentJSON()}, "connectorId", "taskRunId", "approvalRequestId", "targetResourceId", "action", "request")},
		{"run_algorithm", "申请将已有照片提交到已发布算法配置。算法服务读取真实原图；返回 runId 仅代表入队，必须再查询运行结果。", "algorithm:manage", agentObject(map[string]any{"configurationSnapshotId": agentID(), "assetId": agentID(), "parameters": agentJSON()}, "configurationSnapshotId", "assetId", "parameters")},
		{"review_inspection", "申请提交整批巡检人工复核决定，用户需核对每项决定后点击授权，不能由模型自行授权。", "issue:handle", agentObject(map[string]any{"assessmentId": agentText(), "expectedRevision": agentID(), "decisions": map[string]any{"type": "array", "maxItems": 1000, "items": agentJSON()}}, "assessmentId", "expectedRevision", "decisions")},
		{"generate_report", "申请为已终结的任务运行生成报告草稿。失败或不完整运行的报告必须如实保留数据缺口；不对外发布。", "mission:operate", agentObject(map[string]any{"taskRunId": agentID()}, "taskRunId")},
	}
}

func agentWorkflowSpec(name string) (agentWorkflowTool, bool) {
	for _, spec := range agentWorkflowTools() {
		if spec.Name == name {
			return spec, true
		}
	}
	return agentWorkflowTool{}, false
}
func agentIsWriteTool(name string) bool {
	_, ok := agentWorkflowSpec(name)
	return ok || name == "mutate_issue" || name == "create_task_draft"
}
func agentWorkflowPermission(spec agentWorkflowTool, args map[string]any) string {
	if spec.Name == "control_task_run" && args["action"] == "approve" {
		return "mission:approve"
	}
	return spec.Permission
}
func parseAgentWorkflowInput(name string, raw json.RawMessage) (agentWorkflowTool, map[string]any, error) {
	spec, ok := agentWorkflowSpec(name)
	var args map[string]any
	bad := errors.New("AGENT_TOOL_INPUT_INVALID")
	if !ok || len(raw) > 128*1024 || json.Unmarshal(raw, &args) != nil || args == nil || chatScopeArgument(args) != nil {
		return spec, nil, bad
	}
	schema, _ := json.Marshal(spec.Schema)
	args, err := parseFHInput(raw, schema)
	if err != nil {
		return spec, nil, bad
	}
	if name == "submit_flight" && fhObject(args["request"])["taskType"] != "immediate" {
		return spec, nil, bad
	}
	return spec, args, nil
}

// Verify top-level resource IDs before presenting an approval. Handlers repeat
// scope checks inside their audited transactions at execution time.
func validateAgentResources(ctx context.Context, q *sqlcgen.Queries, db interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}, pid int32, args map[string]any) error {
	tables := map[string]string{"taskId": "tasks", "versionId": "task_versions", "taskRunId": "task_runs", "connectorId": "device_adapters", "assetId": "assets", "configurationSnapshotId": "algorithm_definition_versions", "assessmentId": "inspection_assessments", "waylineResourceId": "connector_remote_resources", "targetResourceId": "connector_remote_resources"}
	for key, table := range tables {
		if id, ok := args[key]; ok {
			var exists bool
			query := "select exists(select 1 from " + table + " where project_id=$1 and id=$2)"
			if err := db.QueryRowContext(ctx, query, pid, id).Scan(&exists); err != nil {
				return err
			}
			if !exists {
				return errors.New("AGENT_TOOL_RESOURCE_NOT_FOUND")
			}
		}
	}
	if args["definition"] != nil {
		if _, _, _, err := parseTaskAuthorInput(args); err != nil {
			return err
		}
	}
	return nil
}

func (s *Server) proposeWorkflowWrite(ctx context.Context, uid, pid, sid int32, name string, raw json.RawMessage, requestID string) (gin.H, error) {
	spec, args, err := parseAgentWorkflowInput(name, raw)
	if err != nil {
		return nil, err
	}
	permission := agentWorkflowPermission(spec, args)
	access, err := s.projectAccess(ctx, s.queries, uid, pid, permission)
	if err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	id := uuid.NewString()
	summary := workflowApprovalSummary(name, args)
	stored := gin.H{"input": args, "summary": summary}
	encoded, _ := json.Marshal(stored)
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: requestID, Action: "agent_write.propose", ResourceType: "agent_write_approval", ResourceID: id, Input: stored, PolicyResult: gin.H{"permission": permission, "requiresUserClick": true}}
	_, err = database.AuditedWrite(ctx, s.db, audit, func(ctx context.Context, w *database.WriteTx) error {
		if e := s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false)(ctx, w); e != nil {
			return e
		}
		return s.authorizeWrite(uid, pid, access.TeamID, permission, false)(ctx, w)
	}, func(w *database.WriteTx) (bool, error) {
		if _, e := w.Queries.LockChatSession(ctx, agentSessionLock(uid, pid, sid)); e != nil {
			return false, e
		}
		if e := validateAgentResources(ctx, w.Queries, w.Tx, pid, args); e != nil {
			return false, e
		}
		_, e := w.Tx.ExecContext(ctx, `insert into agent_write_approvals(id,project_id,session_id,user_id,tool_name,arguments) values($1,$2,$3,$4,$5,$6)`, id, pid, sid, uid, name, encoded)
		return true, e
	})
	if err != nil {
		return nil, err
	}
	return gin.H{"approvalId": id, "status": "confirmation_required", "summary": summary}, nil
}

func workflowApprovalSummary(name string, args map[string]any) string {
	switch name {
	case "sync_flight_resources":
		return fmt.Sprintf("同步连接器 #%v 的飞行记录和照片目录；不会启动飞行。", args["connectorId"])
	case "create_inspection_task":
		return fmt.Sprintf("创建任务“%v”的停用草稿；不会发布、启用或运行。", fhObject(args["definition"])["name"])
	case "save_task_draft":
		return fmt.Sprintf("保存任务 #%v 的草稿版本 #%v，基于修订 %v。", args["taskId"], args["versionId"], args["expectedRevision"])
	case "publish_task":
		return fmt.Sprintf("发布任务 #%v 的版本 #%v，基于修订 %v；不会立即启动任务。", args["taskId"], args["versionId"], args["expectedRevision"])
	case "set_task_state":
		if args["status"] == "active" {
			return fmt.Sprintf("启用任务 #%v 的发布版本 #%v；定时任务启用后可按计划运行。", args["taskId"], args["expectedVersionId"])
		}
		return fmt.Sprintf("停用任务 #%v；不会取消正在运行的任务。", args["taskId"])
	case "run_task":
		return fmt.Sprintf("启动任务 #%v 的发布版本 #%v；可能触发真实设备操作，请核对运行参数。", args["taskId"], args["expectedVersionId"])
	case "control_task_run":
		labels := map[string]string{"approve": "审批", "pause": "暂停", "resume": "恢复", "cancel": "取消", "emergency_stop": "紧急停止"}
		return fmt.Sprintf("%s任务运行 #%v，基于状态版本 %v。原因：%v。业务状态变更不代表飞机已停止。", labels[fhString(args["action"])], args["taskRunId"], args["expectedVersion"], args["reason"])
	case "submit_flight":
		return fmt.Sprintf("向连接器 #%v 提交任务运行 #%v 的真实飞行，航线 #%v；仍须通过平台飞行预检和审批。", args["connectorId"], args["taskRunId"], args["waylineResourceId"])
	case "control_flight":
		return fmt.Sprintf("对连接器 #%v 的飞行资源 #%v 执行 %v；请核对控制参数和现场状态。", args["connectorId"], args["targetResourceId"], args["action"])
	case "run_algorithm":
		return fmt.Sprintf("将照片 #%v 提交至算法配置 #%v 识别；提交后需查询算法运行结果。", args["assetId"], args["configurationSnapshotId"])
	case "review_inspection":
		return fmt.Sprintf("提交研判 %v 的整批复核决定，基于修订 %v；请逐项核对下面的决定。", args["assessmentId"], args["expectedRevision"])
	case "generate_report":
		return fmt.Sprintf("为任务运行 #%v 生成报告草稿；不会对外发布。", args["taskRunId"])
	}
	return name
}

type agentHandlerCall struct {
	handler gin.HandlerFunc
	method  string
	params  gin.Params
	query   url.Values
	body    map[string]any
	photo   bool
}

func agentParam(key string, value any) gin.Param {
	return gin.Param{Key: key, Value: fmt.Sprint(value)}
}
func (s *Server) workflowHandler(name string, args map[string]any, key string) (agentHandlerCall, error) {
	call := agentHandlerCall{method: "POST", body: map[string]any{}, query: url.Values{}}
	for k, v := range args {
		call.body[k] = v
	}
	task := func() {
		call.params = append(call.params, agentParam("taskId", args["taskId"]))
		delete(call.body, "taskId")
	}
	run := func() {
		call.params = append(call.params, agentParam("runId", args["taskRunId"]))
		delete(call.body, "taskRunId")
	}
	switch name {
	case "sync_flight_resources":
		call.params = append(call.params, agentParam("connectorId", args["connectorId"]))
		call.body = map[string]any{}
		call.handler = s.syncFlightHub
	case "create_inspection_task":
		call.handler = s.createTask
		call.body["idempotencyKey"] = key
	case "save_task_draft":
		task()
		call.handler = s.taskDraft
		call.body["action"] = "save"
	case "publish_task":
		task()
		call.handler = s.taskDraft
		call.body["action"] = "publish"
	case "set_task_state":
		task()
		call.method = "PATCH"
		call.handler = s.updateTaskState
	case "run_task":
		task()
		call.handler = s.triggerTaskRun
		call.body["type"] = "manual"
		call.body["idempotencyKey"] = key
		call.body["occurredAt"] = time.Now().UTC().Format(time.RFC3339Nano)
	case "control_task_run":
		run()
		call.handler = s.controlMissionRun
	case "submit_flight", "control_flight":
		call.params = append(call.params, agentParam("connectorId", args["connectorId"]))
		delete(call.body, "connectorId")
		call.handler = s.fhGovernedAction("flight")
		call.body["idempotencyKey"] = key
		if name == "submit_flight" {
			call.body["action"] = "flight-task-create"
		}
	case "run_algorithm":
		call.handler = s.startAlgorithmRun
	case "review_inspection":
		call.params = append(call.params, agentParam("assessmentId", args["assessmentId"]))
		delete(call.body, "assessmentId")
		call.handler = s.inspectionAssessment
		call.body["idempotencyKey"] = key
	case "generate_report":
		run()
		call.handler = s.createReportDraft
	default:
		return call, errors.New("AGENT_TOOL_NOT_ALLOWED")
	}
	return call, nil
}

// Capture JSON without any network request. Authenticate the actor from the DB,
// then call only a catalogue-selected business handler. This is not an HTTP
// proxy; no model-provided paths, headers, cookies or credentials are accepted.
type agentJSONWriter struct {
	headers http.Header
	body    bytes.Buffer
	status  int
	limit   int
}

func (w *agentJSONWriter) Header() http.Header { return w.headers }
func (w *agentJSONWriter) WriteHeader(status int) {
	if w.status == 0 {
		w.status = status
	}
}
func (w *agentJSONWriter) Write(p []byte) (int, error) {
	if w.status == 0 {
		w.status = 200
	}
	if w.body.Len()+len(p) > w.limit {
		return 0, errors.New("AGENT_TOOL_RESPONSE_TOO_LARGE")
	}
	return w.body.Write(p)
}
func (s *Server) callAgentHandler(ctx context.Context, uid, pid int32, call agentHandlerCall, requestID string) (int, any, error) {
	user, err := s.queries.GetUser(ctx, uid)
	if err != nil {
		return 0, nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	if _, err = s.projectAccess(ctx, s.queries, uid, pid, "agent:use"); err != nil {
		return 0, nil, err
	}
	raw, _ := json.Marshal(call.body)
	req, err := http.NewRequestWithContext(ctx, call.method, "http://internal.invalid/?"+call.query.Encode(), bytes.NewReader(raw))
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Request-ID", requestID)
	limit := 256 * 1024
	if call.photo {
		limit = 64 << 20
	}
	writer := &agentJSONWriter{headers: http.Header{}, limit: limit}
	c := gin.CreateTestContextOnly(writer, s.router)
	c.Request = req
	c.Params = append(gin.Params{agentParam("id", pid)}, call.params...)
	c.Set("user", userDTO(user))
	if s.limitAuthenticatedRequest(c) {
		var result any
		_ = json.Unmarshal(writer.body.Bytes(), &result)
		return writer.status, result, nil
	}
	call.handler(c)
	if call.photo && writer.status == 200 {
		preview, err := agentPhotoPreview(writer.body.Bytes())
		return writer.status, preview, err
	}
	var result any
	if json.Unmarshal(writer.body.Bytes(), &result) != nil {
		return writer.status, nil, errors.New("AGENT_TOOL_RESPONSE_INVALID")
	}
	return writer.status, result, nil
}

func (s *Server) decideWorkflowWrite(c *gin.Context, uid, pid, sid int32, id, name string, stored map[string]any) {
	ctx := c.Request.Context()
	args := fhObject(stored["input"])
	raw, _ := json.Marshal(args)
	spec, args, err := parseAgentWorkflowInput(name, raw)
	if err != nil {
		s.failure(c, 400, "AGENT_APPROVAL_FAILED")
		return
	}
	permission := agentWorkflowPermission(spec, args)
	access, err := s.projectAccess(ctx, s.queries, uid, pid, permission)
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	call, err := s.workflowHandler(name, args, "agent-approval:"+id)
	if err != nil {
		s.failure(c, 400, "AGENT_APPROVAL_FAILED")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "agent_write.authorize", ResourceType: "agent_write_approval", ResourceID: id, Input: stored, PolicyResult: gin.H{"permission": permission, "userApproved": true}}
	_, err = database.AuditedWrite(ctx, s.db, audit, func(ctx context.Context, w *database.WriteTx) error {
		if e := s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false)(ctx, w); e != nil {
			return e
		}
		return s.authorizeWrite(uid, pid, access.TeamID, permission, false)(ctx, w)
	}, func(w *database.WriteTx) (bool, error) {
		if _, e := w.Queries.LockChatSession(ctx, agentSessionLock(uid, pid, sid)); e != nil {
			return false, e
		}
		result, e := w.Tx.ExecContext(ctx, `update agent_write_approvals set status='executing',decided_at=now() where id=$1 and project_id=$2 and session_id=$3 and user_id=$4 and status='pending' and expires_at>now()`, id, pid, sid, uid)
		if e != nil {
			return false, e
		}
		n, e := result.RowsAffected()
		if e == nil && n != 1 {
			e = errors.New("AGENT_APPROVAL_ALREADY_DECIDED")
		}
		return n == 1, e
	})
	if err != nil {
		code := err.Error()
		if code == "PROJECT_ACCESS_DENIED" {
			s.failure(c, 403, code)
		} else {
			s.failure(c, 409, "AGENT_APPROVAL_ALREADY_DECIDED")
		}
		return
	}
	// An executing approval can never be retried automatically. If the process
	// dies after dispatch, the user must reconcile the platform job/run first.
	status, result, err := s.callAgentHandler(ctx, uid, pid, call, c.GetHeader("X-Request-ID"))
	terminal := "succeeded"
	if err != nil || status < 200 || status >= 300 {
		terminal = "failed"
	}
	if err != nil {
		terminal = "executing"
		result = gin.H{"error": "AGENT_TOOL_EXECUTION_UNCERTAIN", "message": "请核对平台任务或作业状态，勿重复提交。"}
	}
	receipt := gin.H{"httpStatus": status, "output": result}
	encoded, _ := json.Marshal(receipt)
	persistCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if _, e := s.db.ExecContext(persistCtx, `update agent_write_approvals set status=$2,result=$3 where id=$1 and status='executing'`, id, terminal, encoded); e != nil {
		s.failure(c, 500, "AGENT_APPROVAL_RESULT_UNCERTAIN")
		return
	}
	followup := "sent"
	message := fmt.Sprintf("用户已手动授权工具 %s。平台 API 返回状态 %d，授权处理结果 %s，返回内容：%s。只根据返回状态说明结果；入队或接受不代表运行、起飞、识别或报告已完成。", name, status, terminal, string(encoded))
	if e := s.appendApprovalFollowup(ctx, uid, pid, sid, message, c.GetHeader("X-Request-ID")); e != nil {
		followup = "failed"
	}
	c.JSON(200, gin.H{"status": terminal, "result": receipt, "followupStatus": followup})
}

func agentInspectionQuerySchema() map[string]any {
	p := map[string]any{"resource": agentEnum("readiness", "task_templates", "validate_task", "task", "task_run", "task_run_audit", "flight_operations", "flight_plan_options", "flight_plan", "flight_job", "observation", "photo", "evidence_set", "assessment", "report", "algorithms", "algorithm_run"), "definition": agentJSON()}
	for _, k := range []string{"taskId", "taskRunId", "connectorId", "deviceId", "waylineResourceId", "assetId"} {
		p[k] = agentID()
	}
	for _, k := range []string{"observationId", "evidenceSetId", "assessmentId", "reportId", "jobId", "algorithmRunId"} {
		p[k] = agentText()
	}
	return agentObject(p, "resource")
}

func (s *Server) executeInspectionQuery(ctx context.Context, uid, pid int32, raw json.RawMessage) (gin.H, error) {
	if len(raw) > 128*1024 {
		return nil, errors.New("AGENT_TOOL_INPUT_INVALID")
	}
	schema, _ := json.Marshal(agentInspectionQuerySchema())
	args, err := parseFHInput(raw, schema)
	if err != nil || chatScopeArgument(args) != nil {
		return nil, errors.New("AGENT_TOOL_INPUT_INVALID")
	}
	call := agentHandlerCall{method: "GET", query: url.Values{}}
	require := func(key string) bool {
		if args[key] == nil {
			return false
		}
		call.params = append(call.params, agentParam(key, args[key]))
		return true
	}
	valid := true
	switch args["resource"] {
	case "task_templates":
		call.handler = func(c *gin.Context) {
			if _, e := s.projectAccess(ctx, s.queries, uid, pid, "project:view"); e != nil {
				s.failure(c, 403, "PROJECT_ACCESS_DENIED")
				return
			}
			c.JSON(200, agentInspectionTemplates())
		}
	case "validate_task":
		valid = args["definition"] != nil
		call.method = "POST"
		call.body = map[string]any{"definition": args["definition"]}
		call.handler = s.validateTaskSource
	case "readiness":
		call.handler = s.inspectionReadiness
	case "task":
		valid = require("taskId")
		call.handler = s.taskWorkbench
	case "task_run":
		valid = require("taskRunId")
		if valid {
			call.params[len(call.params)-1].Key = "runId"
		}
		call.handler = s.readInspectionSummary
	case "task_run_audit":
		valid = require("taskRunId")
		if valid {
			call.params[len(call.params)-1].Key = "runId"
		}
		call.handler = s.getMissionAuditTrace
	case "flight_operations":
		call.handler = s.fhWorkspace("FlightOps")
	case "flight_plan_options":
		call.handler = s.inspectionFlightPlanOptions
	case "flight_plan":
		for _, k := range []string{"connectorId", "deviceId", "waylineResourceId"} {
			if args[k] == nil {
				valid = false
			} else {
				call.query.Set(k, fmt.Sprint(args[k]))
			}
		}
		call.handler = s.previewInspectionFlightPlan
	case "flight_job":
		valid = require("connectorId") && require("jobId")
		call.query.Set("jobId", fmt.Sprint(args["jobId"]))
		call.handler = s.fhGovernedAction("flight")
	case "observation":
		valid = require("observationId")
		call.handler = s.getInspectionObservation
	case "photo":
		valid = require("observationId") && require("assetId")
		call.handler = s.readInspectionImage
		call.photo = true
	case "evidence_set":
		valid = require("evidenceSetId")
		call.handler = s.getInspectionEvidenceSet
	case "assessment":
		valid = require("assessmentId")
		call.handler = s.inspectionAssessment
	case "report":
		valid = require("reportId")
		call.handler = s.readGeneratedReport
	case "algorithms":
		call.handler = func(c *gin.Context) {
			s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
				rows, e := q.ListAlgorithmCatalog(ctx, a.ProjectID)
				if e != nil {
					return nil, e
				}
				return decodeSnapshotRows(rows)
			})
		}
	case "algorithm_run":
		valid = require("algorithmRunId")
		call.handler = func(c *gin.Context) {
			s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
				id, e := uuid.Parse(fmt.Sprint(args["algorithmRunId"]))
				if e != nil {
					return nil, e
				}
				raw, e := q.ReadAlgorithmRunDetail(ctx, sqlcgen.ReadAlgorithmRunDetailParams{ProjectID: pid, ID: id})
				if e != nil {
					return nil, e
				}
				rows, e := decodeSnapshotRows([]json.RawMessage{raw})
				if e != nil {
					return nil, e
				}
				run := rows[0]
				delete(run, "inputSnapshot")
				return run, nil
			})
		}
	default:
		valid = false
	}
	if !valid {
		return nil, errors.New("AGENT_TOOL_INPUT_INVALID")
	}
	status, result, err := s.callAgentHandler(ctx, uid, pid, call, "agent-inspection-query")
	if err != nil {
		return nil, err
	}
	if status < 200 || status >= 300 {
		return gin.H{"projectId": pid, "status": "failed", "httpStatus": status, "result": result}, nil
	}
	if photo, ok := result.(agentPhoto); ok {
		return gin.H{"projectId": pid, "observedAt": timestamp(time.Now()), "quality": "sealed-photo-preview", "items": []gin.H{{"id": args["assetId"], "width": photo.width, "height": photo.height, "reference": gin.H{"type": "inspection-photo", "id": fmt.Sprint(args["assetId"]), "href": fmt.Sprintf("/projects/%d/inspection/observations/%v/", pid, args["observationId"])}}}, "_image": photo}, nil
	}
	encoded, _ := json.Marshal(result)
	if len(encoded) > 64*1024 {
		return gin.H{"projectId": pid, "quality": "authoritative-project-query", "truncated": true, "summary": "结果超过工具大小上限，请查询单个资源。", "items": []any{}}, nil
	}
	items := []gin.H{}
	switch value := result.(type) {
	case map[string]any:
		items = append(items, gin.H(value))
	case []any:
		for _, raw := range value {
			if row, ok := raw.(map[string]any); ok {
				items = append(items, gin.H(row))
			}
		}
	}
	return gin.H{"projectId": pid, "observedAt": timestamp(time.Now()), "quality": "authoritative-project-query", "truncated": false, "items": items}, nil
}

func agentInspectionTemplates() []gin.H {
	rows := []gin.H{}
	for _, mode := range []string{"assets", "existing-flight", "flighthub-flight"} {
		observe := map[string]any{"mode": mode}
		detect := map[string]any{"observationId": "steps.observe.outputs.observationId", "source": "external", "algorithmDefinitionVersionId": 0}
		if mode == "assets" {
			observe["assetIds"] = []int{}
		} else {
			observe["connectorId"] = 0
			detect["source"] = "flighthub-ai"
			delete(detect, "algorithmDefinitionVersionId")
		}
		if mode == "existing-flight" {
			observe["flightUuid"] = ""
		}
		if mode == "flighthub-flight" {
			observe["deviceId"] = 0
			observe["waylineResourceId"] = 0
			observe["waylineVersion"] = map[string]any{}
			observe["schedulerOwner"] = "aerosight"
			observe["taskType"] = "immediate"
		}
		definition := gin.H{"apiVersion": "aerosight/v2", "name": "巡检任务", "trigger": gin.H{"type": "manual"}, "concurrencyLimit": 1, "steps": []gin.H{
			{"key": "observe", "uses": "inspection.observe", "with": observe},
			{"key": "detect", "uses": "inspection.detect", "dependsOn": []string{"observe"}, "with": detect},
			{"key": "assess", "uses": "copilot.run", "dependsOn": []string{"detect"}, "with": gin.H{"mode": "assessment", "temperature": 0.2, "evidenceSetId": "steps.detect.outputs.evidenceSetId"}},
			{"key": "issue", "uses": "issue.create-or-update", "dependsOn": []string{"assess"}, "with": gin.H{"assessmentId": "steps.assess.outputs.assessmentId"}},
			{"key": "report", "uses": "report.generate", "dependsOn": []string{"issue"}, "with": gin.H{"scope": "current-run"}},
		}}
		rows = append(rows, gin.H{"mode": mode, "definition": definition, "requiresConfiguration": true, "verification": "template-only", "notice": "替换占位资源后必须 validate_task；新飞行模式目前仅可保存草稿，不能把模板存在当作运行可用。"})
	}
	return rows
}

// A bounded JPEG preview is supplied as a model image input, never as a signed
// URL or base64 inside function output. Algorithm execution still uses the original.
type agentPhoto struct {
	dataURL       string
	width, height int
}

func agentPhotoPreview(body []byte) (agentPhoto, error) {
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil || config.Width <= 0 || config.Height <= 0 || int64(config.Width)*int64(config.Height) > 64_000_000 {
		return agentPhoto{}, errors.New("AGENT_TOOL_IMAGE_UNSUPPORTED")
	}
	source, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return agentPhoto{}, errors.New("AGENT_TOOL_IMAGE_UNSUPPORTED")
	}
	width, height := config.Width, config.Height
	if width > 1024 || height > 1024 {
		if width >= height {
			height = height * 1024 / width
			width = 1024
		} else {
			width = width * 1024 / height
			height = 1024
		}
	}
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	scaled := image.NewRGBA(image.Rect(0, 0, width, height))
	bounds := source.Bounds()
	for y := 0; y < height; y++ {
		for x := 0; x < width; x++ {
			scaled.Set(x, y, source.At(bounds.Min.X+x*config.Width/width, bounds.Min.Y+y*config.Height/height))
		}
	}
	var encoded bytes.Buffer
	if err = jpeg.Encode(&encoded, scaled, &jpeg.Options{Quality: 85}); err != nil {
		return agentPhoto{}, err
	}
	return agentPhoto{dataURL: "data:image/jpeg;base64," + base64.StdEncoding.EncodeToString(encoded.Bytes()), width: width, height: height}, nil
}
