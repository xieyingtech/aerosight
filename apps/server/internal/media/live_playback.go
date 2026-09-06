package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
	"unicode/utf16"
)

func IssueSimulatorLocator(secret string, pid int32, sid int64, ref string, now time.Time) (Access, error) {
	if len(utf16.Encode([]rune(secret))) < 16 {
		return Access{}, errors.New("PLAYBACK_SIGNING_SECRET_TOO_SHORT")
	}
	expiry := now.Add(time.Minute)
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d\n%d\n%s\n%d", pid, sid, ref, expiry.UnixMilli())
	return Access{URL: fmt.Sprintf("/api/projects/%d/live-streams/%d/simulator-playback?expires=%d&signature=%s", pid, sid, expiry.UnixMilli(), hex.EncodeToString(mac.Sum(nil))), ExpiresAt: expiry.UTC().Format("2006-01-02T15:04:05.000Z")}, nil
}

type PlaybackCandidate struct {
	Protocol string `json:"protocol"`
	URL      string `json:"url"`
}

func PlaybackCandidates(path, token, hls, webrtc string) []PlaybackCandidate {
	candidates := []PlaybackCandidate{}
	escaped := url.QueryEscape(token)
	if webrtc != "" {
		candidates = append(candidates, PlaybackCandidate{Protocol: "webrtc", URL: strings.TrimSuffix(webrtc, "/") + "/" + path + "?token=" + escaped})
	}
	if hls != "" {
		candidates = append(candidates, PlaybackCandidate{Protocol: "hls", URL: strings.TrimSuffix(hls, "/") + "/" + path + "/index.m3u8?token=" + escaped})
	}
	return candidates
}
