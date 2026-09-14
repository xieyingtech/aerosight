package report

import (
	"context"
	"database/sql"
)

// InspectionSummary is a live, read-only view. It neither generates a report
// version nor completes a report step while the workflow is awaiting review.
type InspectionSummary struct {
	RunID              int               `json:"runId"`
	RunStatus          string            `json:"runStatus"`
	StateVersion       int               `json:"stateVersion"`
	PendingReviewCount int               `json:"pendingReviewCount"`
	Final              bool              `json:"final"`
	Inspection         *inspectionReport `json:"inspection,omitempty"`
	DataGaps           []string          `json:"dataGaps"`
}

func ReadInspectionSummary(ctx context.Context, tx *sql.Tx, projectID, runID int) (InspectionSummary, error) {
	result := InspectionSummary{RunID: runID, DataGaps: []string{}}
	err := tx.QueryRowContext(ctx, `select status,state_version,(select count(*) from inspection_assessments a where a.project_id=r.project_id and a.task_run_id=r.id and a.status='needs_review') from task_runs r where r.project_id=$1 and r.id=$2`, projectID, runID).Scan(&result.RunStatus, &result.StateVersion, &result.PendingReviewCount)
	if err != nil {
		return result, err
	}
	content := reportContent{Assets: []assetFact{}, DataGaps: []string{}}
	if err = appendInspectionContent(ctx, tx, projectID, runID, &content); err != nil {
		return result, err
	}
	result.Inspection = content.Inspection
	result.DataGaps = content.DataGaps
	return result, nil
}
