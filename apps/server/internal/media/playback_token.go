package media

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf16"
)

type PlaybackClaims struct {
	ProjectID int64    `json:"projectId"`
	StreamID  int64    `json:"streamId"`
	Path      string   `json:"path"`
	Protocols []string `json:"protocols"`
	Exp       int64    `json:"exp"`
}
type PlaybackToken struct {
	Token     string `json:"token"`
	ExpiresAt string `json:"expiresAt"`
}

func playbackSignature(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
func IssuePlaybackToken(secret string, claims PlaybackClaims, now time.Time, ttl int) (PlaybackToken, error) {
	if len(utf16.Encode([]rune(secret))) < 16 {
		return PlaybackToken{}, errors.New("PLAYBACK_SIGNING_SECRET_TOO_SHORT")
	}
	if ttl < 1 || ttl > 300 || len(claims.Protocols) == 0 || claims.ProjectID <= 0 || claims.StreamID <= 0 || claims.Path == "" {
		return PlaybackToken{}, errors.New("INVALID_PLAYBACK_TOKEN_INPUT")
	}
	protocols := []string{}
	seen := map[string]bool{}
	for _, p := range claims.Protocols {
		if p != "hls" && p != "webrtc" {
			return PlaybackToken{}, errors.New("INVALID_PLAYBACK_TOKEN_INPUT")
		}
		if !seen[p] {
			protocols = append(protocols, p)
			seen[p] = true
		}
	}
	claims.Protocols = protocols
	claims.Exp = now.Add(time.Duration(ttl) * time.Second).UnixMilli()
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(claims); err != nil {
		return PlaybackToken{}, err
	}
	payload := base64.RawURLEncoding.EncodeToString(bytes.TrimSuffix(buffer.Bytes(), []byte("\n")))
	return PlaybackToken{Token: payload + "." + playbackSignature(secret, payload), ExpiresAt: time.UnixMilli(claims.Exp).UTC().Format("2006-01-02T15:04:05.000Z")}, nil
}
func VerifyPlaybackToken(secret, token, path, protocol string, now time.Time) (PlaybackClaims, bool) {
	var claims PlaybackClaims
	if len(utf16.Encode([]rune(secret))) < 16 || (protocol != "webrtc" && protocol != "hls") {
		return claims, false
	}
	parts := strings.Split(token, ".")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" || !hmac.Equal([]byte(parts[1]), []byte(playbackSignature(secret, parts[0]))) {
		return claims, false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil || json.Unmarshal(raw, &claims) != nil {
		return PlaybackClaims{}, false
	}
	if claims.ProjectID < 1 || claims.ProjectID > 9007199254740991 || claims.StreamID < 1 || claims.StreamID > 9007199254740991 || claims.Exp <= now.UnixMilli() || claims.Path != path {
		return PlaybackClaims{}, false
	}
	for _, p := range claims.Protocols {
		if p == protocol {
			return claims, true
		}
	}
	return PlaybackClaims{}, false
}
