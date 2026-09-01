package connector

import "testing"

func TestAutomaticOnboardingRequiresOneExactConflictFreeMatch(t *testing.T) {
	exact := DeviceTypeMatch{DeviceTypeID: 7, TypeKey: "acme.temperature", Confidence: 1}
	tests := []struct {
		name    string
		input   OnboardingInput
		status  string
		created bool
		reason  string
	}{
		{"unique", OnboardingInput{Policy: OnboardingAutomatic, ExternalID: "sensor-1", Matches: []DeviceTypeMatch{exact}}, "managed", true, "unique_exact_match"},
		{"unknown", OnboardingInput{Policy: OnboardingAutomatic, ExternalID: "sensor-1"}, "discovered", false, "device_type_not_exact"},
		{"low confidence", OnboardingInput{Policy: OnboardingAutomatic, ExternalID: "sensor-1", Matches: []DeviceTypeMatch{{DeviceTypeID: 7, Confidence: .9}}}, "discovered", false, "device_type_not_exact"},
		{"ambiguous", OnboardingInput{Policy: OnboardingAutomatic, ExternalID: "sensor-1", Matches: []DeviceTypeMatch{exact, {DeviceTypeID: 8, Confidence: 1}}}, "discovered", false, "device_type_match_ambiguous"},
		{"conflict", OnboardingInput{Policy: OnboardingAutomatic, ExternalID: "sensor-1", Matches: []DeviceTypeMatch{exact}, IdentityConflict: true}, "conflicted", false, "external_identity_conflict"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			decision, err := EvaluateOnboarding(test.input)
			if err != nil || decision.Status != test.status || decision.CreateDevice != test.created || decision.Reason != test.reason {
				t.Fatalf("unexpected decision: %#v err=%v", decision, err)
			}
		})
	}
}

func TestReviewObserveAndIgnoredIdentitiesNeverAutoCreate(t *testing.T) {
	exact := []DeviceTypeMatch{{DeviceTypeID: 7, Confidence: 1}}
	for _, fixture := range []struct {
		policy OnboardingPolicy
		status string
		reason string
	}{
		{OnboardingReview, "discovered", "review_required"},
		{OnboardingObserveOnly, "discovered", "observe_only"},
	} {
		decision, err := EvaluateOnboarding(OnboardingInput{Policy: fixture.policy, ExternalID: "sensor-1", Matches: exact})
		if err != nil || decision.CreateDevice || decision.Status != fixture.status || decision.Reason != fixture.reason {
			t.Fatalf("policy %s widened onboarding: %#v err=%v", fixture.policy, decision, err)
		}
	}
	ignored, err := EvaluateOnboarding(OnboardingInput{Policy: OnboardingAutomatic, ExternalID: "sensor-1", CurrentStatus: "ignored", Matches: exact})
	if err != nil || ignored.Status != "ignored" || ignored.CreateDevice {
		t.Fatalf("ignored identity was revived by discovery: %#v err=%v", ignored, err)
	}
}
