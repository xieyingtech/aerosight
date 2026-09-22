package httpapi

import (
	"aerosight/server/internal/database"
	"context"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

func hasAITextModel(models []aiModel, id string) bool {
	for _, model := range models {
		if model.ID == id && model.Enabled && (model.Protocol == "responses" || model.Protocol == "openai-compatible" || model.Protocol == "anthropic-messages") {
			return true
		}
	}
	return false
}

// A provider edit cannot replace a concurrently selected global default.
// Removing its selected model or disabling the provider clears that default.
func preserveAIDefaults(ctx context.Context, w *database.WriteTx, id int64, input *aiProviderInput) error {
	var text, voice string
	var textDefault, voiceDefault bool
	if err := w.Tx.QueryRowContext(ctx, `SELECT model_id,realtime_model_id,is_default,is_realtime_default FROM ai_providers WHERE id=$1`, id).Scan(&text, &voice, &textDefault, &voiceDefault); err != nil {
		return err
	}
	input.Params.ModelID = ""
	input.Params.IsDefault = input.Params.Enabled && textDefault && hasAITextModel(input.Models, text)
	if input.Params.IsDefault {
		input.Params.ModelID = text
	}
	input.IsRealtimeDefault = input.Params.Enabled && voiceDefault && hasAIModel(input.Models, voice, "stepfun-realtime") && input.Params.BaseUrl.Valid
	input.Params.RealtimeProtocol, input.Params.RealtimeModelID = "disabled", ""
	if input.IsRealtimeDefault {
		input.Params.RealtimeProtocol, input.Params.RealtimeModelID = "stepfun", voice
	}
	return nil
}

func (s *Server) setAIDefault(c *gin.Context) {
	var input struct {
		Kind       string `json:"kind"`
		ProviderID string `json:"providerId"`
		ModelID    string `json:"modelId"`
	}
	if strictJSON(c, &input) != nil || (input.Kind != "text" && input.Kind != "realtime") {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_INPUT_INVALID"))
		return
	}
	var id int64
	var err error
	if input.ProviderID != "" {
		id, err = strconv.ParseInt(input.ProviderID, 10, 64)
		if err != nil || id <= 0 || input.ModelID == "" {
			s.aiProviderFailure(c, errors.New("AI_PROVIDER_INPUT_INVALID"))
			return
		}
	} else if input.ModelID != "" {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_INPUT_INVALID"))
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "ai_provider.default", ResourceType: "ai_provider", ResourceID: input.ProviderID, Input: input}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		if e := w.Queries.LockAIProviderRegistry(ctx); e != nil {
			return nil, e
		}
		if id != 0 {
			var raw []byte
			var enabled bool
			var base string
			if e := w.Tx.QueryRowContext(ctx, `SELECT models_json,enabled,coalesce(base_url,'') FROM ai_providers WHERE id=$1 FOR UPDATE`, id).Scan(&raw, &enabled, &base); e != nil {
				return nil, errors.New("AI_PROVIDER_NOT_FOUND")
			}
			var models []aiModel
			if json.Unmarshal(raw, &models) != nil || !enabled {
				return nil, errors.New("AI_PROVIDER_INPUT_INVALID")
			}
			valid := hasAITextModel(models, input.ModelID)
			if input.Kind == "realtime" {
				valid = base != "" && hasAIModel(models, input.ModelID, "stepfun-realtime")
			}
			if !valid {
				return nil, errors.New("AI_PROVIDER_INPUT_INVALID")
			}
		}
		if input.Kind == "text" {
			if e := w.Queries.ClearAIProviderDefault(ctx); e != nil {
				return nil, e
			}
			if id != 0 {
				if _, e := w.Tx.ExecContext(ctx, `UPDATE ai_providers SET model_id=$2,is_default=true,updated_by_user_id=$3,updated_at=now() WHERE id=$1`, id, input.ModelID, uid); e != nil {
					return nil, e
				}
			}
		} else {
			if e := w.Queries.ClearAIProviderRealtimeDefault(ctx); e != nil {
				return nil, e
			}
			if id != 0 {
				if _, e := w.Tx.ExecContext(ctx, `UPDATE ai_providers SET realtime_model_id=$2,realtime_protocol='stepfun',is_realtime_default=true,updated_by_user_id=$3,updated_at=now() WHERE id=$1`, id, input.ModelID, uid); e != nil {
					return nil, e
				}
			}
		}
		return gin.H{"ok": true}, nil
	})
	if err != nil {
		s.aiProviderFailure(c, err)
		return
	}
	c.JSON(200, result)
}
