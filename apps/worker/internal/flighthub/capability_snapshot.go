package flighthub

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

type CapabilitySnapshotRepository interface {
	SaveCapabilitySnapshot(context.Context, connector.Instance, connector.CapabilitySnapshot) error
	SaveCapabilityAccountFingerprint(context.Context, connector.Instance, string) error
	ListCapabilitySnapshots(context.Context, connector.Instance, string, string) ([]connector.CapabilitySnapshot, error)
}

type CapabilitySnapshotEvidence struct {
	Level              string
	Region             string
	Deployment         string
	AccountFingerprint string
	DeviceModel        string
	FirmwareVersion    string
	VerifiedAt         time.Time
	ExpiresAt          *time.Time
}

type CapabilityEvaluationScope struct {
	Region             string
	Deployment         string
	AccountFingerprint string
	DeviceModel        string
	FirmwareVersion    string
	Now                time.Time
}

var deviceBoundFieldAcceptanceCapabilities = map[string]struct{}{
	"device.control":               {},
	"flight.execute":               {},
	"live.control":                 {},
	"live.quality.set":             {},
	"device.camera.change":         {},
	"device.lens.change":           {},
	"device.rtk.calibrate":         {},
	"device.relay.pair":            {},
	"device.active-project.update": {},
}

var accountBoundFieldAcceptanceCapabilities = map[string]struct{}{
	"security.temporary-credential":     {},
	"live.recording.control":            {},
	"live.share.manage":                 {},
	"live.converter.create":             {},
	"live.converter.toggle":             {},
	"live.converter.delete":             {},
	"geospatial.write":                  {},
	"geospatial.element.delete":         {},
	"model.write":                       {},
	"model.delete":                      {},
	"model.resource.delete":             {},
	"security.sn.decrypt":               {},
	"organization.project-member.write": {},
	"organization.write":                {},
}

func CapabilityAccountFingerprint(organizationUUID, userID string) (string, error) {
	organizationUUID = strings.ToLower(strings.TrimSpace(organizationUUID))
	userID = strings.TrimSpace(userID)
	if !uuidPattern.MatchString(organizationUUID) || userID == "" || len(userID) > 512 {
		return "", &APIError{SafeCode: "request_invalid"}
	}
	digest := sha256.Sum256([]byte("aerosight:dji-flighthub2:account:v1\n" + organizationUUID + "\n" + userID))
	return hex.EncodeToString(digest[:]), nil
}

func PersistCapabilityProbeResults(
	ctx context.Context,
	repository CapabilitySnapshotRepository,
	instance connector.Instance,
	results []CapabilityProbeResult,
	evidence CapabilitySnapshotEvidence,
) error {
	if repository == nil {
		return errors.New("DJI_FLIGHTHUB_CAPABILITY_REPOSITORY_UNAVAILABLE")
	}
	evidence.Region = strings.TrimSpace(evidence.Region)
	evidence.Deployment = strings.TrimSpace(evidence.Deployment)
	evidence.AccountFingerprint = strings.TrimSpace(evidence.AccountFingerprint)
	evidence.DeviceModel = strings.TrimSpace(evidence.DeviceModel)
	evidence.FirmwareVersion = strings.TrimSpace(evidence.FirmwareVersion)
	if !validEvidenceLevel(evidence.Level) || evidence.Region == "" || evidence.Deployment == "" || evidence.VerifiedAt.IsZero() ||
		(evidence.AccountFingerprint != "" && !validAccountFingerprint(evidence.AccountFingerprint)) ||
		(evidence.Level == "field-write" && !validAccountFingerprint(evidence.AccountFingerprint)) ||
		(evidence.ExpiresAt != nil && !evidence.ExpiresAt.After(evidence.VerifiedAt)) {
		return &APIError{SafeCode: "request_invalid"}
	}
	known := make(map[string]struct{}, len(Capabilities()))
	for _, capability := range Capabilities() {
		known[capability.Code] = struct{}{}
	}
	seen := make(map[string]struct{}, len(results))
	for _, result := range results {
		if _, exists := known[result.CapabilityCode]; !exists || !validProbeStatus(result.Status) {
			return &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seen[result.CapabilityCode]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		seen[result.CapabilityCode] = struct{}{}
	}
	for _, result := range results {
		details := map[string]any{
			"reason":     result.Reason,
			"endpointId": result.EndpointID,
			"layers": map[string]any{
				"contract":       result.Layers.Contract,
				"deployment":     result.Layers.Deployment,
				"account":        result.Layers.Account,
				"implementation": result.Layers.Implementation,
				"acceptance":     result.Layers.Acceptance,
			},
		}
		if result.ItemCount != nil {
			details["itemCount"] = *result.ItemCount
		}
		if err := repository.SaveCapabilitySnapshot(ctx, instance, connector.CapabilitySnapshot{
			CapabilityCode: result.CapabilityCode, Status: string(result.Status), EvidenceLevel: evidence.Level,
			Region: evidence.Region, Deployment: evidence.Deployment, AccountFingerprint: evidence.AccountFingerprint, DeviceModel: evidence.DeviceModel,
			FirmwareVersion: evidence.FirmwareVersion, Details: details, VerifiedAt: evidence.VerifiedAt,
			ExpiresAt: evidence.ExpiresAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func LoadAndApplyCapabilitySnapshots(
	ctx context.Context,
	repository CapabilitySnapshotRepository,
	instance connector.Instance,
	results []CapabilityProbeResult,
	scope CapabilityEvaluationScope,
) ([]CapabilityProbeResult, error) {
	if repository == nil {
		return nil, errors.New("DJI_FLIGHTHUB_CAPABILITY_REPOSITORY_UNAVAILABLE")
	}
	snapshots, err := repository.ListCapabilitySnapshots(ctx, instance, strings.TrimSpace(scope.Region), strings.TrimSpace(scope.Deployment))
	if err != nil {
		return nil, err
	}
	return ApplyCapabilitySnapshots(results, snapshots, scope), nil
}

func ApplyCapabilitySnapshots(
	results []CapabilityProbeResult,
	snapshots []connector.CapabilitySnapshot,
	scope CapabilityEvaluationScope,
) []CapabilityProbeResult {
	if scope.Now.IsZero() {
		scope.Now = time.Now()
	}
	scope.Region = strings.TrimSpace(scope.Region)
	scope.Deployment = strings.TrimSpace(scope.Deployment)
	scope.AccountFingerprint = strings.TrimSpace(scope.AccountFingerprint)
	scope.DeviceModel = strings.TrimSpace(scope.DeviceModel)
	scope.FirmwareVersion = strings.TrimSpace(scope.FirmwareVersion)
	actions := make(map[string]bool, len(Capabilities()))
	for _, capability := range Capabilities() {
		actions[capability.Code] = capability.Kind == connector.CapabilityAction
	}
	effective := append([]CapabilityProbeResult(nil), results...)
	for index := range effective {
		if !actions[effective[index].CapabilityCode] {
			continue
		}
		snapshot, reason := selectFieldAcceptanceSnapshot(effective[index].CapabilityCode, snapshots, scope)
		if snapshot == nil {
			effective[index].Layers.Acceptance = ProbeUnverified
			status, statusReason := effectiveProbeStatus(effective[index].Layers, "")
			effective[index].Status = status
			if acceptanceLayerDecides(effective[index].Layers) {
				effective[index].Reason = reason
			} else {
				effective[index].Reason = statusReason
			}
			continue
		}
		acceptance := CapabilityProbeStatus(snapshot.Status)
		if !validProbeStatus(acceptance) {
			acceptance = ProbeUnverified
		}
		effective[index].Layers.Acceptance = acceptance
		status, statusReason := effectiveProbeStatus(effective[index].Layers, "")
		effective[index].Status = status
		if !acceptanceLayerDecides(effective[index].Layers) {
			effective[index].Reason = statusReason
		} else if acceptance == ProbeSupported {
			effective[index].Reason = "field_acceptance_current"
		} else {
			effective[index].Reason = "field_acceptance_" + string(acceptance)
		}
	}
	return effective
}

func acceptanceLayerDecides(layers CapabilityProbeLayers) bool {
	return layers.Contract == ProbeSupported && layers.Deployment == ProbeSupported &&
		(layers.Account == ProbeSupported || layers.Account == ProbeEmpty) && layers.Implementation == ProbeSupported
}

func selectFieldAcceptanceSnapshot(
	capabilityCode string,
	snapshots []connector.CapabilitySnapshot,
	scope CapabilityEvaluationScope,
) (*connector.CapabilitySnapshot, string) {
	_, firmwareBound := deviceBoundFieldAcceptanceCapabilities[capabilityCode]
	if _, accountBound := accountBoundFieldAcceptanceCapabilities[capabilityCode]; !firmwareBound && !accountBound {
		return nil, "field_acceptance_scope_unclassified"
	}
	var selected *connector.CapabilitySnapshot
	firmwareChanged := false
	accountChanged := false
	for index := range snapshots {
		snapshot := &snapshots[index]
		if snapshot.CapabilityCode != capabilityCode || snapshot.EvidenceLevel != "field-write" ||
			snapshot.Region != scope.Region || snapshot.Deployment != scope.Deployment {
			continue
		}
		if scope.AccountFingerprint == "" || snapshot.AccountFingerprint != scope.AccountFingerprint {
			if validAccountFingerprint(snapshot.AccountFingerprint) {
				accountChanged = true
			}
			continue
		}
		if firmwareBound {
			if scope.DeviceModel == "" || scope.FirmwareVersion == "" {
				continue
			}
			if snapshot.DeviceModel == scope.DeviceModel && snapshot.FirmwareVersion != scope.FirmwareVersion {
				firmwareChanged = true
			}
			if snapshot.DeviceModel != scope.DeviceModel || snapshot.FirmwareVersion != scope.FirmwareVersion {
				continue
			}
		} else if snapshot.DeviceModel != "" || snapshot.FirmwareVersion != "" {
			continue
		}
		if selected == nil || snapshot.VerifiedAt.After(selected.VerifiedAt) {
			selected = snapshot
		}
	}
	if selected == nil {
		if accountChanged {
			return nil, "account_acceptance_changed"
		}
		if firmwareChanged {
			return nil, "firmware_acceptance_changed"
		}
		return nil, "field_acceptance_missing"
	}
	if selected.VerifiedAt.After(scope.Now) || (selected.ExpiresAt != nil && !selected.ExpiresAt.After(scope.Now)) {
		return nil, "field_acceptance_expired"
	}
	return selected, ""
}

func validEvidenceLevel(value string) bool {
	switch value {
	case "documented", "fixture", "live-read", "field-write":
		return true
	default:
		return false
	}
}

func validAccountFingerprint(value string) bool {
	if len(value) != 64 {
		return false
	}
	for _, character := range value {
		if (character < '0' || character > '9') && (character < 'a' || character > 'f') {
			return false
		}
	}
	return true
}

func validProbeStatus(value CapabilityProbeStatus) bool {
	switch value {
	case ProbeSupported, ProbeEmpty, ProbeForbidden, ProbeNotApplicable, ProbeUnverified, ProbeDegraded, ProbeFailed:
		return true
	default:
		return false
	}
}
