package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/media"
	"crypto/hmac"
	"database/sql"
	"encoding/json"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) mediaAuth(c *gin.Context) {
	deny := func() { s.failure(c, 401, "MEDIA_AUTH_DENIED") }
	// MediaMTX may send additional fields; accept them as the previous handler
	// did, without logging credentials or requiring a browser cookie/CSRF token.
	var body map[string]any
	if strictJSON(c, &body) != nil {
		deny()
		return
	}
	value := func(key string) string { s, _ := body[key].(string); return s }
	equal := func(actual, expected string) bool {
		return expected != "" && hmac.Equal([]byte(actual), []byte(expected))
	}
	action := value("action")
	if action == "api" {
		if equal(value("user"), s.cfg.MediaAdminUser) && equal(value("password"), s.cfg.MediaAdminPassword) {
			c.Status(204)
		} else {
			deny()
		}
		return
	}
	path := value("path")
	if path == "" {
		deny()
		return
	}
	if action == "publish" {
		if !strings.HasPrefix(path, "demo/aerosight/") {
			deny()
			return
		}
		row, err := s.queries.ReadMediaPublishCredential(c.Request.Context(), sql.NullString{String: path, Valid: true})
		if err != nil || !row.CredentialEnvelopeJson.Valid {
			deny()
			return
		}
		var envelope credentials.Envelope
		if json.Unmarshal(row.CredentialEnvelopeJson.RawMessage, &envelope) != nil {
			deny()
			return
		}
		var secret struct {
			User     string `json:"mediaPublishUser"`
			Password string `json:"mediaPublishPassword"`
		}
		if credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("device-adapter", row.AdapterID, row.ProjectID), &secret) != nil || !equal(value("user"), secret.User) || !equal(value("password"), secret.Password) {
			deny()
			return
		}
		c.Status(204)
		return
	}
	if action != "read" && action != "playback" {
		deny()
		return
	}
	protocol := value("protocol")
	if protocol != "webrtc" && protocol != "hls" {
		deny()
		return
	}
	token := value("token")
	if token == "" {
		query, _ := url.ParseQuery(value("query"))
		token = query.Get("token")
	}
	if _, valid := media.VerifyPlaybackToken(s.credentialSecret, token, path, protocol, time.Now()); !valid {
		deny()
		return
	}
	c.Status(204)
}
