package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/url"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type aiProviderInput struct {
	Params sqlcgen.CreateAIProviderParams
	APIKey string
	Audit  map[string]any
}

func parseAIProvider(raw map[string]any) (aiProviderInput, error) {
	out := aiProviderInput{Audit: map[string]any{}}
	bad := errors.New("AI_PROVIDER_INPUT_INVALID")
	for k, v := range raw {
		switch k {
		case "name", "providerType", "baseUrl", "modelId", "enabled", "isDefault":
			out.Audit[k] = v
		case "apiKey":
		default:
			return out, bad
		}
	}
	for _, f := range []struct {
		key  string
		max  int
		dest *string
	}{{"name", 120, &out.Params.Name}, {"modelId", 255, &out.Params.ModelID}} {
		value, ok := raw[f.key].(string)
		value = strings.TrimSpace(value)
		if !ok || utf16Length(value) < 1 || utf16Length(value) > f.max {
			return out, bad
		}
		*f.dest = value
		out.Audit[f.key] = value
	}
	if raw["providerType"] != "openai" {
		return out, bad
	}
	if v, present := raw["baseUrl"]; present {
		text, ok := v.(string)
		if !ok {
			return out, bad
		}
		if text != "" {
			parsed, err := url.Parse(text)
			if err != nil || parsed.Scheme == "" || parsed.Hostname() == "" {
				return out, bad
			}
			out.Params.BaseUrl = sql.NullString{String: text, Valid: true}
		}
	}
	if v, present := raw["apiKey"]; present {
		text, ok := v.(string)
		if !ok || utf16Length(text) > 16384 {
			return out, bad
		}
		out.APIKey = strings.TrimSpace(text)
	}
	for _, f := range []struct {
		key  string
		dest *bool
	}{{"enabled", &out.Params.Enabled}, {"isDefault", &out.Params.IsDefault}} {
		if v, present := raw[f.key]; present {
			value, ok := v.(bool)
			if !ok {
				return out, bad
			}
			*f.dest = value
		}
		out.Audit[f.key] = *f.dest
	}
	if out.Params.IsDefault && !out.Params.Enabled {
		return out, bad
	}
	return out, nil
}

func (s *Server) authorizePlatformWrite(uid int32) database.Reauthorize {
	return func(ctx context.Context, w *database.WriteTx) error {
		role, err := w.Queries.LockPlatformUserRole(ctx, uid)
		if errors.Is(err, sql.ErrNoRows) || (err == nil && role != "admin") {
			return errors.New("FORBIDDEN")
		}
		return err
	}
}

func (s *Server) aiProviderFailure(c *gin.Context, err error) {
	code, status := "AI_PROVIDER_FAILED", 400
	switch err.Error() {
	case "FORBIDDEN":
		code, status = "FORBIDDEN", 403
	case "AI_PROVIDER_INPUT_INVALID", "AI_PROVIDER_CREDENTIAL_REQUIRED", "AI_PROVIDER_NOT_FOUND", "OUTBOUND_URL_INVALID", "OUTBOUND_HTTPS_REQUIRED", "OUTBOUND_URL_CREDENTIALS_FORBIDDEN", "OUTBOUND_DNS_EMPTY", "OUTBOUND_DNS_FAILED", "OUTBOUND_ADDRESS_RESTRICTED":
		code = err.Error()
	}
	s.failure(c, status, code)
}

func (s *Server) aiProviderRoutes() {
	g := s.router.Group("/api/admin/ai-providers", s.requireUser, s.requireAdmin, s.timeout)
	g.GET("", func(c *gin.Context) {
		raw, err := s.queries.ListAIProviders(c.Request.Context())
		if err != nil {
			s.aiProviderFailure(c, err)
			return
		}
		rows, err := decodeSnapshotRows(raw)
		if err != nil {
			s.aiProviderFailure(c, err)
			return
		}
		c.JSON(200, rows)
	})
	g.POST("", s.saveAIProvider)
	g.PATCH("/:providerId", s.saveAIProvider)
	g.DELETE("/:providerId", s.deleteAIProvider)
}

func (s *Server) saveAIProvider(c *gin.Context) {
	creating := c.Request.Method == "POST"
	var id int64
	var err error
	if !creating {
		id, err = strconv.ParseInt(c.Param("providerId"), 10, 64)
		if err != nil || id <= 0 {
			s.aiProviderFailure(c, errors.New("AI_PROVIDER_NOT_FOUND"))
			return
		}
	}
	var raw map[string]any
	if strictJSON(c, &raw) != nil {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_INPUT_INVALID"))
		return
	}
	input, err := parseAIProvider(raw)
	if err != nil {
		s.aiProviderFailure(c, err)
		return
	}
	if creating && input.APIKey == "" {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_CREDENTIAL_REQUIRED"))
		return
	}
	ctx := c.Request.Context()
	if input.Params.BaseUrl.Valid {
		target, _ := url.Parse(input.Params.BaseUrl.String)
		if _, _, err = s.validateOutboundURL(ctx, input.Params.BaseUrl.String, []string{target.Hostname()}); err != nil {
			s.aiProviderFailure(c, err)
			return
		}
	}
	uid := currentUser(c).ID
	action, resource := "ai_provider.create", ""
	if !creating {
		action, resource = "ai_provider.update", strconv.FormatInt(id, 10)
	}
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: action, ResourceType: "ai_provider", ResourceID: resource, Input: input.Audit}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		// Serialize registry writes before row locks, including the first default creation.
		if e := w.Queries.LockAIProviderRegistry(ctx); e != nil {
			return nil, e
		}
		if !creating {
			if _, e := w.Queries.LockAIProvider(ctx, id); e != nil {
				if errors.Is(e, sql.ErrNoRows) {
					e = errors.New("AI_PROVIDER_NOT_FOUND")
				}
				return nil, e
			}
		}
		if input.Params.IsDefault {
			if e := w.Queries.ClearAIProviderDefault(ctx); e != nil {
				return nil, e
			}
		}
		p := input.Params
		p.CreatedByUserID = uid
		if creating {
			id, err = w.Queries.CreateAIProvider(ctx, p)
		} else {
			err = w.Queries.UpdateAIProvider(ctx, sqlcgen.UpdateAIProviderParams{ID: id, Name: p.Name, BaseUrl: p.BaseUrl, ModelID: p.ModelID, Enabled: p.Enabled, IsDefault: p.IsDefault, UpdatedByUserID: uid})
		}
		if err != nil {
			return nil, err
		}
		if input.APIKey != "" {
			envelope, e := credentials.EncryptJSON(map[string]string{"apiKey": input.APIKey}, s.credentialSecret, credentials.AAD("ai-provider", id, nil))
			if e != nil {
				return nil, e
			}
			encoded, e := json.Marshal(envelope)
			if e != nil {
				return nil, e
			}
			if e = w.Queries.SetAIProviderCredential(ctx, sqlcgen.SetAIProviderCredentialParams{ID: id, CredentialEnvelopeJson: encoded}); e != nil {
				return nil, e
			}
		}
		row, e := w.Queries.ReadAIProviderPublic(ctx, id)
		if e != nil {
			return nil, e
		}
		rows, e := decodeSnapshotRows([]json.RawMessage{row})
		if e != nil {
			return nil, e
		}
		return rows[0], nil
	})
	if err != nil {
		s.aiProviderFailure(c, err)
		return
	}
	status := 200
	if creating {
		status = 201
	}
	c.JSON(status, result)
}

func (s *Server) deleteAIProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("providerId"), 10, 64)
	if err != nil || id <= 0 {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_NOT_FOUND"))
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "ai_provider.delete", ResourceType: "ai_provider", ResourceID: strconv.FormatInt(id, 10), Input: gin.H{"providerId": id}}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		if e := w.Queries.LockAIProviderRegistry(ctx); e != nil {
			return nil, e
		}
		count, e := w.Queries.DeleteAIProvider(ctx, id)
		if e != nil {
			return nil, e
		}
		if count != 1 {
			return nil, errors.New("AI_PROVIDER_NOT_FOUND")
		}
		return gin.H{"id": id, "deleted": true}, nil
	})
	if err != nil {
		s.aiProviderFailure(c, err)
		return
	}
	c.JSON(200, result)
}
