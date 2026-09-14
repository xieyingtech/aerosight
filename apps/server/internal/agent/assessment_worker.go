package agent

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"aerosight/server/internal/inspection"
	"aerosight/server/internal/taskdefinition"
	"aerosight/server/internal/tasktrigger"
)

// Run -> job -> assessment is the lock order. started_at fences a two-minute
// lease, longer than the bounded model request. Model I/O never holds Run locks.
// An expired read-only attempt may be reclaimed; only its current lease can
// commit a result. Completed jobs are never requested again.
func (processor JobProcessor) ProcessAssessmentNext(ctx context.Context) (bool, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	tx, err := processor.Database.BeginTx(ctx, nil)
	if err != nil {
		return false, err
	}
	defer tx.Rollback()
	var job copilotJob
	var runState string
	var user int32
	var version int64
	err = tx.QueryRowContext(ctx, `select job.id::text,run.project_id,run.team_id,run.id,rs.id,run.status,coalesce(run.created_by_user_id,0),run.task_version_id
 from task_runs run join task_versions version on version.id=run.task_version_id and version.project_id=run.project_id and version.dsl_version='aerosight/v2'
 join task_run_steps rs on rs.task_run_id=run.id and rs.project_id=run.project_id
 join agent_tool_jobs job on job.project_id=run.project_id and job.team_id=run.team_id and (job.args_json->>'taskRunStepId')::bigint=rs.id
 where (job.status='queued' or (job.status='running' and job.started_at < $1)) and job.tool_name='inspection_assessment' and run.status<>'paused'
 order by job.created_at,job.id for update of run skip locked limit 1`, processor.now().Add(-2*time.Minute)).Scan(&job.ID, &job.ProjectID, &job.TeamID, &job.TaskRunID, &job.TaskRunStepID, &runState, &user, &version)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	job.TriggerType = "task_step"
	job.ToolName = "inspection_assessment"
	var jobState, assessmentID, evidenceHash, stepState, policy, promptVersion, jobPromptVersion string
	var sessionID int
	var temperature float64
	var evidenceRaw, observationRaw, outputSchema []byte
	err = tx.QueryRowContext(ctx, `select job.status,job.session_id,a.id::text,a.evidence_hash,e.evidence_json,o.manifest_json,rs.status,step.output_schema_json,coalesce(step.failure_policy_json->>'onFailure','abort'),coalesce((job.args_json->>'temperature')::double precision,0.2),a.prompt_version,coalesce(job.args_json->>'promptVersion','')
 from agent_tool_jobs job join agent_sessions session on session.id=job.session_id and session.project_id=job.project_id and session.task_run_id=$3
 join inspection_assessments a on a.id=(job.args_json->>'assessmentId')::uuid and a.project_id=job.project_id and a.task_run_id=$3 and a.task_run_step_id=$4
 join inspection_evidence_sets e on e.id=a.evidence_set_id and e.project_id=a.project_id and e.task_run_id=a.task_run_id
 join inspection_observations o on o.id=e.observation_id and o.project_id=e.project_id and o.task_run_id=e.task_run_id
 join task_run_steps rs on rs.id=a.task_run_step_id and rs.project_id=a.project_id
 join task_steps step on step.id=rs.task_step_id and step.project_id=rs.project_id
 where job.id=$1 and job.project_id=$2 for update of job,a`, job.ID, job.ProjectID, job.TaskRunID, job.TaskRunStepID).Scan(&jobState, &sessionID, &assessmentID, &evidenceHash, &evidenceRaw, &observationRaw, &stepState, &outputSchema, &policy, &temperature, &promptVersion, &jobPromptVersion)
	if err != nil {
		return false, err
	}
	if jobState == "running" {
		// The selection above only includes expired leases. Revalidate all
		// authorization before reclaiming this read-only model job.
		if _, err = tx.ExecContext(ctx, "update agent_tool_jobs set status='queued' where id=$1", job.ID); err != nil {
			return false, err
		}
		jobState = "queued"
	}
	if jobState != "queued" {
		return false, nil
	}
	result := inspectionModelResult{Assessment: inspection.Assessment{ID: assessmentID, Run: inspection.RunRef{Scope: inspection.Scope{ProjectID: job.ProjectID, TeamID: job.TeamID}, RunID: int64(job.TaskRunID), StepID: job.TaskRunStepID}, Revision: 1}}
	if runState != "running" && runState != "dispatching" || stepState != "running" {
		if _, err = tx.ExecContext(ctx, `update agent_tool_jobs set status='failed',failure_code='INSPECTION_RUN_INACTIVE',finished_at=now() where id=$1`, job.ID); err != nil {
			return false, err
		}
		if _, err = tx.ExecContext(ctx, `update inspection_assessments set status='canceled',failure_code='INSPECTION_RUN_INACTIVE' where id=$1 and status='pending'`, assessmentID); err != nil {
			return false, err
		}
		return true, tx.Commit()
	}
	var executeErr error
	if promptVersion != jobPromptVersion {
		executeErr = errors.New("INSPECTION_ASSESSMENT_PROMPT_VERSION_MISMATCH")
	} else {
		_, executeErr = inspectionInstructionsForVersion(promptVersion)
	}
	if executeErr == nil && user == 0 {
		executeErr = errors.New("TASK_STEP_DELEGATE_REQUIRED")
	} else if executeErr == nil {
		executeErr = tasktrigger.AuthorizeDelegate(ctx, tx, job.ProjectID, version, user)
	}
	if executeErr == nil {
		if err = RevalidateQueuedJob(ctx, tx, job.ID, processor.now()); err != nil {
			return false, err
		}
		if err = tx.QueryRowContext(ctx, "select status from agent_tool_jobs where id=$1", job.ID).Scan(&jobState); err != nil {
			return false, err
		}
		if jobState != "running" {
			executeErr = errors.New("AUTHORIZATION_REVALIDATION_FAILED")
		}
	}
	var evidence inspection.EvidenceSet
	var observation inspection.Observation
	if executeErr == nil {
		digest := sha256.Sum256(evidenceRaw)
		if hex.EncodeToString(digest[:]) != evidenceHash || json.Unmarshal(evidenceRaw, &evidence) != nil || json.Unmarshal(observationRaw, &observation) != nil {
			executeErr = errors.New("INSPECTION_EVIDENCE_CHANGED")
		}
	}
	var linked []inspection.LinkedIssue
	allowedIssues := map[int64]bool{}
	if executeErr == nil {
		linked, executeErr = inspection.LinkedIssues(ctx, tx, evidence, observation)
		for _, issue := range linked {
			if issue.CanUpdate {
				allowedIssues[issue.IssueID] = true
			}
		}
	}
	if executeErr == nil && (!(temperature >= 0 && temperature <= 2)) {
		executeErr = errors.New("INSPECTION_ASSESSMENT_TEMPERATURE_INVALID")
	}
	if executeErr == nil {
		contextRaw, marshalErr := json.Marshal(linked)
		if marshalErr != nil {
			return false, marshalErr
		}
		if _, err = tx.ExecContext(ctx, `update agent_tool_jobs set args_json=jsonb_set(args_json,'{linkedIssues}',$2::jsonb) where id=$1`, job.ID, contextRaw); err != nil {
			return false, err
		}
		var leaseStarted time.Time
		if err = tx.QueryRowContext(ctx, "select started_at from agent_tool_jobs where id=$1", job.ID).Scan(&leaseStarted); err != nil {
			return false, err
		}
		if err = tx.Commit(); err != nil {
			return false, err
		}
		result.Assessment.EvidenceSetID = evidence.ID
		modelContext, modelCancel := context.WithTimeout(ctx, 45*time.Second)
		result, executeErr = processor.assessEvidenceWithPromptVersion(modelContext, result.Assessment, evidence, observation, allowedIssues, temperature, promptVersion, linked...)
		modelCancel()
		tx, err = processor.Database.BeginTx(ctx, nil)
		if err != nil {
			return false, err
		}
		defer tx.Rollback()
		if err = tx.QueryRowContext(ctx, "select status from task_runs where project_id=$1 and id=$2 for update", job.ProjectID, job.TaskRunID).Scan(&runState); err != nil {
			return false, err
		}
		var currentLease time.Time
		var expires time.Time
		if err = tx.QueryRowContext(ctx, "select status,started_at,context_expires_at from agent_tool_jobs where id=$1 for update", job.ID).Scan(&jobState, &currentLease, &expires); err != nil {
			return false, err
		}
		if jobState != "running" || !currentLease.Equal(leaseStarted) {
			return true, nil // A newer lease owns this result; discard the stale response.
		}
		if err = tx.QueryRowContext(ctx, "select status from task_run_steps where project_id=$1 and id=$2", job.ProjectID, job.TaskRunStepID).Scan(&stepState); err != nil {
			return false, err
		}
		if (runState != "running" && runState != "dispatching") || stepState != "running" {
			if _, err = tx.ExecContext(ctx, "update inspection_assessments set status='canceled',original_output=$2,failure_code='INSPECTION_RUN_INACTIVE' where id=$1 and status='pending'", assessmentID, result.RawOutput); err != nil {
				return false, err
			}
			if _, err = tx.ExecContext(ctx, "update agent_tool_jobs set status='failed',failure_code='INSPECTION_RUN_INACTIVE',finished_at=now() where id=$1", job.ID); err != nil {
				return false, err
			}
			if _, err = tx.ExecContext(ctx, "update task_run_steps set status='failed',result_json=result_json||'{\"errorCode\":\"INSPECTION_RUN_INACTIVE\"}'::jsonb,finished_at=now() where project_id=$1 and id=$2 and status in('running','paused')", job.ProjectID, job.TaskRunStepID); err != nil {
				return false, err
			}
			return true, tx.Commit()
		}
		if !expires.After(processor.now()) {
			executeErr = errors.New("AUTHORIZATION_REVALIDATION_FAILED")
		} else if authErr := tasktrigger.AuthorizeDelegate(ctx, tx, job.ProjectID, version, user); authErr != nil {
			executeErr = authErr
		}
	}
	output := map[string]any{"assessmentId": assessmentID, "sessionId": sessionID, "jobId": job.ID}
	if executeErr == nil {
		var schema map[string]any
		if json.Unmarshal(outputSchema, &schema) != nil {
			executeErr = errors.New("TASK_STEP_OUTPUT_SCHEMA_INVALID")
		} else {
			compiled, compileErr := taskdefinition.CompileSchema(schema)
			if compileErr != nil {
				executeErr = compileErr
			} else {
				raw, _ := json.Marshal(output)
				var value any
				_ = json.Unmarshal(raw, &value)
				if compiled.Validate(value) != nil {
					executeErr = errors.New("TASK_STEP_OUTPUT_INVALID")
				}
			}
		}
	}
	state, code := "succeeded", ""
	if executeErr != nil {
		state = "failed"
		code = assessmentFailureCode(executeErr)
	} else if result.Assessment.NeedsReview() {
		state = "needs_review"
	}
	if _, err = tx.ExecContext(ctx, `update inspection_assessments set status=$2,provider_id=nullif($3,'')::bigint,model_version=nullif($4,''),original_output=$5,failure_code=nullif($6,''),revision=case when $2 in('succeeded','needs_review') then 1 else 0 end where id=$1`, assessmentID, state, result.ProviderID, result.ModelID, result.RawOutput, code); err != nil {
		return false, err
	}
	if executeErr == nil {
		raw, _ := json.Marshal(result.Assessment.Decisions)
		if _, err = tx.ExecContext(ctx, `insert into inspection_assessment_revisions(assessment_id,project_id,revision,source,decisions_json,idempotency_key) values($1,$2,1,'model',$3,$4)`, assessmentID, job.ProjectID, raw, "model:"+job.ID); err != nil {
			return false, err
		}
	}
	jobResult, _ := json.Marshal(output)
	terminalJob := "succeeded"
	if executeErr != nil {
		terminalJob = "failed"
	}
	if _, err = tx.ExecContext(ctx, `update agent_tool_jobs set status=$2,failure_code=nullif($3,''),result_json=$4,finished_at=now() where id=$1`, job.ID, terminalJob, code, jobResult); err != nil {
		return false, err
	}
	stepOutcome, runOutcome, reason := "succeeded", "running", "inspection_assessment_completed"
	if state == "needs_review" {
		stepOutcome, runOutcome, reason = "paused", "paused", "inspection_assessment_needs_review"
	}
	if state == "failed" {
		stepOutcome, runOutcome, reason = "failed", "failed", "inspection_assessment_failed"
		if policy == "pause" {
			stepOutcome, runOutcome = "paused", "paused"
		}
		jobResult, _ = json.Marshal(map[string]any{"errorCode": code, "assessmentId": assessmentID})
	}
	if _, err = tx.ExecContext(ctx, `update task_run_steps set status=$3,output_snapshot_json=$4,result_json=result_json||$4,finished_at=now() where project_id=$1 and id=$2`, job.ProjectID, job.TaskRunStepID, stepOutcome, jobResult); err != nil {
		return false, err
	}
	if runOutcome != "running" {
		if _, err = tx.ExecContext(ctx, `update task_runs set status=$3,state_reason=$4,state_version=state_version+1,finished_at=case when $3='failed' then now() else null end where project_id=$1 and id=$2`, job.ProjectID, job.TaskRunID, runOutcome, reason); err != nil {
			return false, err
		}
	} else if err = enqueueTaskContinuation(ctx, tx, job, "succeeded"); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

func assessmentFailureCode(err error) string {
	code := err.Error()
	if len(code) <= 96 && (strings.HasPrefix(code, "INSPECTION_") || strings.HasPrefix(code, "TASK_") || code == "AUTHORIZATION_REVALIDATION_FAILED") && !strings.ContainsAny(code, " :\n") {
		return code
	}
	return failureCodeFor(err)
}
