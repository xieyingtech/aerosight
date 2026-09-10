package connector

import (
	"errors"
	"sort"
	"strings"
)

type OnboardingPolicy string

const (
	OnboardingAutomatic   OnboardingPolicy = "automatic"
	OnboardingReview      OnboardingPolicy = "review"
	OnboardingObserveOnly OnboardingPolicy = "observe-only"
)

type DeviceTypeMatch struct {
	DeviceTypeID int64
	TypeKey      string
	Confidence   float64
}

type OnboardingInput struct {
	Policy            OnboardingPolicy
	ExternalID        string
	CurrentStatus     string
	Matches           []DeviceTypeMatch
	IdentityConflict  bool
	DuplicateExternal bool
}

type OnboardingDecision struct {
	Status       string
	DeviceTypeID int64
	Confidence   float64
	CreateDevice bool
	Reason       string
}

func EvaluateOnboarding(input OnboardingInput) (OnboardingDecision, error) {
	if strings.TrimSpace(input.ExternalID) == "" {
		return OnboardingDecision{}, errors.New("connector onboarding external identity is required")
	}
	switch input.Policy {
	case OnboardingAutomatic, OnboardingReview, OnboardingObserveOnly:
	default:
		return OnboardingDecision{}, errors.New("connector onboarding policy is invalid")
	}
	if input.CurrentStatus == "ignored" {
		return OnboardingDecision{Status: "ignored", Reason: "identity_explicitly_ignored"}, nil
	}
	if input.IdentityConflict || input.DuplicateExternal {
		return OnboardingDecision{Status: "conflicted", Reason: "external_identity_conflict"}, nil
	}
	matches := append([]DeviceTypeMatch(nil), input.Matches...)
	sort.Slice(matches, func(i, j int) bool {
		if matches[i].Confidence == matches[j].Confidence {
			return matches[i].DeviceTypeID < matches[j].DeviceTypeID
		}
		return matches[i].Confidence > matches[j].Confidence
	})
	if len(matches) == 0 || matches[0].DeviceTypeID <= 0 || matches[0].Confidence < 1 {
		return OnboardingDecision{Status: "discovered", Reason: "device_type_not_exact"}, nil
	}
	if len(matches) > 1 && matches[1].Confidence == matches[0].Confidence {
		return OnboardingDecision{Status: "discovered", Reason: "device_type_match_ambiguous"}, nil
	}
	decision := OnboardingDecision{
		Status: "discovered", DeviceTypeID: matches[0].DeviceTypeID,
		Confidence: matches[0].Confidence, Reason: "review_required",
	}
	if input.Policy == OnboardingObserveOnly {
		decision.Reason = "observe_only"
		return decision, nil
	}
	if input.Policy == OnboardingAutomatic {
		decision.Status = "managed"
		decision.CreateDevice = true
		decision.Reason = "unique_exact_match"
	}
	return decision, nil
}
