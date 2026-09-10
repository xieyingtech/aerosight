-- name: ReadIssueFeedback :many
SELECT to_jsonb(r) FROM (
select feedback.id,feedback.detection_id as "detectionId",feedback.action,
      feedback.corrected_label as "correctedLabel",feedback.disposition,feedback.reason,
      feedback.algorithm_definition_version_id as "algorithmDefinitionVersionId",feedback.task_version_id as "taskVersionId",
      feedback.task_run_step_id as "taskRunStepId",feedback.evidence_snapshot_json as "evidenceSnapshot",
      actor.name as "actorName",feedback.created_at as "createdAt"
      from issue_feedback feedback join users actor on actor.id=feedback.actor_user_id
      where feedback.project_id=$1 and feedback.issue_id=$2 order by feedback.created_at,feedback.id
) r;

-- name: ReadIssueQualityStats :many
SELECT to_jsonb(r) FROM (
select algorithm_definition_version_id as "algorithmDefinitionVersionId",
      task_version_id as "taskVersionId",count(*)::int as total,
      count(*) filter(where action='confirm')::int as confirmed,
      count(*) filter(where action='false_positive')::int as "falsePositives",
      count(*) filter(where action='category_correction')::int as corrections,
      count(*) filter(where action='disposition')::int as dispositions,
      round((count(*) filter(where action='false_positive')::numeric/nullif(count(*) filter(where action in('confirm','false_positive')),0)),4) as "falsePositiveRate"
      from issue_feedback where project_id=$1 group by algorithm_definition_version_id,task_version_id
      order by algorithm_definition_version_id,task_version_id
) r;
