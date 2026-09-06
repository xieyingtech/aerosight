package automation

import (
	"context"
	"fmt"
)

type Draft struct {
	Type           string              `json:"type"`
	Status         string              `json:"status"`
	Title          string              `json:"title"`
	Payload        map[string]any      `json:"payload"`
	Evidence       []map[string]string `json:"evidence"`
	ExternalEffect string              `json:"externalEffect"`
}

func PlanAutomaticDrafts(mode, eventID string) []Draft {
	if mode != "agent-auto-draft" && mode != "follow-up-draft" {
		return nil
	}
	evidence := []map[string]string{{"type": "event", "id": eventID, "version": "captured-at-run"}}
	drafts := []Draft{
		{Type: "report", Status: "draft", Title: "告警分析报告草案", Payload: map[string]any{"eventId": eventID, "requiresConfirmation": true}, Evidence: evidence, ExternalEffect: "none"},
		{Type: "issue", Status: "draft", Title: "告警复核工单草案", Payload: map[string]any{"eventId": eventID, "requiresConfirmation": true}, Evidence: evidence, ExternalEffect: "none"},
	}
	if mode == "follow-up-draft" {
		drafts = append(drafts, Draft{Type: "follow-up-task", Status: "draft", Title: "后续复核任务草案",
			Payload: map[string]any{"eventId": eventID, "objective": fmt.Sprintf("复核告警 %s", eventID), "requiresPreflight": true, "requiresApproval": true}, Evidence: evidence, ExternalEffect: "none"})
	}
	return drafts
}

type DeterministicDraftGenerator struct{}

func (DeterministicDraftGenerator) Generate(_ context.Context, request Request) (Result, error) {
	return Result{Drafts: PlanAutomaticDrafts(request.Mode, request.PerceptionEventID)}, nil
}
