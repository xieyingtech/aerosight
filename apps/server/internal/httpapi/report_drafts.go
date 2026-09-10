package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"net/url"
	"strconv"
)

type reportSources struct {
	Run      gin.H   `json:"taskRun"`
	Steps    []gin.H `json:"steps"`
	Track    gin.H   `json:"track"`
	Issues   []gin.H `json:"issues"`
	Comments []gin.H `json:"comments"`
	Events   []gin.H `json:"events"`
	Feedback []gin.H `json:"feedback"`
	Assets   []gin.H `json:"assets"`
}

func reportText(value any) string {
	if value == nil {
		return "null"
	}
	return fmt.Sprint(value)
}
func normalizeReportRow(row gin.H) error {
	if row == nil {
		return nil
	}
	raw, err := json.Marshal(row)
	if err != nil {
		return err
	}
	normalized, err := decodeSnapshotRows([]json.RawMessage{raw})
	if err != nil {
		return err
	}
	for k, v := range normalized[0] {
		row[k] = v
	}
	return nil
}
func aggregateReport(pid, id int32, s reportSources) (gin.H, []gin.H, error) {
	for _, row := range []gin.H{s.Run, s.Track} {
		if err := normalizeReportRow(row); err != nil {
			return nil, nil, err
		}
	}
	for _, rows := range [][]gin.H{s.Steps, s.Issues, s.Comments, s.Events, s.Feedback, s.Assets} {
		for _, row := range rows {
			if err := normalizeReportRow(row); err != nil {
				return nil, nil, err
			}
		}
	}
	link := func(section, key, value string) string {
		parameters := url.Values{}
		if key != "" {
			parameters.Set(key, value)
		}
		return projectPageURL(pid, "/projects/"+section+"/", parameters)
	}
	runLink := link("tasks/runs/detail", "runId", fmt.Sprint(id))
	refs := []gin.H{}
	ref := func(kind string, key, version any, href string) {
		refs = append(refs, gin.H{"type": kind, "id": reportText(key), "version": reportText(version), "href": href})
	}
	ref("task_run", id, "state:"+reportText(s.Run["stateVersion"]), runLink)
	var taskVersion, device any
	if s.Run["taskVersionId"] != nil {
		taskVersion = gin.H{"id": s.Run["taskVersionId"], "version": s.Run["taskVersion"], "definition": s.Run["definition"]}
		ref("task_version", s.Run["taskVersionId"], s.Run["taskVersion"], link("tasks", "", ""))
	}
	if s.Run["deviceId"] != nil {
		device = gin.H{"id": s.Run["deviceId"], "name": s.Run["deviceName"], "type": s.Run["deviceType"]}
		ref("device", s.Run["deviceId"], s.Run["deviceUpdatedAt"], link("devices", "selected", reportText(s.Run["deviceId"])))
	}
	if s.Track != nil {
		ref("track", s.Run["deviceId"], reportText(s.Track["pointCount"])+":"+reportText(s.Track["endedAt"]), link("detail", "selected", reportText(s.Run["deviceId"])))
	}
	for _, row := range s.Steps {
		ref("step", row["id"], row["status"], runLink)
	}
	for _, row := range s.Issues {
		ref("event", "issue:"+reportText(row["id"]), row["updatedAt"], link("issues/detail", "issueId", reportText(row["id"])))
	}
	for _, row := range s.Comments {
		ref("event", "comment:"+reportText(row["id"]), row["createdAt"], link("issues/detail", "issueId", reportText(row["issueId"]))+"#activity")
	}
	for _, row := range s.Events {
		ref("event", row["id"], "state:"+reportText(row["stateVersion"]), link("events/detail", "eventId", reportText(row["id"])))
	}
	for _, row := range s.Feedback {
		ref("feedback", row["id"], row["createdAt"], link("issues/detail", "issueId", reportText(row["issueId"])))
	}
	for _, row := range s.Assets {
		version := row["checksum"]
		if version == nil {
			version = "version:" + reportText(row["version"])
		}
		ref("asset", row["id"], version, link("assets", "selected", reportText(row["id"])))
		refs[len(refs)-1]["assetId"] = row["id"]
		refs[len(refs)-1]["checksumSha256"] = row["checksum"]
	}
	gaps := []gin.H{}
	missing := func(section string, absent bool) {
		if absent {
			gaps = append(gaps, gin.H{"section": section, "code": "SOURCE_MISSING", "message": section + " 数据不可用"})
		}
	}
	missing("taskRun", s.Run == nil)
	missing("taskVersion", taskVersion == nil)
	if device != nil {
		missing("track", s.Track == nil)
	}
	missing("steps", len(s.Steps) == 0)
	missing("evidence", len(refs) == 0)
	conditions := []gin.H{}
	missingCondition := false
	for _, step := range s.Steps {
		conditions = append(conditions, gin.H{"id": step["id"], "key": step["key"], "result": step["conditionResult"]})
		if step["condition"] != nil && step["conditionResult"] == nil && !missingCondition {
			gaps = append(gaps, gin.H{"section": "conditions", "code": "CONDITION_RESULT_MISSING", "message": "部分步骤缺少条件判断快照"})
			missingCondition = true
		}
	}
	conclusions := []gin.H{{"kind": "observed-fact", "text": "任务运行状态：" + reportText(s.Run["status"]), "evidenceRefs": []string{"task_run:" + reportText(id) + ":state:" + reportText(s.Run["stateVersion"])}}}
	for _, row := range s.Comments {
		if body, ok := row["body"].(string); ok && body != "" {
			conclusions = append(conclusions, gin.H{"kind": "human-conclusion", "text": body, "evidenceRefs": []string{"event:comment:" + reportText(row["id"]) + ":" + reportText(row["createdAt"])}})
		}
	}
	for _, row := range s.Feedback {
		value := row["reason"]
		if value == nil {
			value = row["action"]
		}
		if value == nil {
			value = "人工处置"
		}
		for _, reference := range refs {
			if reference["type"] == "feedback" && reference["id"] == reportText(row["id"]) {
				conclusions = append(conclusions, gin.H{"kind": "human-conclusion", "text": reportText(value), "evidenceRefs": []string{"feedback:" + reportText(row["id"]) + ":" + reportText(reference["version"])}})
				break
			}
		}
	}
	completeness := "complete"
	if len(gaps) > 0 {
		completeness = "incomplete"
	}
	return gin.H{"projectId": pid, "completeness": completeness, "dataGaps": gaps, "sections": gin.H{"taskRun": s.Run, "taskVersion": taskVersion, "device": device, "track": s.Track, "steps": s.Steps, "conditions": conditions, "issues": s.Issues, "comments": s.Comments, "events": s.Events, "feedback": s.Feedback}, "conclusions": conclusions, "evidence": refs}, refs, nil
}
func (s *Server) createReportDraft(c *gin.Context) {
	fail := func(err error) {
		code := "REPORT_CREATE_FAILED"
		status := 409
		if err != nil {
			switch err.Error() {
			case "PROJECT_ACCESS_DENIED":
				code = err.Error()
				status = 403
			case "TASK_RUN_NOT_FOUND", "REPORT_TASK_RUN_NOT_TERMINAL", "TASK_RUN_VERSION_CONFLICT":
				code = err.Error()
			}
		}
		s.failure(c, status, code)
	}
	pid, err := projectID(c)
	if err != nil {
		fail(errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	id, err := missionRunID(c)
	if err != nil {
		fail(errors.New("TASK_RUN_NOT_FOUND"))
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
	if err != nil {
		fail(errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	raw, err := s.queries.ReadReportSources(ctx, sqlcgen.ReadReportSourcesParams{ProjectID: pid, TaskRunID: id})
	if errors.Is(err, sql.ErrNoRows) {
		err = errors.New("TASK_RUN_NOT_FOUND")
	}
	if err != nil {
		fail(err)
		return
	}
	var sources reportSources
	if err = json.Unmarshal(raw, &sources); err != nil {
		fail(err)
		return
	}
	switch sources.Run["status"] {
	case "succeeded", "failed", "canceled":
	default:
		fail(errors.New("REPORT_TASK_RUN_NOT_TERMINAL"))
		return
	}
	content, refs, err := aggregateReport(pid, id, sources)
	if err != nil {
		fail(err)
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "generated_report.create_draft", ResourceType: "task_run", ResourceID: strconv.Itoa(int(id)), Input: gin.H{"completeness": content["completeness"]}, PolicyResult: map[string]any{"permission": "mission:operate"}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		current, e := w.Queries.LockMissionControlRun(ctx, sqlcgen.LockMissionControlRunParams{ProjectID: pid, ID: id})
		if e != nil {
			return nil, e
		}
		if current.Status != sources.Run["status"] || float64(current.StateVersion) != sources.Run["stateVersion"] {
			return nil, errors.New("TASK_RUN_VERSION_CONFLICT")
		}
		actor := sql.NullInt32{Int32: uid, Valid: true}
		report, e := w.Queries.UpsertDraftReport(ctx, sqlcgen.UpsertDraftReportParams{ProjectID: pid, TeamID: a.TeamID, SourceID: strconv.Itoa(int(id)), Title: reportText(sources.Run["taskName"]) + " 巡检报告", CreatedByUserID: actor})
		if e != nil {
			return nil, e
		}
		if e = w.Queries.RetireReportDrafts(ctx, sqlcgen.RetireReportDraftsParams{ProjectID: pid, GeneratedReportID: report}); e != nil {
			return nil, e
		}
		next, e := w.Queries.NextReportVersion(ctx, report)
		if e != nil {
			return nil, e
		}
		raw, e := json.Marshal(content)
		if e != nil {
			return nil, e
		}
		gaps, e := json.Marshal(content["dataGaps"])
		if e != nil {
			return nil, e
		}
		version, e := w.Queries.InsertReportDraft(ctx, sqlcgen.InsertReportDraftParams{ProjectID: pid, TeamID: a.TeamID, GeneratedReportID: report, Version: next, Completeness: content["completeness"].(string), ContentJson: raw, DataGapsJson: gaps, CreatedByUserID: actor})
		if e != nil {
			return nil, e
		}
		for _, ref := range refs {
			asset := sql.NullInt32{}
			if value, ok := ref["assetId"].(float64); ok {
				asset = sql.NullInt32{Int32: int32(value), Valid: true}
			}
			checksum := sql.NullString{}
			if value, ok := ref["checksumSha256"].(string); ok {
				checksum = sql.NullString{String: value, Valid: true}
			}
			if e = w.Queries.InsertReportEvidence(ctx, sqlcgen.InsertReportEvidenceParams{ProjectID: pid, ReportVersionID: version, EvidenceType: ref["type"].(string), EvidenceID: ref["id"].(string), EvidenceVersion: ref["version"].(string), AssetID: asset, ChecksumSha256: checksum, Href: ref["href"].(string)}); e != nil {
				return nil, e
			}
		}
		return gin.H{"reportId": report.String(), "versionId": version.String(), "version": next, "completeness": content["completeness"], "dataGaps": content["dataGaps"]}, nil
	})
	if err != nil {
		fail(err)
		return
	}
	c.JSON(201, result)
}
