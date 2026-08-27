package automation

import "testing"

func TestAgentAutoDraftProducesOnlyReportAndIssueDrafts(t *testing.T) {
	drafts := PlanAutomaticDrafts("agent-auto-draft", "event-1")
	if len(drafts) != 2 || drafts[0].Type != "report" || drafts[1].Type != "issue" {
		t.Fatalf("unexpected automatic drafts: %+v", drafts)
	}
	for _, draft := range drafts {
		if draft.Status != "draft" || draft.ExternalEffect != "none" {
			t.Fatalf("automatic artifact escaped draft-only boundary: %+v", draft)
		}
	}
}

func TestFollowUpModeAddsUnpublishedTaskDraft(t *testing.T) {
	drafts := PlanAutomaticDrafts("follow-up-draft", "event-2")
	if len(drafts) != 3 || drafts[2].Type != "follow-up-task" || drafts[2].Payload["requiresApproval"] != true {
		t.Fatalf("follow-up draft missing safety requirements: %+v", drafts)
	}
}

func TestManualAndOnDemandModesDoNotRunAutomatically(t *testing.T) {
	if PlanAutomaticDrafts("manual", "event-3") != nil || PlanAutomaticDrafts("agent-on-demand", "event-3") != nil {
		t.Fatal("non-automatic policy created an automatic draft")
	}
}
