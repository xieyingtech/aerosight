package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sort"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

const LiveControlAcceptanceEndpoint = "456809558e0"

type LiveControlAcceptanceGuard func(context.Context) error

type LiveControlAcceptanceRepository interface {
	SaveCapabilitySnapshot(context.Context, connector.Instance, connector.CapabilitySnapshot) error
	SaveCapabilityAccountFingerprint(context.Context, connector.Instance, string) error
}

type LiveControlAcceptanceResult struct {
	Endpoint   string   `json:"endpoint"`
	Category   string   `json:"category"`
	Fields     []string `json:"fields"`
	Supplier   string   `json:"supplier,omitempty"`
	Protocol   string   `json:"protocol,omitempty"`
	DurationMS int64    `json:"durationMs"`
}

func RunLiveControlAcceptance(
	ctx context.Context,
	client FlightHubLiveStartClient,
	normalizer FlightHubLiveNormalizer,
	token, projectUUID, serial, cameraIndex string,
	guard LiveControlAcceptanceGuard,
) (result LiveControlAcceptanceResult) {
	started := time.Now()
	result = LiveControlAcceptanceResult{Endpoint: LiveControlAcceptanceEndpoint, Fields: []string{}}
	defer func() { result.DurationMS = time.Since(started).Milliseconds() }()
	if client == nil || normalizer == nil || guard == nil {
		result.Category = "acceptance_guard_failed"
		return
	}
	if err := guard(ctx); err != nil {
		result.Category = safeLiveAcceptanceCategory(err)
		return
	}
	authorization, err := client.StartLiveStream(ctx, token, projectUUID, LiveStreamStartRequest{
		SN: serial, CameraIndex: cameraIndex, VideoExpire: 3600, QualityType: LiveQualityAdaptive,
	})
	if err != nil {
		result.Category = safeLiveAcceptanceCategory(err)
		return
	}
	playback, err := normalizer.Normalize(authorization)
	authorization.URL = ""
	if err != nil {
		result.Category = safeLiveAcceptanceCategory(err)
		return
	}
	result.Category = "succeeded"
	result.Fields = []string{"expire_ts", "url", "url_type"}
	result.Supplier = playback.Description.Supplier
	result.Protocol = playback.Description.Protocol
	return
}

func PersistLiveControlAcceptanceEvidence(
	ctx context.Context,
	repository LiveControlAcceptanceRepository,
	instance connector.Instance,
	result LiveControlAcceptanceResult,
	accountFingerprint, deviceModel, firmwareVersion, cameraIndex, acceptanceRunID string,
	verifiedAt time.Time,
	ttl time.Duration,
) error {
	accountFingerprint = strings.TrimSpace(accountFingerprint)
	deviceModel = strings.TrimSpace(deviceModel)
	firmwareVersion = strings.TrimSpace(firmwareVersion)
	cameraIndex = strings.TrimSpace(cameraIndex)
	acceptanceRunID = strings.TrimSpace(acceptanceRunID)
	if repository == nil || instance.ID <= 0 || instance.ProjectID <= 0 ||
		!validAccountFingerprint(accountFingerprint) || deviceModel == "" ||
		cameraIndex == "" || acceptanceRunID == "" || len(acceptanceRunID) > 128 || verifiedAt.IsZero() || ttl <= 0 ||
		result.Endpoint != LiveControlAcceptanceEndpoint || result.Category != "succeeded" ||
		strings.TrimSpace(result.Supplier) == "" || strings.TrimSpace(result.Protocol) == "" || result.DurationMS < 0 {
		return &APIError{SafeCode: "acceptance_incomplete"}
	}
	fields := append([]string(nil), result.Fields...)
	sort.Strings(fields)
	if len(fields) != 3 || fields[0] != "expire_ts" || fields[1] != "url" || fields[2] != "url_type" {
		return &APIError{SafeCode: "acceptance_incomplete"}
	}
	if err := repository.SaveCapabilityAccountFingerprint(ctx, instance, accountFingerprint); err != nil {
		return err
	}
	digest := sha256.Sum256([]byte(cameraIndex))
	expiresAt := verifiedAt.Add(ttl)
	return repository.SaveCapabilitySnapshot(ctx, instance, connector.CapabilitySnapshot{
		CapabilityCode: "live.control", Status: "supported", EvidenceLevel: "field-write",
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: accountFingerprint,
		DeviceModel: deviceModel, FirmwareVersion: firmwareVersion,
		Details: map[string]any{
			"source": "dock-live-control-acceptance", "reason": "live_start_and_supplier_normalization_succeeded",
			"remoteMutation": true, "endpointId": result.Endpoint, "fields": fields,
			"supplier": result.Supplier, "protocol": result.Protocol, "durationMs": result.DurationMS,
			"cameraIndexDigest": hex.EncodeToString(digest[:]), "acceptanceRunId": acceptanceRunID,
		},
		VerifiedAt: verifiedAt, ExpiresAt: &expiresAt,
	})
}

func safeLiveAcceptanceCategory(err error) string {
	code := SafeCode(err)
	switch code {
	case "request_invalid", "credential_invalid", "scope_forbidden", "capability_not_supported",
		"configuration_required", "live_supplier_unsupported", "live_supplier_schema_incompatible",
		"temporary_link_expired", "acceptance_guard_failed", "connector_changed":
		return code
	default:
		return "remote_response_unknown"
	}
}
