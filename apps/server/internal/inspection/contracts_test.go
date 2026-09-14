package inspection

import (
	"encoding/json"
	"testing"
	"time"
)

func pointer[T any](v T) *T { return &v }
func testObservation() Observation {
	return Observation{
		ID: "observation-1", ContractVersion: ContractVersion,
		Run:  RunRef{Scope: Scope{ProjectID: 1, TeamID: 2}, RunID: 100, StepID: 101},
		Mode: Assets, Assets: []AssetRef{{Scope: Scope{ProjectID: 1, TeamID: 2}, AssetID: 7, Version: 3, SourceRunID: pointer(int64(50))}},
		Completeness: Complete, ScopeDescription: "用户选定的照片", ObservedFrom: time.Unix(100, 0), ObservedTo: time.Unix(200, 0),
	}
}
func testEvidence() EvidenceSet {
	return EvidenceSet{ID: "evidence-1", Run: RunRef{Scope: Scope{ProjectID: 1, TeamID: 2}, RunID: 100, StepID: 102},
		ObservationID: "observation-1", Source: "external", ModelVersion: "model-v1", Completeness: Complete,
		TargetAlgorithmConfirmed: true, EvidenceRefs: []string{"image-7-v3"}, Candidates: []Candidate{{ID: "candidate-1", EvidenceRefs: []string{"image-7-v3"}, Position: Position{Source: "capture", Quality: "unverified"}}}}
}

func TestObservationAllowsReanalysisWithoutChangingProvenance(t *testing.T) {
	o := testObservation()
	f := FlightRef{Scope: o.Run.Scope, ConnectorID: 5, FlightUUID: "flight-1", ProjectedRunID: pointer(int64(70))}
	o.Mode = ExistingFlight
	o.Flight = &f
	o.Assets[0].Flight = &f
	before, _ := json.Marshal(o.Assets)
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	// A second business run references the same remote flight and asset versions.
	o.Run.RunID = 200
	o.Run.StepID = 201
	if err := o.Validate(); err != nil {
		t.Fatal(err)
	}
	after, _ := json.Marshal(o.Assets)
	if string(before) != string(after) || *o.Flight.ProjectedRunID != 70 || *o.Assets[0].SourceRunID != 50 {
		t.Fatal("validation changed source provenance")
	}
}

func TestObservationRejectsScopeAndIdentityConfusion(t *testing.T) {
	cases := map[string]func(*Observation){
		"other project":   func(o *Observation) { o.Assets[0].ProjectID = 9 },
		"other team":      func(o *Observation) { o.Assets[0].TeamID = 9 },
		"duplicate asset": func(o *Observation) { o.Assets = append(o.Assets, o.Assets[0]) },
		"mutable version": func(o *Observation) { o.Assets[0].Version = 0 },
		"empty assets":    func(o *Observation) { o.Assets = nil },
		"other flight": func(o *Observation) {
			o.Mode = ExistingFlight
			o.Flight = &FlightRef{Scope: o.Run.Scope, ConnectorID: 1, FlightUUID: "a"}
			o.Assets[0].Flight = &FlightRef{Scope: o.Run.Scope, ConnectorID: 1, FlightUUID: "b"}
		},
		"unknown completeness": func(o *Observation) { o.Completeness = "success" },
	}
	for name, modify := range cases {
		t.Run(name, func(t *testing.T) {
			o := testObservation()
			modify(&o)
			if o.Validate() == nil {
				t.Fatal("accepted invalid observation")
			}
		})
	}
}

func TestMediaCompletenessRequiresFlightAndComparableCounts(t *testing.T) {
	cases := []struct {
		name   string
		status MediaStatus
		want   Completeness
	}{
		{"complete", MediaStatus{true, true, pointer(2), pointer(2), 2, 2, false}, Complete},
		{"unknown zero", MediaStatus{true, true, nil, nil, 0, 0, false}, Partial},
		{"known zero", MediaStatus{true, true, pointer(0), pointer(0), 0, 0, false}, Complete},
		{"flight not finished", MediaStatus{false, true, pointer(2), pointer(2), 2, 2, false}, Partial},
		{"missing upload", MediaStatus{true, true, pointer(2), pointer(1), 1, 1, false}, Partial},
		{"missing access", MediaStatus{true, true, pointer(2), pointer(2), 2, 1, false}, Partial},
		{"truncated", MediaStatus{true, true, pointer(2), pointer(2), 2, 2, true}, Partial},
		{"different scope", MediaStatus{true, false, pointer(2), pointer(2), 2, 2, false}, Partial},
		{"invalid counts", MediaStatus{true, true, pointer(-1), pointer(-1), 0, 0, false}, Unavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.status.Completeness(); got != tc.want {
				t.Fatalf("got %s want %s", got, tc.want)
			}
		})
	}
}

func TestEvidenceRejectsCrossRunAndInventedReferences(t *testing.T) {
	o := testObservation()
	e := testEvidence()
	if err := e.Validate(o); err != nil {
		t.Fatal(err)
	}
	for _, modify := range []func(*EvidenceSet){
		func(e *EvidenceSet) { e.Run.RunID++ }, func(e *EvidenceSet) { e.Run.ProjectID++ },
		func(e *EvidenceSet) { e.Candidates[0].EvidenceRefs = []string{"invented"} },
		func(e *EvidenceSet) { e.Source = "flighthub-ai" },
	} {
		e := testEvidence()
		modify(&e)
		if e.Validate(o) == nil {
			t.Fatal("accepted invalid evidence")
		}
	}
}

func TestAssessmentRejectsUnsupportedConclusions(t *testing.T) {
	evidence := testEvidence()
	makeAssessment := func(action string) Assessment {
		return Assessment{ID: "assessment-1", Run: RunRef{Scope: evidence.Run.Scope, RunID: 100, StepID: 103}, EvidenceSetID: evidence.ID, Revision: 1, Decisions: []Decision{{CandidateID: "candidate-1", Action: action, Reason: "基于选定图片", EvidenceRefs: []string{"image-7-v3"}}}}
	}
	a := makeAssessment("create")
	if err := a.Validate(evidence, nil); err != nil {
		t.Fatal(err)
	}
	a.Decisions[0].EvidenceRefs = []string{"invented"}
	if a.Validate(evidence, nil) == nil {
		t.Fatal("accepted invented evidence")
	}
	a = makeAssessment("update")
	a.Decisions[0].IssueID = pointer(int64(12))
	if a.Validate(evidence, nil) == nil {
		t.Fatal("accepted unapproved issue")
	}
	if err := a.Validate(evidence, map[int64]bool{12: true}); err != nil {
		t.Fatal(err)
	}
	a = makeAssessment("no_issue")
	for _, modify := range []func(*EvidenceSet){func(e *EvidenceSet) { e.TargetAlgorithmConfirmed = false }, func(e *EvidenceSet) { e.Completeness = Partial }, func(e *EvidenceSet) { e.Completeness = AlertOnly }} {
		e := testEvidence()
		modify(&e)
		if a.Validate(e, nil) == nil {
			t.Fatal("accepted unsupported no_issue")
		}
	}
	a = makeAssessment("needs_review")
	if !a.NeedsReview() {
		t.Fatal("review not detected")
	}
	a.Run.ProjectID++
	if a.Validate(evidence, nil) == nil {
		t.Fatal("accepted cross-project assessment")
	}
}

func TestSourceKeysDoNotDependOnTaskVersionOrRun(t *testing.T) {
	scope := Scope{ProjectID: 1, TeamID: 2}
	first, err := SourceKey(scope, 5, "alert-1", 0, 0, "", "")
	if err != nil {
		t.Fatal(err)
	}
	replay, err := SourceKey(scope, 5, "alert-1", 0, 0, "", "")
	if err != nil || first != replay {
		t.Fatal("unstable alert identity")
	}
	other, _ := SourceKey(Scope{ProjectID: 2, TeamID: 2}, 5, "alert-1", 0, 0, "", "")
	if other == first {
		t.Fatal("cross-project collision")
	}
	first, err = SourceKey(scope, 0, "", 7, 3, "box-1", "suspected-change")
	if err != nil {
		t.Fatal(err)
	}
	other, _ = SourceKey(scope, 0, "", 7, 4, "box-1", "suspected-change")
	if first == other {
		t.Fatal("different asset versions collided")
	}
	if _, err = SourceKey(scope, 5, "alert-1", 7, 3, "box-1", "suspected-change"); err == nil {
		t.Fatal("accepted ambiguous identity")
	}
}

func TestAssessmentCannotSilentlyOmitCandidates(t *testing.T) {
	evidence := testEvidence()
	a := Assessment{ID: "assessment-1", Run: RunRef{Scope: evidence.Run.Scope, RunID: 100, StepID: 103}, EvidenceSetID: evidence.ID, Revision: 1,
		Decisions: []Decision{{Action: "no_issue", Reason: "whole scope", EvidenceRefs: evidence.EvidenceRefs}}}
	if a.Validate(evidence, nil) == nil {
		t.Fatal("accepted no_issue without addressing existing candidate")
	}
	a.Decisions[0].Action = "needs_review"
	if err := a.Validate(evidence, nil); err != nil {
		t.Fatal(err)
	}
	evidence.Candidates = nil
	a.Decisions[0].Action = "no_issue"
	if err := a.Validate(evidence, nil); err != nil {
		t.Fatal(err)
	}
}

func TestEvidenceCannotUpgradeAnUnconfirmedPartialObservation(t *testing.T) {
	o, e := testObservation(), testEvidence()
	o.Completeness = Partial
	if e.Validate(o) == nil {
		t.Fatal("silently upgraded a partial observation")
	}
	o.LimitedScopeConfirmedBy = pointer(int64(42))
	if err := e.Validate(o); err != nil {
		t.Fatal(err)
	}
	o.Completeness = Unavailable
	if e.Validate(o) == nil {
		t.Fatal("unavailable evidence became complete")
	}
}
