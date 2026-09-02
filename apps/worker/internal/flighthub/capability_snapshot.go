package flighthub

import (
	"context"
	"errors"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/driver"
)

type CapabilitySnapshotRepository interface {
	SaveCapabilitySnapshot(context.Context, connector.Instance, connector.CapabilitySnapshot) error
	ListCapabilitySnapshots(context.Context, connector.Instance, string, string) ([]connector.CapabilitySnapshot, error)
}

type CapabilitySnapshotEvidence struct {
	Level           string
	Region          string
	Deployment      string
	DeviceModel     string
	FirmwareVersion string
	VerifiedAt      time.Time
	ExpiresAt       *time.Time
}

type CapabilityEvaluationScope struct {
	Region          string
	Deployment      string
	DeviceModel     string
	FirmwareVersion string
	Now             time.Time
}

var firmwareBoundCapabilities = map[string]struct{}{
	"device.control":       {},
	"flight.execute":       {},
	"live.control":         {},
	"live.quality.set":     {},
	"device.camera.change": {},
	"device.lens.change":   {},
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
	evidence.DeviceModel = strings.TrimSpace(evidence.DeviceModel)
	evidence.FirmwareVersion = strings.TrimSpace(evidence.FirmwareVersion)
	if !validEvidenceLevel(evidence.Level) || evidence.Region == "" || evidence.Deployment == "" || evidence.VerifiedAt.IsZero() ||
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
			Region: evidence.Region, Deployment: evidence.Deployment, DeviceModel: evidence.DeviceModel,
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
	scope.DeviceModel = strings.TrimSpace(scope.DeviceModel)
	scope.FirmwareVersion = strings.TrimSpace(scope.FirmwareVersion)
	risks := make(map[string]driver.RiskLevel, len(Capabilities()))
	for _, capability := range Capabilities() {
		risks[capability.Code] = capability.Risk
	}
	effective := append([]CapabilityProbeResult(nil), results...)
	for index := range effective {
		risk := risks[effective[index].CapabilityCode]
		if risk != driver.RiskHigh && risk != driver.RiskCritical {
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
	_, firmwareBound := firmwareBoundCapabilities[capabilityCode]
	var selected *connector.CapabilitySnapshot
	firmwareChanged := false
	for index := range snapshots {
		snapshot := &snapshots[index]
		if snapshot.CapabilityCode != capabilityCode || snapshot.EvidenceLevel != "field-write" ||
			snapshot.Region != scope.Region || snapshot.Deployment != scope.Deployment {
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

func validProbeStatus(value CapabilityProbeStatus) bool {
	switch value {
	case ProbeSupported, ProbeEmpty, ProbeForbidden, ProbeNotApplicable, ProbeUnverified, ProbeDegraded, ProbeFailed:
		return true
	default:
		return false
	}
}
