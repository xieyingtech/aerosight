package flighthub

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"net/url"
	"sort"
	"strconv"
	"strings"
	"time"
)

const liveSupplierAdapterVersion = "v1"

type LivePlaybackDescription struct {
	Supplier        string    `json:"supplier"`
	Protocol        string    `json:"protocol"`
	CredentialKind  string    `json:"credentialKind"`
	AdapterVersion  string    `json:"adapterVersion"`
	ReferenceDigest string    `json:"referenceDigest"`
	ExpiresAt       time.Time `json:"expiresAt"`
}

// LivePlaybackSecret deliberately keeps the supplier credential unexported.
// Reveal is for the short-lived media adapter boundary only.
type LivePlaybackSecret struct {
	value string
}

func (secret LivePlaybackSecret) Reveal() string { return secret.value }

func (secret LivePlaybackSecret) String() string { return "[REDACTED]" }

func (secret LivePlaybackSecret) GoString() string { return "[REDACTED]" }

func (secret LivePlaybackSecret) MarshalJSON() ([]byte, error) {
	return []byte(`{"redacted":true}`), nil
}

type NormalizedLivePlayback struct {
	Description LivePlaybackDescription `json:"description"`
	Secret      LivePlaybackSecret      `json:"secret"`
}

type LiveSupplierAdapter interface {
	Supplier() string
	Normalize(*Client, LiveStreamAuthorization) (NormalizedLivePlayback, error)
}

type LiveSupplierRegistry struct {
	client   *Client
	adapters map[string]LiveSupplierAdapter
}

func NewLiveSupplierRegistry(client *Client, adapters ...LiveSupplierAdapter) (*LiveSupplierRegistry, error) {
	if client == nil || len(adapters) == 0 {
		return nil, &APIError{SafeCode: "live_supplier_registry_invalid"}
	}
	registry := &LiveSupplierRegistry{client: client, adapters: make(map[string]LiveSupplierAdapter, len(adapters))}
	for _, adapter := range adapters {
		if adapter == nil {
			return nil, &APIError{SafeCode: "live_supplier_registry_invalid"}
		}
		supplier := strings.ToLower(strings.TrimSpace(adapter.Supplier()))
		if supplier == "" || !validFlightOperationCode(supplier) {
			return nil, &APIError{SafeCode: "live_supplier_registry_invalid"}
		}
		if _, duplicate := registry.adapters[supplier]; duplicate {
			return nil, &APIError{SafeCode: "live_supplier_registry_invalid"}
		}
		registry.adapters[supplier] = adapter
	}
	return registry, nil
}

func NewDefaultLiveSupplierRegistry(client *Client) (*LiveSupplierRegistry, error) {
	return NewLiveSupplierRegistry(client, volcLiveSupplierAdapter{}, agoraLiveSupplierAdapter{}, srsLiveSupplierAdapter{})
}

func (registry *LiveSupplierRegistry) Suppliers() []string {
	if registry == nil {
		return nil
	}
	result := make([]string, 0, len(registry.adapters))
	for supplier := range registry.adapters {
		result = append(result, supplier)
	}
	sort.Strings(result)
	return result
}

func (registry *LiveSupplierRegistry) Normalize(input LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	if registry == nil || registry.client == nil {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_registry_invalid"}
	}
	supplier := strings.ToLower(strings.TrimSpace(input.URLType))
	adapter, ok := registry.adapters[supplier]
	if !ok {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_unsupported"}
	}
	normalized, err := adapter.Normalize(registry.client, input)
	if err != nil {
		return NormalizedLivePlayback{}, err
	}
	if !validNormalizedLivePlayback(normalized, input, supplier) {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	return normalized, nil
}

type volcLiveSupplierAdapter struct{}

func (volcLiveSupplierAdapter) Supplier() string { return "volc" }

func (volcLiveSupplierAdapter) Normalize(client *Client, input LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	fields, err := parseLiveSupplierCredential(input.URL)
	if err != nil || !requireSupplierFields(fields, "app_id", "room_id", "token", "user_id", "expire_time") {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	if !supplierExpiryMatches(fields["expire_time"], input.ExpireTimestamp) {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	return normalizedLivePlayback(client, input, "volc", "volc-rtc", "sdk-query")
}

type agoraLiveSupplierAdapter struct{}

func (agoraLiveSupplierAdapter) Supplier() string { return "agora" }

func (agoraLiveSupplierAdapter) Normalize(client *Client, input LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	fields, err := parseLiveSupplierCredential(input.URL)
	if err != nil || !requireSupplierFields(fields, "app_id", "token") ||
		firstSupplierField(fields, "channel_name", "channel", "room_id") == "" ||
		firstSupplierField(fields, "uid", "user_id") == "" {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	if expiry := firstSupplierField(fields, "expire_time", "expire_ts", "privilege_expired_ts"); expiry != "" && !supplierExpiryMatches(expiry, input.ExpireTimestamp) {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	return normalizedLivePlayback(client, input, "agora", "agora-rtc", "sdk-credential")
}

type srsLiveSupplierAdapter struct{}

func (srsLiveSupplierAdapter) Supplier() string { return "srs" }

func (srsLiveSupplierAdapter) Normalize(client *Client, input LiveStreamAuthorization) (NormalizedLivePlayback, error) {
	parsed, err := client.ValidateTemporaryLink(LinkLive, input.URL, input.ExpiresAt)
	if err != nil {
		return NormalizedLivePlayback{}, err
	}
	protocol := srsPlaybackProtocol(parsed)
	return normalizedLivePlayback(client, input, "srs", protocol, "signed-url")
}

func parseLiveSupplierCredential(raw string) (map[string]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" || len(raw) > maxLiveCredentialBytes || strings.ContainsAny(raw, "\x00\r\n") {
		return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	if strings.HasPrefix(raw, "{") {
		decoder := json.NewDecoder(strings.NewReader(raw))
		decoder.UseNumber()
		var object map[string]any
		if decoder.Decode(&object) != nil || decoder.Decode(&struct{}{}) != io.EOF || object == nil || len(object) == 0 || len(object) > 32 {
			return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
		}
		result := make(map[string]string, len(object))
		for key, value := range object {
			if !validLiveOpaque(key, 64, false) {
				return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
			}
			switch typed := value.(type) {
			case string:
				result[key] = typed
			case json.Number:
				result[key] = typed.String()
			default:
				return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
			}
		}
		if !validSupplierFields(result) {
			return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
		}
		return result, nil
	}
	values, err := url.ParseQuery(raw)
	if err != nil || len(values) == 0 || len(values) > 32 {
		return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	result := make(map[string]string, len(values))
	for key, entries := range values {
		if len(entries) != 1 || !validLiveOpaque(key, 64, false) {
			return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
		}
		result[key] = entries[0]
	}
	if !validSupplierFields(result) {
		return nil, &APIError{SafeCode: "live_supplier_schema_incompatible"}
	}
	return result, nil
}

func validSupplierFields(fields map[string]string) bool {
	for _, value := range fields {
		if !validLiveOpaque(value, maxLiveStringBytes, false) {
			return false
		}
	}
	return true
}

func requireSupplierFields(fields map[string]string, names ...string) bool {
	for _, name := range names {
		if fields[name] == "" {
			return false
		}
	}
	return true
}

func firstSupplierField(fields map[string]string, names ...string) string {
	for _, name := range names {
		if value := fields[name]; value != "" {
			return value
		}
	}
	return ""
}

func supplierExpiryMatches(raw string, expected int64) bool {
	parsed, err := strconv.ParseInt(raw, 10, 64)
	return err == nil && parsed > 0 && parsed == expected
}

func normalizedLivePlayback(client *Client, input LiveStreamAuthorization, supplier, protocol, credentialKind string) (NormalizedLivePlayback, error) {
	if client == nil || input.ExpireTimestamp <= 0 || input.ExpiresAt.IsZero() || input.URL == "" ||
		!input.ExpiresAt.Equal(time.Unix(input.ExpireTimestamp, 0).UTC()) || !input.ExpiresAt.After(client.now()) ||
		input.ExpiresAt.After(client.now().Add(maxLiveCredentialTTL)) {
		return NormalizedLivePlayback{}, &APIError{SafeCode: "temporary_link_expired"}
	}
	digest := sha256.Sum256([]byte(input.URL))
	return NormalizedLivePlayback{
		Description: LivePlaybackDescription{
			Supplier: supplier, Protocol: protocol, CredentialKind: credentialKind,
			AdapterVersion: liveSupplierAdapterVersion, ReferenceDigest: hex.EncodeToString(digest[:]), ExpiresAt: input.ExpiresAt,
		},
		Secret: LivePlaybackSecret{value: input.URL},
	}, nil
}

func validNormalizedLivePlayback(normalized NormalizedLivePlayback, input LiveStreamAuthorization, supplier string) bool {
	description := normalized.Description
	digest, err := hex.DecodeString(description.ReferenceDigest)
	return err == nil && len(digest) == sha256.Size && description.Supplier == supplier &&
		validFlightOperationCode(description.Protocol) && validFlightOperationCode(description.CredentialKind) &&
		description.AdapterVersion == liveSupplierAdapterVersion && description.ExpiresAt.Equal(input.ExpiresAt) &&
		normalized.Secret.value != "" && normalized.Secret.value == input.URL
}

func srsPlaybackProtocol(parsed *url.URL) string {
	if parsed == nil {
		return "srs-https"
	}
	if strings.EqualFold(parsed.Query().Get("schema"), "webrtc") {
		return "webrtc"
	}
	path := strings.ToLower(parsed.Path)
	switch {
	case strings.HasSuffix(path, ".m3u8"):
		return "hls"
	case strings.HasSuffix(path, ".flv"):
		return "http-flv"
	default:
		return "srs-https"
	}
}
