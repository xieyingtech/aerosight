package inspection

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

// CandidateSourceKeys preserves the identity of an original alert or image
// detection across business Runs and task versions. It never uses AI text.
func CandidateSourceKeys(ctx context.Context, tx *sql.Tx, e EvidenceSet, o Observation) (map[string]string, error) {
	if err := e.Validate(o); err != nil {
		return nil, err
	}
	keys := map[string]string{}
	if e.Source == "external" {
		for _, item := range e.ExternalResults {
			for n, detection := range item.Result.Detections {
				key, err := SourceKey(e.Run.Scope, 0, "", item.Asset.AssetID, item.Asset.Version, detection.DetectionKey, detection.Label)
				if err != nil {
					return nil, err
				}
				keys[fmt.Sprintf("external:%s:%d", item.AlgorithmRunID, n)] = key
			}
		}
	} else {
		for _, candidate := range e.Candidates {
			var resourceID int64
			if _, err := fmt.Sscanf(candidate.ID, fmt.Sprintf("flighthub-alert:%d:%%d", e.Run.ProjectID), &resourceID); err != nil || candidate.ID != fmt.Sprintf("flighthub-alert:%d:%d", e.Run.ProjectID, resourceID) {
				return nil, errors.New("INSPECTION_SOURCE_IDENTITY_INVALID")
			}
			var alertID string
			err := tx.QueryRowContext(ctx, `select resource.remote_id from connector_remote_resources resource join inspection_alert_sources source on source.remote_resource_id=resource.id and source.project_id=resource.project_id and source.connector_instance_id=resource.connector_instance_id where resource.project_id=$1 and resource.id=$2 and source.connector_instance_id=$3 and source.remote_flight_id=$4`, e.Run.ProjectID, resourceID, o.Flight.ConnectorID, o.Flight.FlightUUID).Scan(&alertID)
			if err != nil {
				return nil, err
			}
			key, err := SourceKey(e.Run.Scope, o.Flight.ConnectorID, alertID, 0, 0, "", "")
			if err != nil {
				return nil, err
			}
			keys[candidate.ID] = key
		}
	}
	for _, candidate := range e.Candidates {
		if keys[candidate.ID] == "" {
			return nil, errors.New("INSPECTION_SOURCE_IDENTITY_INVALID")
		}
	}
	return keys, nil
}

type LinkedIssue struct {
	CandidateID string `json:"candidateId"`
	IssueID     int64  `json:"issueId"`
	Title       string `json:"title"`
	Status      string `json:"status"`
	CanUpdate   bool   `json:"canUpdate"`
}

// Only already persisted source associations qualify for automatic update.
// A nearby or similarly named issue is never inferred to be the same object.
func LinkedIssues(ctx context.Context, tx *sql.Tx, e EvidenceSet, o Observation) ([]LinkedIssue, error) {
	keys, err := CandidateSourceKeys(ctx, tx, e, o)
	if err != nil {
		return nil, err
	}
	result := []LinkedIssue{}
	for _, candidate := range e.Candidates {
		item := LinkedIssue{CandidateID: candidate.ID}
		err = tx.QueryRowContext(ctx, `select issue.id,issue.title,issue.status from inspection_issue_sources source join issues issue on issue.id=source.issue_id and issue.project_id=source.project_id where source.project_id=$1 and source.source_key=$2`, e.Run.ProjectID, keys[candidate.ID]).Scan(&item.IssueID, &item.Title, &item.Status)
		if errors.Is(err, sql.ErrNoRows) {
			continue
		}
		if err != nil {
			return nil, err
		}
		item.CanUpdate = item.Status != "closed"
		result = append(result, item)
	}
	return result, nil
}
