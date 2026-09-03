package flighthub

import (
	"context"
	"errors"
	"sort"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

const TemporaryCredentialCapability = "security.temporary-credential"

type TemporaryCredentialAcceptanceClient interface {
	CreateStorageSTS(context.Context, string, string, StorageSTSRequest) (StorageSTS, error)
	ObtainOpenModelUploadCredential(context.Context, string, string) (OpenModelUploadCredential, error)
}

type TemporaryCredentialAcceptanceGuard func(context.Context, string) error

type TemporaryCredentialAcceptanceResult struct {
	Endpoint   string   `json:"endpoint"`
	Category   string   `json:"category"`
	Count      int      `json:"count"`
	Fields     []string `json:"fields"`
	DurationMS int64    `json:"durationMs"`
}

type TemporaryCredentialAcceptanceRepository interface {
	SaveCapabilitySnapshot(context.Context, connector.Instance, connector.CapabilitySnapshot) error
	SaveCapabilityAccountFingerprint(context.Context, connector.Instance, string) error
}

var temporaryCredentialAcceptanceEndpoints = []string{"454273351e0", "458069518e0"}

// RunTemporaryCredentialAcceptance performs only credential issuance. It never
// uploads an object, starts a reconstruction, creates a flight task, or invokes
// a device action. Returned credentials are not retained in the result or
// persistence payload.
func RunTemporaryCredentialAcceptance(
	ctx context.Context,
	client TemporaryCredentialAcceptanceClient,
	token string,
	projectUUID string,
	fileUUID string,
	guard TemporaryCredentialAcceptanceGuard,
) []TemporaryCredentialAcceptanceResult {
	if client == nil || guard == nil || strings.TrimSpace(token) == "" || strings.TrimSpace(projectUUID) == "" || strings.TrimSpace(fileUUID) == "" {
		return []TemporaryCredentialAcceptanceResult{{Endpoint: "command", Category: "request_invalid", Fields: []string{}}}
	}

	results := make([]TemporaryCredentialAcceptanceResult, 0, len(temporaryCredentialAcceptanceEndpoints))
	started := time.Now()
	err := guard(ctx, temporaryCredentialAcceptanceEndpoints[0])
	if err == nil {
		_, err = client.CreateStorageSTS(ctx, token, projectUUID, StorageSTSRequest{
			SpecifyPath: "aerosight/field-acceptance/probe.bin",
			FileUUID:    fileUUID,
		})
	}
	results = append(results, temporaryCredentialResult(
		temporaryCredentialAcceptanceEndpoints[0],
		[]string{"bucket", "credentials", "endpoint", "object_key_prefix", "provider", "region"},
		started,
		err,
	))
	started = time.Now()
	err = guard(ctx, temporaryCredentialAcceptanceEndpoints[1])
	if err == nil {
		_, err = client.ObtainOpenModelUploadCredential(ctx, token, projectUUID)
	}
	results = append(results, temporaryCredentialResult(
		temporaryCredentialAcceptanceEndpoints[1],
		[]string{"access_key_id", "callback_param", "cloud_bucket_name", "cloud_name", "end_point", "expire_time", "region", "secret_access_key", "session_token", "store_path"},
		started,
		err,
	))
	return results
}

func temporaryCredentialResult(endpoint string, fields []string, started time.Time, err error) TemporaryCredentialAcceptanceResult {
	result := TemporaryCredentialAcceptanceResult{Endpoint: endpoint, Category: "succeeded", Count: 1, Fields: append([]string(nil), fields...), DurationMS: elapsedMilliseconds(started)}
	if err == nil {
		sort.Strings(result.Fields)
		return result
	}
	result.Category = "upstream_error"
	result.Count = 0
	result.Fields = []string{}
	var apiError *APIError
	if errors.As(err, &apiError) && safeTemporaryCredentialCategory(apiError.SafeCode) {
		result.Category = apiError.SafeCode
	} else if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		result.Category = "cancelled"
	}
	return result
}

func safeTemporaryCredentialCategory(value string) bool {
	return safeSmokeCategory(value) || value == "connector_changed" || value == "acceptance_guard_failed" ||
		value == "temporary_link_invalid" || value == "temporary_link_expired" || value == "temporary_link_host_forbidden"
}

func PersistTemporaryCredentialAcceptanceEvidence(
	ctx context.Context,
	repository TemporaryCredentialAcceptanceRepository,
	instance connector.Instance,
	results []TemporaryCredentialAcceptanceResult,
	accountFingerprint string,
	verifiedAt time.Time,
	ttl time.Duration,
) error {
	if repository == nil || instance.ID <= 0 || instance.ProjectID <= 0 || !validAccountFingerprint(accountFingerprint) ||
		verifiedAt.IsZero() || ttl <= 0 || len(results) != len(temporaryCredentialAcceptanceEndpoints) {
		return &APIError{SafeCode: "request_invalid"}
	}
	safeResults := make([]map[string]any, 0, len(results))
	for index, result := range results {
		if result.Endpoint != temporaryCredentialAcceptanceEndpoints[index] || result.Category != "succeeded" || result.Count != 1 || len(result.Fields) == 0 || result.DurationMS < 0 {
			return &APIError{SafeCode: "acceptance_incomplete"}
		}
		fields := append([]string(nil), result.Fields...)
		for _, field := range fields {
			if strings.TrimSpace(field) == "" || len(field) > 128 {
				return &APIError{SafeCode: "request_invalid"}
			}
		}
		sort.Strings(fields)
		safeResults = append(safeResults, map[string]any{
			"endpointId": result.Endpoint,
			"category":   result.Category,
			"count":      result.Count,
			"fields":     fields,
			"durationMs": result.DurationMS,
		})
	}
	if err := repository.SaveCapabilityAccountFingerprint(ctx, instance, accountFingerprint); err != nil {
		return err
	}
	expiresAt := verifiedAt.Add(ttl)
	return repository.SaveCapabilitySnapshot(ctx, instance, connector.CapabilitySnapshot{
		CapabilityCode:     TemporaryCredentialCapability,
		Status:             "supported",
		EvidenceLevel:      "field-write",
		Region:             "cn",
		Deployment:         "cn-public-cloud",
		AccountFingerprint: accountFingerprint,
		Details: map[string]any{
			"source":         "temporary-credential-acceptance",
			"reason":         "credential_issuance_succeeded",
			"remoteMutation": false,
			"endpoints":      safeResults,
		},
		VerifiedAt: verifiedAt,
		ExpiresAt:  &expiresAt,
	})
}
