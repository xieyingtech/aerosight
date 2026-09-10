package httpapi

import "testing"

func TestFlightHubLiveMediaSummaryRejectsSecretsAndNestedData(t *testing.T) {
	got := safeLiveMediaSummary(map[string]any{
		"name": "recording", "duration": float64(12), "ready": true,
		"downloadUrl": "https://private.example", "accessToken": "secret",
		"deviceId": "remote-id", "rtspEndpoint": "private",
		"metadata": map[string]any{"token": "secret"},
	})
	if len(got) != 3 || got["name"] != "recording" || got["duration"] != float64(12) || got["ready"] != true {
		t.Fatalf("unexpected public summary: %#v", got)
	}
	if len(safeLiveMediaSummary(nil)) != 0 {
		t.Fatal("nil summary must be an empty object")
	}
}
