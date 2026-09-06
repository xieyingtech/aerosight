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

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/sqlc-dev/pqtype"
)

func decodeIssueMutation(input map[string]any) (issue.Mutation, int32, string, error) {
	bad := errors.New("ISSUE_MUTATION_INPUT_INVALID")
	version, ok := input["expectedVersion"].(float64)
	key, keyOK := input["clientKey"].(string)
	raw, rawOK := input["mutation"].(map[string]any)
	if len(input) != 3 || !ok || version < 0 || version > math.MaxInt32 || math.Trunc(version) != version || !keyOK || len(key) != 36 || uuid.Validate(key) != nil || !rawOK {
		return issue.Mutation{}, 0, "", bad
	}
	action, _ := raw["action"].(string)
	m := issue.Mutation{Action: action}
	valid := false
	switch action {
	case "comment":
		m.Body, valid = raw["body"].(string)
		valid = valid && len(raw) == 2
	case "status":
		m.Status, valid = raw["status"].(string)
		valid = valid && len(raw) == 2 && (m.Status == "open" || m.Status == "closed")
	case "labels":
		labels, present := raw["labels"].([]any)
		valid = present && len(raw) == 2
		m.Labels = []string{}
		for _, v := range labels {
			label, yes := v.(string)
			if !yes {
				valid = false
			}
			m.Labels = append(m.Labels, label)
		}
	case "assign", "unassign":
		m.AssigneeType, _ = raw["assigneeType"].(string)
		id, yes := raw["assigneeId"].(float64)
		valid = len(raw) == 3 && yes && id > 0 && id <= math.MaxInt32 && math.Trunc(id) == id && (m.AssigneeType == "user" || m.AssigneeType == "agent")
		if valid {
			m.AssigneeID = int32(id)
		}
	}
	if !valid {
		return issue.Mutation{}, 0, "", bad
	}
	return m, int32(version), key, nil
}

func (s *Server) issueMutationFailure(c *gin.Context, err error) {
	code, status := "ISSUE_MUTATION_FAILED", 400
	switch err.Error() {
	case "PROJECT_ACCESS_DENIED":
		code, status = err.Error(), 403
	case "ISSUE_NOT_FOUND":
		code, status = err.Error(), 404
	case "ISSUE_VERSION_CONFLICT":
		code, status = err.Error(), 409
	case "ISSUE_MUTATION_INPUT_INVALID", "ISSUE_COMMENT_INVALID", "ISSUE_LABELS_INVALID", "ISSUE_ASSIGNEE_INVALID", "ISSUE_ASSIGNEE_SCOPE_INVALID", "ISSUE_STATUS_INVALID", "ISSUE_MUTATION_INVALID":
		code = err.Error()
	}
	s.failure(c, status, code)
}

func (s *Server) mutateIssue(c *gin.Context) {
	bad := func() { s.failure(c, 400, "ISSUE_MUTATION_INPUT_INVALID") }
	pid, err := projectID(c)
	if err != nil {
		bad()
		return
	}
	id, err := strconv.ParseInt(c.Param("issueId"), 10, 32)
	if err != nil || id <= 0 {
		bad()
		return
	}
	iid := int32(id)
	var input map[string]any
	if strictJSON(c, &input) != nil {
		bad()
		return
	}
	m, expected, key, err := decodeIssueMutation(input)
	if err != nil {
		bad()
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	permission := issue.MutationPermission(m.Action)
	access, err := s.projectAccess(ctx, s.queries, uid, pid, permission)
	if err != nil {
		s.issueMutationFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), IdempotencyKey: key, Action: "issue." + m.Action, ResourceType: "issue", ResourceID: strconv.FormatInt(id, 10), Input: input, PolicyResult: map[string]any{"permission": permission, "optimisticConcurrency": true}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, permission, false), func(w *database.WriteTx) (gin.H, error) {
		version, e := w.Queries.LockIssueMutation(ctx, sqlcgen.LockIssueMutationParams{ProjectID: pid, ID: iid})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("ISSUE_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		replay, e := w.Queries.IssueMutationReplayed(ctx, sqlcgen.IssueMutationReplayedParams{ProjectID: pid, IssueID: iid, ClientKey: sql.NullString{String: key, Valid: true}})
		if e != nil {
			return nil, e
		}
		if replay {
			return gin.H{"issueId": iid, "stateVersion": version, "replayed": true}, nil
		}
		// Re-read the rows locked by authorizeWrite; auxiliary agent permission must
		// be current too, rather than coming from the pre-transaction access check.
		membership, e := w.Queries.LockProjectMembership(ctx, sqlcgen.LockProjectMembershipParams{ProjectID: pid, UserID: uid})
		if e != nil {
			return nil, e
		}
		grants, e := w.Queries.LockProjectPermissions(ctx, sqlcgen.LockProjectPermissionsParams{ProjectID: pid, TeamID: access.TeamID, UserID: uid})
		if e != nil {
			return nil, e
		}
		permissions := effectivePermissions(membership.Role, grants)
		plan, e := issue.PlanMutation(m, permissions, version, expected)
		if e != nil {
			return nil, e
		}
		var copilot int32
		if plan.Assignment != nil {
			var changed bool
			copilot, changed, e = applyIssueAssignment(ctx, w.Queries, pid, access.TeamID, iid, uid, m, permissions)
			if e != nil {
				return nil, e
			}
			if !changed {
				return gin.H{"issueId": iid, "stateVersion": version, "replayed": true, "noOp": true}, nil
			}
		}
		update := sqlcgen.UpdateIssueMutationParams{ProjectID: pid, IssueID: iid, ExpectedVersion: version, NextVersion: plan.NextVersion}
		if plan.Status != nil {
			update.Status = sql.NullString{String: *plan.Status, Valid: true}
		}
		if plan.Labels != nil {
			raw, e := json.Marshal(plan.Labels)
			if e != nil {
				return nil, e
			}
			update.Labels = pqtype.NullRawMessage{RawMessage: raw, Valid: true}
		}
		version, e = w.Queries.UpdateIssueMutation(ctx, update)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("ISSUE_VERSION_CONFLICT")
		}
		if e != nil {
			return nil, e
		}
		metadata, e := json.Marshal(plan.Metadata)
		if e != nil {
			return nil, e
		}
		activity := sqlcgen.InsertIssueActivityParams{ProjectID: pid, IssueID: iid, EventType: plan.EventType, Metadata: metadata, ActorUserID: sql.NullInt32{Int32: uid, Valid: true}, ClientKey: sql.NullString{String: key, Valid: true}}
		if plan.Body != nil {
			activity.Body = sql.NullString{String: *plan.Body, Valid: true}
		}
		activityID, e := w.Queries.InsertIssueActivity(ctx, activity)
		if e != nil {
			return nil, e
		}
		trigger := "issue_assignment"
		if m.Action == "comment" && plan.Body != nil && issue.ShouldQueueCopilotMention(*plan.Body, permissions) {
			copilot, e = w.Queries.EnsureIssueCopilot(ctx, pid)
			if e != nil {
				return nil, e
			}
			trigger = "issue_mention"
		}
		var jobID any
		if copilot != 0 && (m.Action == "assign" || trigger == "issue_mention") {
			jobID, e = queueIssueCopilot(ctx, w.Queries, pid, access.TeamID, iid, uid, copilot, activityID, trigger)
			if e != nil {
				return nil, e
			}
		}
		_, e = w.Publish(ctx, database.ProjectEvent{ProjectID: pid, TeamID: access.TeamID, EventID: uuid.NewString(), EventType: "issue.updated", Payload: gin.H{"issueId": iid, "stateVersion": version, "action": m.Action}, NoEnqueue: true})
		if e != nil {
			return nil, e
		}
		return gin.H{"issueId": iid, "stateVersion": version, "replayed": false, "copilotJobId": jobID}, nil
	})
	if err != nil {
		s.issueMutationFailure(c, err)
		return
	}
	c.JSON(200, result)
}

func applyIssueAssignment(ctx context.Context, q *sqlcgen.Queries, pid, team, iid, uid int32, m issue.Mutation, permissions map[string]bool) (int32, bool, error) {
	var copilot int32
	var user, agent sql.NullInt32
	var err error
	if m.AssigneeType == "user" {
		user = sql.NullInt32{Int32: m.AssigneeID, Valid: true}
		_, err = q.LockIssueAssigneeUser(ctx, sqlcgen.LockIssueAssigneeUserParams{TeamID: team, UserID: m.AssigneeID})
	} else {
		agent = sql.NullInt32{Int32: m.AssigneeID, Valid: true}
		var row sqlcgen.LockIssueAssigneeAgentRow
		row, err = q.LockIssueAssigneeAgent(ctx, sqlcgen.LockIssueAssigneeAgentParams{ProjectID: pid, ID: m.AssigneeID})
		if err == nil && issue.IsCopilotAgent(row.Name, row.Kind) {
			copilot = row.ID
			if m.Action == "assign" && !permissions["agent:use"] {
				return 0, false, errors.New("PROJECT_ACCESS_DENIED")
			}
		}
	}
	if errors.Is(err, sql.ErrNoRows) {
		return 0, false, errors.New("ISSUE_ASSIGNEE_SCOPE_INVALID")
	}
	if err != nil {
		return 0, false, err
	}
	active, err := q.IssueAssigneeActive(ctx, sqlcgen.IssueAssigneeActiveParams{ProjectID: pid, IssueID: iid, UserID: user, AgentID: agent})
	if err != nil {
		return 0, false, err
	}
	if !issue.AssignmentChangeRequired(m.Action, active) {
		return copilot, false, nil
	}
	if m.Action == "assign" {
		err = q.AddIssueAssignee(ctx, sqlcgen.AddIssueAssigneeParams{ProjectID: pid, TeamID: team, IssueID: iid, AssigneeType: m.AssigneeType, UserID: user, AgentID: agent, ActorUserID: uid})
	} else {
		err = q.RemoveIssueAssignee(ctx, sqlcgen.RemoveIssueAssigneeParams{ProjectID: pid, IssueID: iid, UserID: user, AgentID: agent})
	}
	return copilot, true, err
}

func queueIssueCopilot(ctx context.Context, q *sqlcgen.Queries, pid, team, iid, uid, agent, activity int32, trigger string) (string, error) {
	ni := func(id int32) sql.NullInt32 { return sql.NullInt32{Int32: id, Valid: true} }
	ns := func(s string) sql.NullString { return sql.NullString{String: s, Valid: true} }
	session, err := q.CreateIssueCopilotSession(ctx, sqlcgen.CreateIssueCopilotSessionParams{ProjectID: pid, AgentID: ni(agent), IssueID: ni(iid), StartedByUserID: ni(uid), Summary: ns(fmt.Sprintf("Copilot · 案件 #%d", iid))})
	if err != nil {
		return "", err
	}
	job, err := q.QueueIssueCopilot(ctx, sqlcgen.QueueIssueCopilotParams{ProjectID: pid, TeamID: team, SessionID: session, ActorUserID: uid, IssueID: ni(iid), ActivityID: ni(activity), TriggerType: ns(trigger), IdempotencyKey: ns(fmt.Sprintf("%s:%d:copilot", trigger, activity))})
	if err != nil {
		return "", err
	}
	metadata, err := json.Marshal(gin.H{"jobId": job.String(), "sessionId": session, "triggerEventId": activity, "triggerType": trigger})
	if err != nil {
		return "", err
	}
	_, err = q.InsertIssueActivity(ctx, sqlcgen.InsertIssueActivityParams{ProjectID: pid, IssueID: iid, EventType: "copilot.requested", Metadata: metadata, ActorUserID: ni(uid)})
	return job.String(), err
}
