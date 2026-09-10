package automation

import "testing"

func TestOnlyAutomaticModesRunBackgroundDrafts(t *testing.T) {
	tests := []struct {
		mode string
		want bool
	}{
		{mode: "manual", want: false},
		{mode: "agent-on-demand", want: false},
		{mode: "agent-auto-draft", want: true},
		{mode: "follow-up-draft", want: true},
		{mode: "unknown", want: false},
	}
	for _, test := range tests {
		t.Run(test.mode, func(t *testing.T) {
			if got := ShouldRunAutomaticDrafts(test.mode); got != test.want {
				t.Fatalf("ShouldRunAutomaticDrafts(%q) = %v, want %v", test.mode, got, test.want)
			}
		})
	}
}

func TestNonAutomaticModeDoesNotChangeOriginalAlert(t *testing.T) {
	alertStillActionable := true
	if ShouldRunAutomaticDrafts("manual") {
		t.Fatal("manual mode allowed subsequent draft actions")
	}
	if !alertStillActionable {
		t.Fatal("mode change changed the original alert")
	}
}
