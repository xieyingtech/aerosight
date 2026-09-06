package media

import (
	"strings"
	"testing"
	"time"
)

func TestPlaybackTokenOriginalTSContract(t *testing.T) {
	now, _ := time.Parse(time.RFC3339, "2026-08-27T00:00:00Z")
	// Captured from the original MediaPlaybackTokenIssuer with fixed time.
	expected := "eyJwcm9qZWN0SWQiOjE3LCJzdHJlYW1JZCI6NDIsInBhdGgiOiJkZW1vL2Flcm9zaWdodC90ZXN0IiwicHJvdG9jb2xzIjpbIndlYnJ0YyIsImhscyJdLCJleHAiOjE3ODc3ODg4NjAwMDB9.AYG-uffaUwJjri0_uIUEzKjs7EqyWdbNkQEVBpnaiYQ"
	result, err := IssuePlaybackToken("0123456789abcdef", PlaybackClaims{ProjectID: 17, StreamID: 42, Path: "demo/aerosight/test", Protocols: []string{"webrtc", "hls", "hls"}}, now, 60)
	if err != nil || result.Token != expected || result.ExpiresAt != "2026-08-27T00:01:00.000Z" {
		t.Fatalf("contract %+v %v", result, err)
	}
	for _, protocol := range []string{"webrtc", "hls"} {
		claims, ok := VerifyPlaybackToken("0123456789abcdef", expected, "demo/aerosight/test", protocol, now)
		if !ok || claims.ProjectID != 17 || claims.StreamID != 42 || len(claims.Protocols) != 2 {
			t.Fatalf("old token %+v %v", claims, ok)
		}
	}
	for _, v := range []struct {
		token, path, protocol string
		now                   time.Time
	}{{expected, "other", "hls", now}, {expected, "demo/aerosight/test", "rtmp", now}, {expected, "demo/aerosight/test", "hls", now.Add(time.Minute)}, {expected + ".extra", "demo/aerosight/test", "hls", now}, {strings.Replace(expected, "AYG", "BYG", 1), "demo/aerosight/test", "hls", now}} {
		if _, ok := VerifyPlaybackToken("0123456789abcdef", v.token, v.path, v.protocol, v.now); ok {
			t.Fatalf("invalid accepted %+v", v)
		}
	}
}
