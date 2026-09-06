package media

import (
	"testing"
	"time"
)

func TestSimulatorLocatorOriginalTSContract(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-08-27T00:00:00Z")
	got, err := IssueSimulatorLocator("0123456789abcdef", 17, 42, "simulator://devices/9/main", now)
	if err != nil || got.URL != "/api/projects/17/live-streams/42/simulator-playback?expires=1787788860000&signature=ca8c19812a2395e72d4bb84bee93226c39e7269e8c23d619c59940c8f0140bd8" {
		t.Fatalf("fixture %+v %v", got, err)
	}
	candidates := PlaybackCandidates("demo/aerosight/test", "token", "https://media.example/hls/", "https://media.example/rtc/")
	if len(candidates) != 2 || candidates[0].Protocol != "webrtc" || candidates[1].URL != "https://media.example/hls/demo/aerosight/test/index.m3u8?token=token" {
		t.Fatalf("candidates %+v", candidates)
	}
}
