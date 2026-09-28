package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/issue"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

func issueToolInput(raw json.RawMessage) (int32, map[string]any, issue.Mutation, int32, string, error) {
	bad := errors.New("AGENT_TOOL_INPUT_INVALID")
	if len(raw) > 8192 {
		return 0, nil, issue.Mutation{}, 0, "", bad
	}
	var args map[string]any
	if json.Unmarshal(raw, &args) != nil || len(args) != 3 {
		return 0, nil, issue.Mutation{}, 0, "", bad
	}
	if chatScopeArgument(args) != nil {
		return 0, nil, issue.Mutation{}, 0, "", bad
	}
	id, ok := args["issueId"].(float64)
	if !ok || id <= 0 || id > math.MaxInt32 || math.Trunc(id) != id {
		return 0, nil, issue.Mutation{}, 0, "", bad
	}
	key := uuid.NewString()
	input := map[string]any{"expectedVersion": args["expectedVersion"], "clientKey": key, "mutation": args["mutation"]}
	m, version, _, err := decodeIssueMutation(input)
	if err != nil || (m.Action != "comment" && m.Action != "status" && m.Action != "labels" && m.Action != "assign" && m.Action != "unassign") {
		return 0, nil, issue.Mutation{}, 0, "", bad
	}
	return int32(id), input, m, version, key, nil
}

func (s *Server) proposeIssueWrite(ctx context.Context, uid, pid, sid int32, raw json.RawMessage, requestID string) (gin.H, error) {
	iid, input, m, version, _, err := issueToolInput(raw)
	if err != nil {
		return nil, err
	}
	permission := issue.MutationPermission(m.Action)
	access, err := s.projectAccess(ctx, s.queries, uid, pid, permission)
	if err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	id := uuid.NewString()
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: requestID, Action: "agent_write.propose", ResourceType: "issue", ResourceID: strconv.Itoa(int(iid)), Input: input, PolicyResult: map[string]any{"permission": permission, "requiresUserClick": true}}
	_, err = database.AuditedWrite(ctx, s.db, audit, func(ctx context.Context, w *database.WriteTx) error {
		if e := s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false)(ctx, w); e != nil {
			return e
		}
		return s.authorizeWrite(uid, pid, access.TeamID, permission, false)(ctx, w)
	}, func(w *database.WriteTx) (bool, error) {
		if _, e := w.Queries.LockChatSession(ctx, agentSessionLock(uid, pid, sid)); e != nil {
			return false, errors.New("AGENT_SESSION_NOT_FOUND")
		}
		current, e := w.Queries.LockIssueMutation(ctx, issueLock(pid, iid))
		if errors.Is(e, sql.ErrNoRows) {
			return false, errors.New("ISSUE_NOT_FOUND")
		}
		if e != nil {
			return false, e
		}
		if current != version {
			return false, errors.New("ISSUE_VERSION_CONFLICT")
		}
		if _, e = issue.PlanMutation(m, effectivePermissions(access.Role, access.Permissions), current, version); e != nil {
			return false, e
		}
		stored := map[string]any{"issueId": iid, "expectedVersion": input["expectedVersion"], "clientKey": input["clientKey"], "mutation": input["mutation"]}
		encoded, e := json.Marshal(stored)
		if e != nil {
			return false, e
		}
		_, e = w.Tx.ExecContext(ctx, `INSERT INTO agent_write_approvals(id,project_id,session_id,user_id,tool_name,arguments) VALUES($1,$2,$3,$4,'mutate_issue',$5)`, id, pid, sid, uid, encoded)
		return true, e
	})
	if err != nil {
		return nil, err
	}
	return gin.H{"approvalId": id, "status": "confirmation_required", "issueId": iid, "action": m.Action, "summary": fmt.Sprintf("案件 #%d 的 %s 操作等待用户确认", iid, m.Action)}, nil
}

func agentSessionLock(uid, pid, sid int32) sqlcgen.LockChatSessionParams {
	return sqlcgen.LockChatSessionParams{ID: sid, ProjectID: pid, StartedByUserID: sql.NullInt32{Int32: uid, Valid: true}}
}

func issueLock(pid, iid int32) sqlcgen.LockIssueMutationParams {
	return sqlcgen.LockIssueMutationParams{ProjectID: pid, ID: iid}
}

func taskDraftToolInput(raw json.RawMessage) (int32, error) {
	bad := errors.New("AGENT_TOOL_INPUT_INVALID")
	var args map[string]any
	if len(raw) > 256 || json.Unmarshal(raw, &args) != nil || len(args) != 1 || chatScopeArgument(args) != nil {
		return 0, bad
	}
	id, ok := args["taskId"].(float64)
	if !ok || id <= 0 || id > math.MaxInt32 || math.Trunc(id) != id {
		return 0, bad
	}
	return int32(id), nil
}

func (s *Server) proposeTaskDraft(ctx context.Context, uid, pid, sid int32, raw json.RawMessage, requestID string) (gin.H, error) {
	tid, err := taskDraftToolInput(raw)
	if err != nil {
		return nil, err
	}
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	id := uuid.NewString()
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: requestID, Action: "agent_write.propose", ResourceType: "task", ResourceID: strconv.Itoa(int(tid)), Input: gin.H{"taskId": tid}, PolicyResult: map[string]any{"permission": "mission:operate", "requiresUserClick": true}}
	_, err = database.AuditedWrite(ctx, s.db, audit, func(ctx context.Context, w *database.WriteTx) error {
		if e := s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false)(ctx, w); e != nil {
			return e
		}
		return s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false)(ctx, w)
	}, func(w *database.WriteTx) (bool, error) {
		if _, e := w.Queries.LockChatSession(ctx, agentSessionLock(uid, pid, sid)); e != nil {
			return false, errors.New("AGENT_SESSION_NOT_FOUND")
		}
		rawTask, e := w.Queries.TaskDraftTask(ctx, sqlcgen.TaskDraftTaskParams{P1: pid, P2: tid})
		if _, e = fhFirstRow(rawTask, e, "TASK_NOT_FOUND"); e != nil {
			return false, e
		}
		_, e = w.Tx.ExecContext(ctx, `INSERT INTO agent_write_approvals(id,project_id,session_id,user_id,tool_name,arguments) VALUES($1,$2,$3,$4,'create_task_draft',$5)`, id, pid, sid, uid, []byte(fmt.Sprintf(`{"taskId":%d}`, tid)))
		return true, e
	})
	if err != nil {
		return nil, err
	}
	return gin.H{"approvalId": id, "status": "confirmation_required", "taskId": tid, "summary": fmt.Sprintf("任务 #%d 的草稿创建等待用户确认", tid)}, nil
}

func (s *Server) executeTaskDraftApproval(ctx context.Context, uid, pid, sid, tid int32, approvalID, requestID string) (any, error) {
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: requestID, Action: "task_version.create_draft", ResourceType: "task", ResourceID: strconv.Itoa(int(tid)), Input: gin.H{"taskId": tid}, PolicyResult: map[string]any{"permission": "mission:operate", "userApproved": true}}
	return database.AuditedWrite(ctx, s.db, audit, func(ctx context.Context, w *database.WriteTx) error {
		if e := s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false)(ctx, w); e != nil {
			return e
		}
		return s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false)(ctx, w)
	}, func(w *database.WriteTx) (any, error) {
		var stored int32
		e := w.Tx.QueryRowContext(ctx, `SELECT (arguments->>'taskId')::integer FROM agent_write_approvals WHERE id=$1 AND project_id=$2 AND session_id=$3 AND user_id=$4 AND tool_name='create_task_draft' AND status='pending' AND expires_at>now() FOR UPDATE`, approvalID, pid, sid, uid).Scan(&stored)
		if errors.Is(e, sql.ErrNoRows) || stored != tid {
			return nil, errors.New("AGENT_APPROVAL_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		result, e := createTaskDraft(ctx, w.Queries, pid, access.TeamID, tid, uid)
		if e != nil {
			return nil, e
		}
		encoded, e := json.Marshal(result)
		if e != nil {
			return nil, e
		}
		_, e = w.Tx.ExecContext(ctx, `UPDATE agent_write_approvals SET status='succeeded',result=$2,decided_at=now() WHERE id=$1`, approvalID, encoded)
		return result, e
	})
}

func (s *Server) decideAgentWrite(c *gin.Context) {
	pid, err := projectID(c)
	sid64, e1 := strconv.ParseInt(c.Param("sessionId"), 10, 32)
	id, e2 := uuid.Parse(c.Param("approvalId"))
	if err != nil || e1 != nil || sid64 <= 0 || e2 != nil {
		s.failure(c, 404, "AGENT_APPROVAL_NOT_FOUND")
		return
	}
	sid, uid := int32(sid64), currentUser(c).ID
	ctx := c.Request.Context()
	if err = s.checkChatAccess(ctx, uid, pid, sid); err != nil {
		s.agentSessionFailure(c, err)
		return
	}
	var body struct {
		Decision string `json:"decision"`
	}
	if strictJSON(c, &body) != nil || (body.Decision != "approve" && body.Decision != "reject") {
		s.failure(c, 400, "AGENT_APPROVAL_INPUT_INVALID")
		return
	}
	var raw []byte
	var toolName, status string
	var expiresAt time.Time
	err = s.db.QueryRowContext(ctx, `SELECT tool_name,status,arguments,expires_at FROM agent_write_approvals WHERE id=$1 AND project_id=$2 AND session_id=$3 AND user_id=$4`, id, pid, sid, uid).Scan(&toolName, &status, &raw, &expiresAt)
	if errors.Is(err, sql.ErrNoRows) {
		s.failure(c, 404, "AGENT_APPROVAL_NOT_FOUND")
		return
	}
	if err != nil {
		s.failure(c, 500, "AGENT_APPROVAL_FAILED")
		return
	}
	if status != "pending" {
		s.failure(c, 409, "AGENT_APPROVAL_ALREADY_DECIDED")
		return
	}
	if !expiresAt.After(time.Now()) {
		s.failure(c, 410, "AGENT_APPROVAL_EXPIRED")
		return
	}
	if body.Decision == "reject" {
		access, e := s.projectAccess(ctx, s.queries, uid, pid, "agent:use")
		if e != nil {
			s.failure(c, 403, "PROJECT_ACCESS_DENIED")
			return
		}
		audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "agent_write.reject", ResourceType: "agent_write_approval", ResourceID: id.String(), Input: gin.H{"decision": "reject"}, PolicyResult: map[string]any{"permission": "agent:use"}}
		_, e = database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "agent:use", false), func(w *database.WriteTx) (bool, error) {
			result, e := w.Tx.ExecContext(ctx, `UPDATE agent_write_approvals SET status='rejected',decided_at=now() WHERE id=$1 AND project_id=$2 AND session_id=$3 AND user_id=$4 AND status='pending' AND expires_at>now()`, id, pid, sid, uid)
			if e != nil {
				return false, e
			}
			count, e := result.RowsAffected()
			if e != nil {
				return false, e
			}
			if count != 1 {
				return false, errors.New("AGENT_APPROVAL_ALREADY_DECIDED")
			}
			return true, nil
		})
		if e != nil {
			if e.Error() == "PROJECT_ACCESS_DENIED" {
				s.failure(c, 403, e.Error())
			} else if e.Error() == "AGENT_APPROVAL_ALREADY_DECIDED" {
				s.failure(c, 409, e.Error())
			} else {
				s.failure(c, 500, "AGENT_APPROVAL_FAILED")
			}
			return
		}
		c.JSON(200, gin.H{"status": "rejected"})
		return
	}
	var input map[string]any
	if json.Unmarshal(raw, &input) != nil {
		s.failure(c, 500, "AGENT_APPROVAL_FAILED")
		return
	}
	if _, known := agentWorkflowSpec(toolName); known {
		s.decideWorkflowWrite(c, uid, pid, sid, id.String(), toolName, input)
		return
	}
	if toolName == "create_task_draft" {
		taskNumber, ok := input["taskId"].(float64)
		if !ok || len(input) != 1 || taskNumber <= 0 || taskNumber > math.MaxInt32 || math.Trunc(taskNumber) != taskNumber {
			s.failure(c, 500, "AGENT_APPROVAL_FAILED")
			return
		}
		result, e := s.executeTaskDraftApproval(ctx, uid, pid, sid, int32(taskNumber), id.String(), c.GetHeader("X-Request-ID"))
		if e != nil {
			if strings.Contains(e.Error(), "ACCESS_DENIED") {
				s.failure(c, 403, "PROJECT_ACCESS_DENIED")
			} else {
				s.failure(c, 409, "AGENT_APPROVAL_FAILED")
			}
			return
		}
		followup := "sent"
		if e := s.appendApprovalFollowup(ctx, uid, pid, sid, fmt.Sprintf("任务 #%d 的可编辑草稿已创建，尚未发布或运行。", int32(taskNumber)), c.GetHeader("X-Request-ID")); e != nil {
			followup = "failed"
		}
		c.JSON(200, gin.H{"status": "succeeded", "result": result, "followupStatus": followup})
		return
	}
	if toolName != "mutate_issue" {
		s.failure(c, 400, "AGENT_APPROVAL_FAILED")
		return
	}
	issueNumber, ok := input["issueId"].(float64)
	if !ok || issueNumber <= 0 || issueNumber > math.MaxInt32 || math.Trunc(issueNumber) != issueNumber {
		s.failure(c, 500, "AGENT_APPROVAL_FAILED")
		return
	}
	iid := int32(issueNumber)
	delete(input, "issueId")
	m, version, key, err := decodeIssueMutation(input)
	if err != nil {
		s.failure(c, 500, "AGENT_APPROVAL_FAILED")
		return
	}
	result, err := s.executeIssueMutation(ctx, uid, pid, iid, m, version, key, input, c.GetHeader("X-Request-ID"), id.String())
	if err != nil {
		if strings.Contains(err.Error(), "ACCESS_DENIED") {
			s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		} else if err.Error() == "ISSUE_VERSION_CONFLICT" {
			s.failure(c, 409, err.Error())
		} else {
			s.failure(c, 409, "AGENT_APPROVAL_FAILED")
		}
		return
	}
	receipt := fmt.Sprintf("案件 #%d 的 %s 操作已执行。", iid, m.Action)
	if m.Action == "comment" {
		receipt = fmt.Sprintf("案件 #%d 已添加评论。", iid)
	}
	followup := "sent"
	if e := s.appendApprovalFollowup(ctx, uid, pid, sid, receipt, c.GetHeader("X-Request-ID")); e != nil {
		followup = "failed"
	}
	c.JSON(200, gin.H{"status": "succeeded", "result": result, "followupStatus": followup})
}
