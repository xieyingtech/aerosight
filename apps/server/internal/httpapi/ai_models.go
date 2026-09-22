package httpapi

import (
	"aerosight/server/internal/credentials"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/openai/openai-go/v3"
	"github.com/openai/openai-go/v3/option"
)

type aiModel struct {
	ID           string   `json:"id"`
	Protocol     string   `json:"protocol"`
	Capabilities []string `json:"capabilities"`
	Enabled      bool     `json:"enabled"`
}

// Provider endpoints are trusted platform-admin configuration, including LAN HTTP.
// Keep DNS pinning and prohibit credentials/query/fragment in a base URL.
func (s *Server) resolveAIURL(ctx context.Context, raw string) (*url.URL, []netip.Addr, error) {
	target, err := url.Parse(raw)
	if err != nil || target.Hostname() == "" || (target.Scheme != "http" && target.Scheme != "https") || target.User != nil || target.Fragment != "" {
		return nil, nil, errors.New("OUTBOUND_URL_INVALID")
	}
	var addresses []netip.Addr
	if ip, e := netip.ParseAddr(target.Hostname()); e == nil {
		addresses = []netip.Addr{ip}
	} else if s.networkResolver != nil {
		addresses, err = s.networkResolver(ctx, target.Hostname())
	} else {
		addresses, err = net.DefaultResolver.LookupNetIP(ctx, "ip", target.Hostname())
	}
	if err != nil {
		return nil, nil, errors.New("OUTBOUND_DNS_FAILED")
	}
	if len(addresses) == 0 {
		return nil, nil, errors.New("OUTBOUND_DNS_EMPTY")
	}
	return target, addresses, nil
}

func parseAIModels(value any) ([]aiModel, error) {
	bad := errors.New("AI_PROVIDER_INPUT_INVALID")
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, bad
	}
	var models []aiModel
	decoder := json.NewDecoder(strings.NewReader(string(raw)))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&models) != nil || models == nil || len(models) > 500 {
		return nil, bad
	}
	seen := map[string]bool{}
	for i := range models {
		m := &models[i]
		m.ID = strings.TrimSpace(m.ID)
		if utf16Length(m.ID) < 1 || utf16Length(m.ID) > 255 || seen[m.ID] {
			return nil, bad
		}
		seen[m.ID] = true
		switch m.Protocol {
		case "openai-compatible", "responses", "anthropic-messages", "stepfun-realtime":
		default:
			return nil, bad
		}
		// Capability declarations are no longer user configuration.
		m.Capabilities = []string{"text", "vision", "audio-input", "audio-output", "realtime", "embedding", "image-generation", "tools"}
		m.Enabled = true
	}
	return models, nil
}

func hasAIModel(models []aiModel, id, protocol string) bool {
	for _, m := range models {
		if m.ID == id && m.Protocol == protocol && m.Enabled {
			return true
		}
	}
	return false
}

// Discovery accepts unsaved connection settings, and can reuse an existing secret.
// An existing secret is never sent to a changed endpoint unless supplied afresh.
func (s *Server) discoverAIModels(c *gin.Context) {
	var input struct {
		ProviderID string `json:"providerId"`
		BaseURL    string `json:"baseUrl"`
		APIKey     string `json:"apiKey"`
	}
	if strictJSON(c, &input) != nil || len(input.APIKey) > 16384 {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_INPUT_INVALID"))
		return
	}
	base := strings.TrimRight(strings.TrimSpace(input.BaseURL), "/")
	if base == "" {
		base = "https://api.openai.com/v1"
	}
	key := strings.TrimSpace(input.APIKey)
	if input.ProviderID != "" && key == "" {
		id, err := strconv.ParseInt(input.ProviderID, 10, 64)
		if err != nil || id <= 0 {
			s.aiProviderFailure(c, errors.New("AI_PROVIDER_NOT_FOUND"))
			return
		}
		p, err := s.queries.LockAIProvider(c.Request.Context(), id)
		if err != nil {
			s.aiProviderFailure(c, errors.New("AI_PROVIDER_NOT_FOUND"))
			return
		}
		savedBase := strings.TrimRight(p.BaseUrl.String, "/")
		if savedBase == "" {
			savedBase = "https://api.openai.com/v1"
		}
		if base != savedBase {
			s.aiProviderFailure(c, errors.New("AI_PROVIDER_ENDPOINT_KEY_REQUIRED"))
			return
		}
		var envelope credentials.Envelope
		var secret struct {
			APIKey string `json:"apiKey"`
		}
		if json.Unmarshal(p.CredentialEnvelopeJson, &envelope) != nil || credentials.DecryptJSON(envelope, s.credentialSecret, credentials.AAD("ai-provider", id, nil), &secret) != nil {
			s.aiProviderFailure(c, errors.New("AI_PROVIDER_CREDENTIAL_UNAVAILABLE"))
			return
		}
		key = secret.APIKey
	}
	ctx, cancel := context.WithTimeout(c.Request.Context(), 15*time.Second)
	defer cancel()
	target, addresses, err := s.resolveAIURL(ctx, base)
	if err != nil || target.RawQuery != "" {
		if err == nil {
			err = errors.New("OUTBOUND_URL_INVALID")
		}
		s.aiProviderFailure(c, err)
		return
	}
	factory := s.aiHTTPClientFactory
	if factory == nil {
		factory = pinnedAIHTTPClient
	}
	client := factory(target, addresses)
	defer client.CloseIdleConnections()
	sdk := openai.NewClient(option.WithAPIKey(key), option.WithBaseURL(base+"/"), option.WithHTTPClient(aiChatClient{client}), option.WithMaxRetries(0))
	page, err := sdk.Models.List(ctx)
	if err != nil {
		s.aiProviderFailure(c, errors.New("AI_PROVIDER_MODELS_FAILED"))
		return
	}
	ids := []string{}
	for _, m := range page.Data {
		if strings.TrimSpace(m.ID) != "" && utf16Length(m.ID) <= 255 && !slices.Contains(ids, m.ID) {
			ids = append(ids, m.ID)
		}
		if len(ids) == 500 {
			break
		}
	}
	slices.Sort(ids)
	c.JSON(200, gin.H{"models": ids})
}
