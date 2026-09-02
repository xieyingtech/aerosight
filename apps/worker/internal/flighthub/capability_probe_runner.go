package flighthub

import (
	"context"
	"errors"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

type RuntimeCapabilityProbeClient interface {
	ProbeCapabilities(context.Context, CapabilityProbeInput) ([]CapabilityProbeResult, error)
	GetCurrentOrganizationRole(context.Context, string, string) (CurrentOrganizationRole, error)
}

type RuntimeCapabilityProbeStore interface {
	CapabilitySnapshotRepository
	ListManagedDevices(context.Context, connector.Instance) ([]connector.ManagedConnectorDevice, error)
}

type CapabilityProbeRunner struct {
	runner   connector.SyncRunner
	client   RuntimeCapabilityProbeClient
	resolver TokenResolver
	store    RuntimeCapabilityProbeStore
	ttl      time.Duration
	now      func() time.Time
}

func NewCapabilityProbeRunner(runner connector.SyncRunner, client RuntimeCapabilityProbeClient, resolver TokenResolver, store RuntimeCapabilityProbeStore, ttl time.Duration, now func() time.Time) (*CapabilityProbeRunner, error) {
	if runner == nil || client == nil || resolver == nil || store == nil || ttl <= 0 {
		return nil, errors.New("FlightHub capability probe runner configuration is invalid")
	}
	if now == nil {
		now = time.Now
	}
	return &CapabilityProbeRunner{runner: runner, client: client, resolver: resolver, store: store, ttl: ttl, now: now}, nil
}

func (runner *CapabilityProbeRunner) Run(ctx context.Context, instance connector.Instance, mode connector.DiscoveryMode) (connector.SyncApplyResult, error) {
	result, err := runner.runner.Run(ctx, instance, mode)
	if err != nil {
		return result, err
	}
	now := runner.now().UTC()
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return result, err
	}
	snapshots, err := runner.store.ListCapabilitySnapshots(ctx, instance, "cn", "cn-public-cloud")
	if err != nil {
		return result, err
	}
	if validAccountFingerprint(scope.AccountFingerprint) && capabilityEvidenceCurrent(snapshots, scope.AccountFingerprint, now) {
		return result, nil
	}
	token, err := runner.resolver.ResolveToken(ctx, instance)
	if err != nil {
		return result, err
	}
	defer func() { token = "" }()
	accountFingerprint := scope.AccountFingerprint
	if scope.OrganizationUUID != "" {
		currentRole, roleErr := runner.client.GetCurrentOrganizationRole(ctx, token, scope.OrganizationUUID)
		if roleErr != nil {
			return result, roleErr
		}
		if !strings.EqualFold(strings.TrimSpace(currentRole.OrganizationUUID), scope.OrganizationUUID) {
			return result, &APIError{SafeCode: "scope_forbidden"}
		}
		accountFingerprint, err = CapabilityAccountFingerprint(scope.OrganizationUUID, currentRole.UserID)
		if err != nil {
			return result, err
		}
		if err := runner.store.SaveCapabilityAccountFingerprint(ctx, instance, accountFingerprint); err != nil {
			return result, err
		}
	}
	if capabilityEvidenceCurrent(snapshots, accountFingerprint, now) {
		return result, nil
	}
	devices, err := runner.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return result, err
	}
	input := CapabilityProbeInput{Token: token, Region: "cn", Deployment: "cn-public-cloud", ProjectUUID: scope.ProjectUUID}
	evidence := CapabilitySnapshotEvidence{Level: "live-read", Region: input.Region, Deployment: input.Deployment, AccountFingerprint: accountFingerprint, VerifiedAt: now}
	if len(devices) > 0 {
		input.DeviceSerial = devices[0].Serial
		evidence.DeviceModel = devices[0].ModelKey
	}
	expiresAt := now.Add(runner.ttl)
	evidence.ExpiresAt = &expiresAt
	results, err := runner.client.ProbeCapabilities(ctx, input)
	if err != nil {
		return result, err
	}
	return result, PersistCapabilityProbeResults(ctx, runner.store, instance, results, evidence)
}

func capabilityEvidenceCurrent(snapshots []connector.CapabilitySnapshot, accountFingerprint string, now time.Time) bool {
	current := make(map[string]bool, len(Capabilities()))
	for _, snapshot := range snapshots {
		if snapshot.EvidenceLevel != "live-read" || snapshot.AccountFingerprint != accountFingerprint || snapshot.VerifiedAt.After(now) || snapshot.ExpiresAt == nil || !snapshot.ExpiresAt.After(now) {
			continue
		}
		current[snapshot.CapabilityCode] = true
	}
	for _, capability := range Capabilities() {
		if !current[capability.Code] {
			return false
		}
	}
	return true
}
