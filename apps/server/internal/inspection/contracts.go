// Package inspection defines the evidence contracts shared by Task inspection
// steps. Scope and provenance must be loaded by the server, never trusted from
// provider responses. These contracts do not mutate connector projections.
package inspection

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"strings"
	"time"
)

const ContractVersion = "aerosight/inspection/v1"

type Scope struct {
	ProjectID int `json:"projectId"`
	TeamID    int `json:"teamId"`
}

func (s Scope) valid() bool { return s.ProjectID > 0 && s.TeamID > 0 }

// RunRef always identifies the business workflow, not a connector's shadow Run.
type RunRef struct {
	Scope
	RunID  int64 `json:"runId"`
	StepID int64 `json:"stepId"`
}

func (r RunRef) valid() bool { return r.Scope.valid() && r.RunID > 0 && r.StepID > 0 }

type FlightRef struct {
	Scope
	ConnectorID    int64  `json:"connectorId"`
	FlightUUID     string `json:"flightUuid"`
	ProjectedRunID *int64 `json:"projectedRunId,omitempty"`
}

func (f FlightRef) valid() bool {
	return f.Scope.valid() && f.ConnectorID > 0 && validIdentity(f.FlightUUID) &&
		(f.ProjectedRunID == nil || *f.ProjectedRunID > 0)
}

type AssetRef struct {
	// SourceDeviceMode records the associated device mode when the observation is sealed; it does not certify image authenticity.
	SourceDeviceMode string `json:"sourceDeviceMode,omitempty"`
	Scope
	AssetID        int64  `json:"assetId"`
	Version        int    `json:"version"`
	ChecksumSHA256 string `json:"checksumSha256,omitempty"`
	ObjectVersion  string `json:"objectVersion,omitempty"`
	// SourceRunID is provenance, not the Run which currently analyzes the asset.
	SourceRunID *int64     `json:"sourceRunId,omitempty"`
	Flight      *FlightRef `json:"flight,omitempty"`
}

func validChecksum(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == 32 && value == strings.ToLower(value)
}

func (a AssetRef) valid() bool {
	return (a.SourceDeviceMode == "" || a.SourceDeviceMode == "simulator" || a.SourceDeviceMode == "unverified") && a.Scope.valid() && a.AssetID > 0 && a.Version > 0 && (a.ChecksumSHA256 == "" || validChecksum(a.ChecksumSHA256)) &&
		(a.SourceRunID == nil || *a.SourceRunID > 0) &&
		(a.Flight == nil || a.Flight.valid() && a.Flight.Scope == a.Scope)
}

type ObservationMode string

const (
	ExistingFlight  ObservationMode = "existing-flight"
	Assets          ObservationMode = "assets"
	FlightHubFlight ObservationMode = "flighthub-flight"
)

type Completeness string

const (
	Complete    Completeness = "complete"
	Partial     Completeness = "partial"
	AlertOnly   Completeness = "alert-only"
	Unavailable Completeness = "unavailable"
)

type Observation struct {
	MediaStatus             *MediaStatus    `json:"mediaStatus,omitempty"`
	DataGaps                []string        `json:"dataGaps,omitempty"`
	ID                      string          `json:"id"`
	ContractVersion         string          `json:"contractVersion"`
	Run                     RunRef          `json:"run"`
	Mode                    ObservationMode `json:"mode"`
	Flight                  *FlightRef      `json:"flight,omitempty"`
	Assets                  []AssetRef      `json:"assets"`
	Completeness            Completeness    `json:"completeness"`
	ScopeDescription        string          `json:"scopeDescription"`
	ObservedFrom            time.Time       `json:"observedFrom"`
	ObservedTo              time.Time       `json:"observedTo"`
	LimitedScopeConfirmedBy *int64          `json:"limitedScopeConfirmedBy,omitempty"`
}

func (o Observation) Validate() error {
	if !validIdentity(o.ID) || o.ContractVersion != ContractVersion || !o.Run.valid() || strings.TrimSpace(o.ScopeDescription) == "" ||
		o.ObservedFrom.IsZero() || o.ObservedTo.Before(o.ObservedFrom) {
		return errors.New("INSPECTION_OBSERVATION_INVALID")
	}
	if o.LimitedScopeConfirmedBy != nil && *o.LimitedScopeConfirmedBy <= 0 {
		return errors.New("INSPECTION_SCOPE_CONFIRMATION_INVALID")
	}
	switch o.Mode {
	case Assets:
		if o.Flight != nil || len(o.Assets) == 0 {
			return errors.New("INSPECTION_SOURCE_INVALID")
		}
	case ExistingFlight, FlightHubFlight:
		if o.Flight == nil || !o.Flight.valid() || o.Flight.Scope != o.Run.Scope {
			return errors.New("INSPECTION_FLIGHT_SCOPE_INVALID")
		}
	default:
		return errors.New("INSPECTION_SOURCE_INVALID")
	}
	switch o.Completeness {
	case Complete, Partial, AlertOnly, Unavailable:
	default:
		return errors.New("INSPECTION_COMPLETENESS_INVALID")
	}
	seen := map[int64]bool{}
	for _, a := range o.Assets {
		if !a.valid() || a.Scope != o.Run.Scope {
			return errors.New("INSPECTION_ASSET_SCOPE_INVALID")
		}
		if seen[a.AssetID] {
			return errors.New("INSPECTION_ASSET_DUPLICATE")
		}
		seen[a.AssetID] = true
		if o.Mode != Assets && (a.Flight == nil || !sameFlight(*o.Flight, *a.Flight)) {
			return errors.New("INSPECTION_ASSET_FLIGHT_MISMATCH")
		}
	}
	return nil
}

func sameFlight(a, b FlightRef) bool {
	return a.Scope == b.Scope && a.ConnectorID == b.ConnectorID && a.FlightUUID == b.FlightUUID
}

// MediaStatus describes the provider's entire file-count scope. Nil counters
// mean unavailable; zero is a known value. Selected image counts must not be
// compared with total photo/video counters by a caller.
type MediaStatus struct {
	FlightSucceeded  bool `json:"flightSucceeded"`
	CountsComparable bool `json:"countsComparable"`
	Expected         *int `json:"expected"`
	Uploaded         *int `json:"uploaded"`
	Listed           int  `json:"listed"`
	Accessible       int  `json:"accessible"`
	Truncated        bool `json:"truncated"`
}

func (m MediaStatus) Completeness() Completeness {
	if m.Listed < 0 || m.Accessible < 0 || m.Accessible > m.Listed ||
		m.Expected != nil && *m.Expected < 0 || m.Uploaded != nil && *m.Uploaded < 0 {
		return Unavailable
	}
	if !m.FlightSucceeded || !m.CountsComparable || m.Expected == nil || m.Uploaded == nil || m.Truncated {
		return Partial
	}
	if *m.Expected != *m.Uploaded || m.Listed != *m.Expected || m.Accessible != m.Listed {
		return Partial
	}
	return Complete
}

type Position struct {
	Source    string   `json:"source"`  // target, capture, unknown
	Quality   string   `json:"quality"` // verified, unverified, image-only
	CRS       string   `json:"crs,omitempty"`
	Longitude *float64 `json:"longitude,omitempty"`
	Latitude  *float64 `json:"latitude,omitempty"`
}

func (p Position) Validate() error {
	if p.Source != "target" && p.Source != "capture" && p.Source != "unknown" {
		return errors.New("INSPECTION_POSITION_INVALID")
	}
	if p.Quality != "verified" && p.Quality != "unverified" && p.Quality != "image-only" {
		return errors.New("INSPECTION_POSITION_INVALID")
	}
	if (p.Longitude == nil) != (p.Latitude == nil) {
		return errors.New("INSPECTION_POSITION_INVALID")
	}
	if p.Longitude != nil && (p.CRS != "EPSG:4326" || !(*p.Longitude >= -180 && *p.Longitude <= 180) || !(*p.Latitude >= -90 && *p.Latitude <= 90)) {
		return errors.New("INSPECTION_POSITION_INVALID")
	}
	if p.Quality == "verified" && (p.Source == "unknown" || p.Longitude == nil) {
		return errors.New("INSPECTION_POSITION_INVALID")
	}
	return nil
}

// Candidate identity comes from persisted evidence, not from the LLM.
type Candidate struct {
	ID           string   `json:"id"`
	EvidenceRefs []string `json:"evidenceRefs"`
	Position     Position `json:"position"`
}

type EvidenceSet struct {
	ExternalResults          []ExternalEvidence    `json:"externalResults,omitempty"`
	NativeAlerts             []NativeAlertEvidence `json:"nativeAlerts,omitempty"`
	DataGaps                 []string              `json:"dataGaps,omitempty"`
	ID                       string                `json:"id"`
	Run                      RunRef                `json:"run"`
	ObservationID            string                `json:"observationId"`
	Source                   string                `json:"source"`       // flighthub-ai or external
	ModelVersion             string                `json:"modelVersion"` // "unknown" if the upstream does not expose it
	Completeness             Completeness          `json:"completeness"`
	TargetAlgorithmConfirmed bool                  `json:"targetAlgorithmConfirmed"`
	Candidates               []Candidate           `json:"candidates"`
	EvidenceRefs             []string              `json:"evidenceRefs"`
}

func (e EvidenceSet) Validate(o Observation) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if !validIdentity(e.ID) || !e.Run.valid() || e.Run.Scope != o.Run.Scope || e.Run.RunID != o.Run.RunID ||
		e.ObservationID != o.ID || strings.TrimSpace(e.ModelVersion) == "" {
		return errors.New("INSPECTION_EVIDENCE_SCOPE_INVALID")
	}
	if e.Source != "flighthub-ai" && e.Source != "external" {
		return errors.New("INSPECTION_DETECTION_SOURCE_INVALID")
	}
	if e.Source == "flighthub-ai" && o.Flight == nil {
		return errors.New("INSPECTION_DETECTION_SOURCE_INVALID")
	}
	if e.Completeness != Complete && e.Completeness != Partial && e.Completeness != AlertOnly && e.Completeness != Unavailable {
		return errors.New("INSPECTION_COMPLETENESS_INVALID")
	}
	if e.Completeness == Complete && o.Completeness != Complete {
		if o.Completeness == Unavailable || o.LimitedScopeConfirmedBy == nil || e.Source == "flighthub-ai" && o.Completeness == AlertOnly {
			return errors.New("INSPECTION_SCOPE_NOT_CONFIRMED")
		}
	}
	refs, err := identitySet(e.EvidenceRefs)
	if err != nil {
		return err
	}
	candidates := map[string]bool{}
	for _, c := range e.Candidates {
		if !validIdentity(c.ID) || candidates[c.ID] || len(c.EvidenceRefs) == 0 {
			return errors.New("INSPECTION_CANDIDATE_INVALID")
		}
		candidates[c.ID] = true
		if err := c.Position.Validate(); err != nil {
			return err
		}
		if err := referencesWithin(c.EvidenceRefs, refs); err != nil {
			return err
		}
	}
	return nil
}

func (e EvidenceSet) CanConcludeNoIssue() bool {
	return e.Completeness == Complete && e.TargetAlgorithmConfirmed
}

type Decision struct {
	CandidateID        string   `json:"candidateId,omitempty"`
	Action             string   `json:"action"`
	Reason             string   `json:"reason"`
	EvidenceRefs       []string `json:"evidenceRefs"`
	MissingInformation []string `json:"missingInformation"`
	IssueID            *int64   `json:"issueId,omitempty"`
}

type Assessment struct {
	ID            string     `json:"id"`
	Run           RunRef     `json:"run"`
	EvidenceSetID string     `json:"evidenceSetId"`
	Revision      int        `json:"revision"`
	Decisions     []Decision `json:"decisions"`
}

// Validate checks the model's proposal against server-loaded evidence and an
// authorized update-candidate set. It does not grant permission to write issues.
func (a Assessment) Validate(e EvidenceSet, allowedIssues map[int64]bool) error {
	if !validIdentity(a.ID) || !a.Run.valid() || a.Run.Scope != e.Run.Scope || a.Run.RunID != e.Run.RunID || a.EvidenceSetID != e.ID || a.Revision < 1 {
		return errors.New("INSPECTION_ASSESSMENT_SCOPE_INVALID")
	}
	if len(a.Decisions) == 0 {
		return errors.New("INSPECTION_DECISION_REQUIRED")
	}
	candidates := map[string]Candidate{}
	for _, c := range e.Candidates {
		candidates[c.ID] = c
	}
	allRefs, err := identitySet(e.EvidenceRefs)
	if err != nil {
		return err
	}
	seen := map[string]bool{}
	globalReview := false
	for _, d := range a.Decisions {
		if strings.TrimSpace(d.Reason) == "" || len(d.EvidenceRefs) == 0 {
			return errors.New("INSPECTION_DECISION_INVALID")
		}
		if err := referencesWithin(d.EvidenceRefs, allRefs); err != nil {
			return err
		}
		if d.CandidateID != "" {
			c, ok := candidates[d.CandidateID]
			if !ok || seen[d.CandidateID] {
				return errors.New("INSPECTION_CANDIDATE_INVALID")
			}
			seen[d.CandidateID] = true
			refs, err := identitySet(c.EvidenceRefs)
			if err != nil {
				return err
			}
			if err := referencesWithin(d.EvidenceRefs, refs); err != nil {
				return err
			}
		}
		switch d.Action {
		case "create":
			if d.CandidateID == "" || d.IssueID != nil {
				return errors.New("INSPECTION_DECISION_INVALID")
			}
		case "update":
			if d.CandidateID == "" || d.IssueID == nil || !allowedIssues[*d.IssueID] {
				return errors.New("INSPECTION_ISSUE_SCOPE_INVALID")
			}
		case "no_issue":
			if !e.CanConcludeNoIssue() || d.IssueID != nil {
				return errors.New("INSPECTION_NO_ISSUE_UNSUPPORTED")
			}
		case "needs_review":
			if d.CandidateID == "" {
				globalReview = true
			}
			if d.IssueID != nil {
				return errors.New("INSPECTION_DECISION_INVALID")
			}
		default:
			return errors.New("INSPECTION_DECISION_INVALID")
		}
	}
	if !globalReview && len(seen) != len(candidates) {
		return errors.New("INSPECTION_DECISIONS_INCOMPLETE")
	}
	return nil
}

func (a Assessment) NeedsReview() bool {
	for _, d := range a.Decisions {
		if d.Action == "needs_review" {
			return true
		}
	}
	return false
}

// SourceKey is stable across task versions and runs, and contains no signed URL.
func SourceKey(scope Scope, connectorID int64, alertID string, assetID int64, version int, detectionKey, problemType string) (string, error) {
	if !scope.valid() {
		return "", errors.New("INSPECTION_SOURCE_SCOPE_INVALID")
	}
	var parts []any
	if connectorID > 0 && validIdentity(alertID) && assetID == 0 && version == 0 && detectionKey == "" {
		parts = []any{"flighthub-ai", scope.ProjectID, connectorID, alertID}
	} else if connectorID == 0 && alertID == "" && assetID > 0 && version > 0 && validIdentity(detectionKey) && validIdentity(problemType) {
		parts = []any{"external", scope.ProjectID, assetID, version, detectionKey, problemType}
	} else {
		return "", errors.New("INSPECTION_SOURCE_IDENTITY_INVALID")
	}
	raw, _ := json.Marshal(parts)
	sum := sha256.Sum256(raw)
	return "inspection:" + hex.EncodeToString(sum[:]), nil
}

func validIdentity(s string) bool { return strings.TrimSpace(s) == s && s != "" && len(s) <= 256 }
func identitySet(values []string) (map[string]bool, error) {
	set := map[string]bool{}
	for _, v := range values {
		if !validIdentity(v) || set[v] {
			return nil, errors.New("INSPECTION_EVIDENCE_REFERENCE_INVALID")
		}
		set[v] = true
	}
	return set, nil
}
func referencesWithin(values []string, allowed map[string]bool) error {
	if _, err := identitySet(values); err != nil {
		return err
	}
	for _, v := range values {
		if !allowed[v] {
			return errors.New("INSPECTION_EVIDENCE_REFERENCE_INVALID")
		}
	}
	return nil
}
