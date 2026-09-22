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
	"time"

	"github.com/gin-gonic/gin"
)

type aiProviderInput struct {
	Params            sqlcgen.CreateAIProviderParams
	APIKey            string
	Models            []aiModel
	IsRealtimeDefault bool
	Audit             map[string]any
}

func parseAIProvider(raw map[string]any) (aiProviderInput, error) {
	out := aiProviderInput{Audit: map[string]any{}}
	bad := errors.New("AI_PROVIDER_INPUT_INVALID")
	for k, v := range raw {
		switch k {
		case "name", "providerType", "baseUrl", "modelId", "enabled", "isDefault", "realtimeProtocol", "realtimeModelId", "models", "isRealtimeDefault":
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
		if f.key == "modelId" && raw["models"] != nil && ((raw[f.key] == nil) || (ok && strings.TrimSpace(value) == "")) {
			continue
		}
		value = strings.TrimSpace(value)
		if !ok || utf16Length(value) < 1 || utf16Length(value) > f.max {
			return out, bad
		}
		*f.dest = value
		out.Audit[f.key] = value
	}
	out.Params.RealtimeProtocol = "disabled"
	if value, present := raw["realtimeProtocol"]; present {
		protocol, ok := value.(string)
		if !ok || (protocol != "disabled" && protocol != "stepfun") {
			return out, bad
		}
		out.Params.RealtimeProtocol = protocol
	}
	if value, present := raw["realtimeModelId"]; present {
		model, ok := value.(string)
		if !ok {
			return out, bad
		}
		out.Params.RealtimeModelID = strings.TrimSpace(model)
	}
	if out.Params.RealtimeProtocol == "disabled" {
		if out.Params.RealtimeModelID != "" {
			return out, bad
		}
	} else if utf16Length(out.Params.RealtimeModelID) < 1 || utf16Length(out.Params.RealtimeModelID) > 255 {
		return out, bad
	}
	out.Audit["realtimeProtocol"] = out.Params.RealtimeProtocol
	out.Audit["realtimeModelId"] = out.Params.RealtimeModelID
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
			if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Hostname() == "" || parsed.User != nil || parsed.RawQuery != "" || parsed.Fragment != "" {
				return out, bad
			}
			out.Params.BaseUrl = sql.NullString{String: text, Valid: true}
		}
	}
	if out.Params.RealtimeProtocol != "disabled" && !out.Params.BaseUrl.Valid {
		return out, bad
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
	if value, present := raw["models"]; present {
		var err error
		out.Models, err = parseAIModels(value)
		if err != nil {
			return out, err
		}
		if out.Params.IsDefault && !hasAITextModel(out.Models, out.Params.ModelID) {
			return out, bad
		}
		if out.Params.RealtimeProtocol != "disabled" && !hasAIModel(out.Models, out.Params.RealtimeModelID, "stepfun-realtime") {
			return out, bad
		}
	} else {
		out.Models = []aiModel{{ID: out.Params.ModelID, Protocol: "responses", Capabilities: []string{"text"}, Enabled: true}}
		if out.Params.RealtimeProtocol != "disabled" {
			out.Models = append(out.Models, aiModel{ID: out.Params.RealtimeModelID, Protocol: "stepfun-realtime", Capabilities: []string{"realtime", "audio-input", "audio-output"}, Enabled: true})
		}
	}
	out.IsRealtimeDefault = out.Params.IsDefault && out.Params.RealtimeProtocol != "disabled"
	if value, present := raw["isRealtimeDefault"]; present {
		var ok bool
		out.IsRealtimeDefault, ok = value.(bool)
		if !ok {
			return out, bad
		}
	}
	if out.IsRealtimeDefault && (!out.Params.Enabled || out.Params.RealtimeProtocol == "disabled") {
		return out, bad
	}
	out.Audit["models"] = out.Models
	out.Audit["isRealtimeDefault"] = out.IsRealtimeDefault
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
	case "AI_PROVIDER_MODELS_FAILED", "AI_PROVIDER_ENDPOINT_KEY_REQUIRED", "AI_PROVIDER_CREDENTIAL_UNAVAILABLE", "AI_PROVIDER_INPUT_INVALID", "AI_PROVIDER_API_KEY_REQUIRED", "AI_PROVIDER_NOT_FOUND", "OUTBOUND_URL_INVALID", "OUTBOUND_HTTPS_REQUIRED", "OUTBOUND_URL_CREDENTIALS_FORBIDDEN", "OUTBOUND_DNS_EMPTY", "OUTBOUND_DNS_FAILED", "OUTBOUND_ADDRESS_RESTRICTED":
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
	g.POST("/models", s.discoverAIModels)
	g.PUT("/defaults", s.setAIDefault)
	g.PATCH("/:providerId", s.saveAIProvider)
	g.DELETE("/:providerId", s.deleteAIProvider)
	g.POST("/:providerId/test", s.testAIProvider)
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

	ctx := c.Request.Context()
	if input.Params.BaseUrl.Valid {
		if _, _, err = s.resolveAIURL(ctx, input.Params.BaseUrl.String); err != nil {
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
		if !creating && raw["isDefault"] == nil && raw["isRealtimeDefault"] == nil {
			if e := preserveAIDefaults(ctx, w, id, &input); e != nil {
				return nil, e
			}
		}
		if input.IsRealtimeDefault {
			if e := w.Queries.ClearAIProviderRealtimeDefault(ctx); e != nil {
				return nil, e
			}
		}
		// Clear before updating enabled/protocol to satisfy the database constraint.
		if !creating {
			if e := w.Queries.SetAIProviderModels(ctx, sqlcgen.SetAIProviderModelsParams{ID: id, ModelsJson: []byte("[]"), IsRealtimeDefault: false}); e != nil {
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
			err = w.Queries.UpdateAIProvider(ctx, sqlcgen.UpdateAIProviderParams{ID: id, Name: p.Name, BaseUrl: p.BaseUrl, ModelID: p.ModelID, RealtimeProtocol: p.RealtimeProtocol, RealtimeModelID: p.RealtimeModelID, Enabled: p.Enabled, IsDefault: p.IsDefault, UpdatedByUserID: uid})
		}
		if err != nil {
			return nil, err
		}
		modelsJSON, _ := json.Marshal(input.Models)
		if e := w.Queries.SetAIProviderModels(ctx, sqlcgen.SetAIProviderModelsParams{ID: id, ModelsJson: modelsJSON, IsRealtimeDefault: input.IsRealtimeDefault}); e != nil {
			return nil, e
		}
		if creating || input.APIKey != "" {
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

func (s *Server) testAIProvider(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("providerId"), 10, 64)
	if err != nil || id <= 0 {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_NOT_FOUND"))
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "ai_provider.test", ResourceType: "ai_provider", ResourceID: strconv.FormatInt(id, 10), Input: gin.H{"providerId": id}}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		p, e := w.Queries.LockAIProvider(ctx, id)
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("AI_PROVIDER_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		var envelope credentials.Envelope
		if e = json.Unmarshal(p.CredentialEnvelopeJson, &envelope); e != nil {
			return nil, e
		}
		var credential struct {
			APIKey string `json:"apiKey"`
		}
		if e = credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("ai-provider", id, nil), &credential); e != nil {
			return nil, e
		}

		baseURL := p.BaseUrl.String
		if baseURL == "" {
			baseURL = "https://api.openai.com/v1"
		}
		target, e := url.Parse(baseURL)
		if e != nil {
			return nil, errors.New("OUTBOUND_URL_INVALID")
		}
		target, addresses, e := s.resolveAIURL(ctx, baseURL)
		if e != nil {
			return nil, e
		}
		factory := s.aiHTTPClientFactory
		if factory == nil {
			factory = pinnedAIHTTPClient
		}
		client := factory(target, addresses)
		defer client.CloseIdleConnections()
		ok, code := probeAIModels(ctx, client, baseURL, credential.APIKey)
		health := gin.H{"ok": ok, "code": code, "checkedAt": timestamp(time.Now())}
		encoded, e := json.Marshal(health)
		if e != nil {
			return nil, e
		}
		status := "failed"
		if ok {
			status = "healthy"
		}
		e = w.Queries.SetAIProviderHealth(ctx, sqlcgen.SetAIProviderHealthParams{ID: id, Status: status, HealthJson: encoded, UpdatedByUserID: uid})
		return health, e
	})
	if err != nil {
		s.aiProviderFailure(c, err)
		return
	}
	c.JSON(200, result)
}
