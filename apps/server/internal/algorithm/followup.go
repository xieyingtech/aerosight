package algorithm

import (
	"errors"
	"time"
)

type FollowUpKind string

const (
	FollowUpTaskDraft   FollowUpKind = "task_draft"
	FollowUpReportDraft FollowUpKind = "report_draft"
	FollowUpIssueDraft  FollowUpKind = "issue_draft"
)

type FollowUpRule struct {
	RuleID               int64
	ProjectID            int
	Kind                 FollowUpKind
	Title                string
	RequiresConfirmation bool
}

type FollowUpDraft struct {
	RuleID            int64
	ProjectID         int
	SourceRunID       string
	Kind              FollowUpKind
	Title             string
	Status            string
	RequiresReview    bool
	CreatedAt         time.Time
	ResultKind        CanonicalKind
	EvidenceReference string
}

// PlanAlgorithmFollowUp deliberately copies only provenance and configured
// presentation fields. Provider-controlled result fields can never become a
// device action, capability code, command parameters, or approval decision.
func PlanAlgorithmFollowUp(rule FollowUpRule, runID string, result CanonicalResult, now time.Time) (FollowUpDraft, error) {
	if rule.RuleID <= 0 || rule.ProjectID <= 0 || runID == "" || rule.Title == "" {
		return FollowUpDraft{}, errors.New("algorithm follow-up rule and run provenance are required")
	}
	switch rule.Kind {
	case FollowUpTaskDraft, FollowUpReportDraft, FollowUpIssueDraft:
	default:
		return FollowUpDraft{}, errors.New("algorithm follow-up can only create a reviewable draft")
	}
	if err := result.Validate(); err != nil {
		return FollowUpDraft{}, err
	}
	return FollowUpDraft{
		RuleID: rule.RuleID, ProjectID: rule.ProjectID, SourceRunID: runID, Kind: rule.Kind,
		Title: rule.Title, Status: "pending_review", RequiresReview: true, CreatedAt: now,
		ResultKind: result.Kind, EvidenceReference: "algorithm-run:" + runID,
	}, nil
}
