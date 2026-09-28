package inspection

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
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
	CandidateID    string   `json:"candidateId"`
	IssueID        int64    `json:"issueId"`
	Title          string   `json:"title"`
	Status         string   `json:"status"`
	CanUpdate      bool     `json:"canUpdate"`
	Description    string   `json:"description"`
	Labels         []string `json:"labels"`
	MatchKind      string   `json:"matchKind"`
	RequiresReview bool     `json:"requiresReview"`
}

// Exact source associations can be reused. Other project issues are only
// candidates for model comparison and require human confirmation before linking.
func LinkedIssues(ctx context.Context, tx *sql.Tx, e EvidenceSet, o Observation) ([]LinkedIssue, error) {
	keys, err := CandidateSourceKeys(ctx, tx, e, o)
	if err != nil {
		return nil, err
	}
	result := []LinkedIssue{}
	searchTerms := []string{}
	for _, item := range e.ExternalResults {
		for _, detection := range item.Result.Detections {
			if label := strings.TrimSpace(detection.Label); label != "" {
				searchTerms = append(searchTerms, strings.ToLower(label))
			}
		}
	}
	var suggestions []LinkedIssue
	loadedSuggestions := false
	for _, candidate := range e.Candidates {
		item := LinkedIssue{CandidateID: candidate.ID}
		err = tx.QueryRowContext(ctx, `select issue.id,issue.title,issue.status from inspection_issue_sources source join issues issue on issue.id=source.issue_id and issue.project_id=source.project_id where source.project_id=$1 and source.source_key=$2`, e.Run.ProjectID, keys[candidate.ID]).Scan(&item.IssueID, &item.Title, &item.Status)
		if errors.Is(err, sql.ErrNoRows) {
			if !loadedSuggestions {
				// Relevant labels/text rank ahead of recency, so older matching
				// issues remain candidates. Category overlap alone is not a match.
				rows, queryErr := tx.QueryContext(ctx, `select id,title,status,left(coalesce(description,''),2000),labels_json::text from issues where project_id=$1 and status!='closed' order by exists(select 1 from unnest($2::text[]) term where strpos(lower(title||' '||coalesce(description,'')||' '||labels_json::text),term)>0) desc,updated_at desc,id desc limit 50`, e.Run.ProjectID, searchTerms)
				if queryErr != nil {
					return nil, queryErr
				}
				for rows.Next() {
					suggestion := LinkedIssue{CanUpdate: true, MatchKind: "candidate", RequiresReview: true}
					var labels string
					if queryErr = rows.Scan(&suggestion.IssueID, &suggestion.Title, &suggestion.Status, &suggestion.Description, &labels); queryErr != nil {
						rows.Close()
						return nil, queryErr
					}
					if queryErr = json.Unmarshal([]byte(labels), &suggestion.Labels); queryErr != nil {
						rows.Close()
						return nil, queryErr
					}
					suggestions = append(suggestions, suggestion)
				}
				queryErr = rows.Err()
				rows.Close()
				if queryErr != nil {
					return nil, queryErr
				}
				loadedSuggestions = true
			}
			for _, suggestion := range suggestions {
				suggestion.CandidateID = candidate.ID
				result = append(result, suggestion)
			}
			continue
		}
		if err != nil {
			return nil, err
		}
		item.CanUpdate = item.Status != "closed"
		item.MatchKind = "source"
		result = append(result, item)
	}
	return result, nil
}
