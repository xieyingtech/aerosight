package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (s *Server) taskWorkbench(c *gin.Context) {
	tid, e := strconv.ParseInt(c.Param("taskId"), 10, 32)
	if e != nil || tid <= 0 {
		s.failure(c, 404, "TASK_NOT_FOUND")
		return
	}
	s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
		ctx := c.Request.Context()
		raw, e := q.TaskWorkbenchTask(ctx, sqlcgen.TaskWorkbenchTaskParams{P1: a.ProjectID, P2: int32(tid)})
		task, e := fhFirstRow(raw, e, "TASK_NOT_FOUND")
		if e != nil {
			return nil, e
		}
		raw, e = q.TaskWorkbenchVersions(ctx, sqlcgen.TaskWorkbenchVersionsParams{P1: a.ProjectID, P2: int32(tid)})
		versions, e := decodeFHRows(raw, e)
		if e != nil {
			return nil, e
		}
		var selected gin.H
		for _, v := range versions {
			if v["status"] == "draft" {
				selected = v
				break
			}
		}
		if selected == nil {
			for _, v := range versions {
				if v["id"] == task["currentPublishedVersionId"] {
					selected = v
					break
				}
			}
		}
		if selected == nil && len(versions) > 0 {
			selected = versions[0]
		}
		steps := []gin.H{}
		if selected != nil {
			raw, e = q.TaskWorkbenchSteps(ctx, sqlcgen.TaskWorkbenchStepsParams{P1: a.ProjectID, P2: fhOptionalNumber(selected, "id")})
			steps, e = decodeFHRows(raw, e)
			if e != nil {
				return nil, e
			}
		}
		definition, parseErr := parseTaskDefinition(selected["definition"])
		if parseErr != nil {
			fallbackSteps := []any{}
			for _, step := range steps {
				requires := []any{}
				if step["capabilityCode"] != nil && step["capabilityCode"] != "" {
					requires = append(requires, step["capabilityCode"])
				}
				out := gin.H{"requires": requires}
				for _, key := range []string{"key", "name", "uses", "with", "inputSchema", "outputSchema", "dependsOn", "timeoutSeconds", "retry"} {
					out[key] = step[key]
				}
				if step["condition"] != nil {
					out["condition"] = step["condition"]
				}
				out["onFailure"] = step["onFailure"]
				if out["onFailure"] == nil {
					out["onFailure"] = "abort"
				}
				fallbackSteps = append(fallbackSteps, out)
			}
			inputSchema, trigger, limit := selected["inputSchema"], selected["trigger"], selected["concurrencyLimit"]
			if inputSchema == nil {
				inputSchema = gin.H{"type": "object", "properties": gin.H{}, "required": []string{}, "additionalProperties": false}
			}
			if trigger == nil {
				trigger = gin.H{"type": "manual"}
			}
			if limit == nil {
				limit = 1
			}
			definition = map[string]any{"name": task["name"], "description": fhString(task["description"]), "inputSchema": inputSchema, "trigger": trigger, "concurrencyLimit": limit, "steps": fallbackSteps}
		}
		return gin.H{"task": task, "versions": versions, "selectedVersion": selected, "steps": steps, "definition": definition, "canEdit": effectivePermissions(a.Role, a.Permissions)["mission:operate"]}, nil
	})
}
