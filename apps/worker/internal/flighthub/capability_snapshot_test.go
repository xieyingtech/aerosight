package flighthub

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type memoryCapabilitySnapshotRepository struct {
	saved  []connector.CapabilitySnapshot
	listed []connector.CapabilitySnapshot
}

func (repository *memoryCapabilitySnapshotRepository) SaveCapabilitySnapshot(_ context.Context, _ connector.Instance, snapshot connector.CapabilitySnapshot) error {
	repository.saved = append(repository.saved, snapshot)
	return nil
}

func (repository *memoryCapabilitySnapshotRepository) ListCapabilitySnapshots(_ context.Context, _ connector.Instance, _, _ string) ([]connector.CapabilitySnapshot, error) {
	return append([]connector.CapabilitySnapshot(nil), repository.listed...), nil
}

func TestCapabilityProbeResultsPersistSanitizedEvidenceScope(t *testing.T) {
	t.Parallel()
	repository := &memoryCapabilitySnapshotRepository{}
	verifiedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := verifiedAt.Add(time.Hour)
	results := []CapabilityProbeResult{{
		CapabilityCode: "state.read", Status: ProbeSupported, Reason: "read_probe_succeeded", EndpointID: "458069501e0",
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeSupported},
	}}
	if err := PersistCapabilityProbeResults(context.Background(), repository, connector.Instance{ID: 7, ProjectID: 11}, results, CapabilitySnapshotEvidence{
		Level: "live-read", Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300",
		VerifiedAt: verifiedAt, ExpiresAt: &expiresAt,
	}); err != nil {
		t.Fatal(err)
	}
	if len(repository.saved) != 1 {
		t.Fatalf("saved snapshots=%d", len(repository.saved))
	}
	snapshot := repository.saved[0]
	if snapshot.CapabilityCode != "state.read" || snapshot.Status != "supported" || snapshot.EvidenceLevel != "live-read" ||
		snapshot.Region != "cn" || snapshot.Deployment != "cn-public-cloud" || snapshot.DeviceModel != "dock-model" ||
		snapshot.FirmwareVersion != "01.02.0300" || !snapshot.VerifiedAt.Equal(verifiedAt) || snapshot.ExpiresAt == nil || !snapshot.ExpiresAt.Equal(expiresAt) {
		t.Fatalf("snapshot scope/evidence lost: %#v", snapshot)
	}
	if snapshot.Details["reason"] != "read_probe_succeeded" || snapshot.Details["endpointId"] != "458069501e0" {
		t.Fatalf("snapshot details are incomplete: %#v", snapshot.Details)
	}
	if err := PersistCapabilityProbeResults(context.Background(), repository, connector.Instance{}, append(results, results[0]), CapabilitySnapshotEvidence{
		Level: "live-read", Region: "cn", Deployment: "cn-public-cloud", VerifiedAt: verifiedAt, ExpiresAt: &expiresAt,
	}); err == nil || len(repository.saved) != 1 {
		t.Fatal("duplicate capability batch partially persisted")
	}
}

func TestHighRiskCapabilityRequiresCurrentFirmwareAndUnexpiredFieldAcceptance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	baseline := []CapabilityProbeResult{{
		CapabilityCode: "device.control", Status: ProbeUnverified, Reason: "acceptance_required",
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeUnverified},
	}}
	snapshot := connector.CapabilitySnapshot{
		CapabilityCode: "device.control", Status: "supported", EvidenceLevel: "field-write",
		Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300",
		VerifiedAt: now.Add(-time.Hour), ExpiresAt: &expiresAt,
	}
	current := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: now,
	})[0]
	if current.Status != ProbeSupported || current.Layers.Acceptance != ProbeSupported || current.Reason != "field_acceptance_current" {
		t.Fatalf("current field acceptance was not applied: %#v", current)
	}
	notImplementedBaseline := append([]CapabilityProbeResult(nil), baseline...)
	notImplementedBaseline[0].Layers.Implementation = ProbeUnverified
	notImplemented := ApplyCapabilitySnapshots(notImplementedBaseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: now,
	})[0]
	if notImplemented.Status != ProbeUnverified || notImplemented.Reason != "implementation_unavailable" {
		t.Fatalf("field acceptance bypassed implementation layer: %#v", notImplemented)
	}
	changed := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.03.0000", Now: now,
	})[0]
	if changed.Status != ProbeUnverified || changed.Layers.Acceptance != ProbeUnverified || changed.Reason != "firmware_acceptance_changed" {
		t.Fatalf("old firmware acceptance widened new firmware: %#v", changed)
	}
	expired := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: expiresAt,
	})[0]
	if expired.Status != ProbeUnverified || expired.Layers.Acceptance != ProbeUnverified || expired.Reason != "field_acceptance_expired" {
		t.Fatalf("expired field acceptance remained enabled: %#v", expired)
	}
	generic := snapshot
	generic.DeviceModel, generic.FirmwareVersion = "", ""
	notInherited := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{generic}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: now,
	})[0]
	if notInherited.Status != ProbeUnverified || notInherited.Layers.Acceptance != ProbeUnverified {
		t.Fatalf("generic evidence widened firmware-bound action: %#v", notInherited)
	}
}

func TestCapabilityProbeSnapshotsPersistAndLoadFromPostgres(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("AEROSIGHT_TEST_DATABASE_URL is not configured")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer database.Close()
	ctx := context.Background()
	suffix := time.Now().UnixNano()
	var teamID, projectID int
	var connectorID, definitionID int64
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("capability-snapshot-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("capability-snapshot-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected') returning id`, projectID, teamID, fmt.Sprintf("capability-snapshot-%d", suffix), definitionID).Scan(&connectorID); err != nil {
		t.Fatal(err)
	}
	repository := connector.NewSQLResourceRepository(database)
	instance := connector.Instance{ID: connectorID, ProjectID: projectID}
	verifiedAt := time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC)
	expiresAt := verifiedAt.Add(time.Hour)
	result := CapabilityProbeResult{
		CapabilityCode: "state.read", Status: ProbeSupported, Reason: "read_probe_succeeded", EndpointID: "458069501e0",
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeSupported},
	}
	evidence := CapabilitySnapshotEvidence{
		Level: "live-read", Region: "cn", Deployment: "cn-public-cloud", DeviceModel: "dock-model", FirmwareVersion: "01.02.0300",
		VerifiedAt: verifiedAt, ExpiresAt: &expiresAt,
	}
	if err := PersistCapabilityProbeResults(ctx, repository, instance, []CapabilityProbeResult{result}, evidence); err != nil {
		t.Fatal(err)
	}
	result.Status = ProbeEmpty
	result.Reason = "upstream_empty"
	evidence.VerifiedAt = verifiedAt.Add(time.Minute)
	evidence.ExpiresAt = nil
	if err := PersistCapabilityProbeResults(ctx, repository, instance, []CapabilityProbeResult{result}, evidence); err != nil {
		t.Fatal(err)
	}
	snapshots, err := repository.ListCapabilitySnapshots(ctx, instance, "cn", "cn-public-cloud")
	if err != nil {
		t.Fatal(err)
	}
	if len(snapshots) != 1 || snapshots[0].Status != "empty" || snapshots[0].EvidenceLevel != "live-read" ||
		snapshots[0].DeviceModel != "dock-model" || snapshots[0].FirmwareVersion != "01.02.0300" ||
		!snapshots[0].VerifiedAt.Equal(evidence.VerifiedAt) || snapshots[0].ExpiresAt != nil || snapshots[0].Details["reason"] != "upstream_empty" {
		t.Fatalf("persisted capability snapshot did not update idempotently: %#v", snapshots)
	}
}
