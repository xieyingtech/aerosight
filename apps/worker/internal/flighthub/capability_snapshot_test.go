package flighthub

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type memoryCapabilitySnapshotRepository struct {
	saved  []connector.CapabilitySnapshot
	listed []connector.CapabilitySnapshot
}

func TestEveryActionCapabilityHasAnExplicitFieldAcceptanceScope(t *testing.T) {
	t.Parallel()
	for _, capability := range Capabilities() {
		if capability.Kind != connector.CapabilityAction {
			continue
		}
		_, deviceBound := deviceBoundFieldAcceptanceCapabilities[capability.Code]
		_, accountBound := accountBoundFieldAcceptanceCapabilities[capability.Code]
		if deviceBound == accountBound {
			t.Fatalf("action capability %q must have exactly one field acceptance scope", capability.Code)
		}
	}
}

func TestFieldAcceptanceNeverCrossesAccountOrDeployment(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	evidence := connector.CapabilitySnapshot{CapabilityCode: "organization.write", Status: "supported", EvidenceLevel: "field-write",
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), VerifiedAt: now.Add(-time.Minute)}
	baseline := []CapabilityProbeResult{{CapabilityCode: "organization.write", Status: ProbeUnverified,
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeUnverified}}}
	for name, scope := range map[string]CapabilityEvaluationScope{
		"account":    {Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("b", 64), Now: now},
		"region":     {Region: "eu", Deployment: "eu-public-cloud", AccountFingerprint: strings.Repeat("a", 64), Now: now},
		"unresolved": {Region: "cn", Deployment: "cn-public-cloud", Now: now},
	} {
		result := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{evidence}, scope)[0]
		if result.Status == ProbeSupported {
			t.Fatalf("%s scope inherited incompatible field acceptance: %#v", name, result)
		}
	}
	matched := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{evidence}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), Now: now,
	})[0]
	if matched.Status != ProbeSupported {
		t.Fatalf("exact non-device acceptance did not match: %#v", matched)
	}
}

func TestCapabilityAccountFingerprintIsStableAndDoesNotContainVendorIdentity(t *testing.T) {
	t.Parallel()
	organizationUUID := "00000000-0000-4000-8000-000000000010"
	userID := "CURRENT_VENDOR_USER_REDACTED"
	first, err := CapabilityAccountFingerprint(organizationUUID, userID)
	if err != nil {
		t.Fatal(err)
	}
	second, err := CapabilityAccountFingerprint(strings.ToUpper(organizationUUID), " "+userID+" ")
	if err != nil || first != second || !validAccountFingerprint(first) {
		t.Fatalf("account fingerprint is not stable: first=%q second=%q err=%v", first, second, err)
	}
	if strings.Contains(first, organizationUUID) || strings.Contains(first, userID) {
		t.Fatal("account fingerprint exposed a raw vendor identifier")
	}
}

func TestFieldWriteEvidenceRequiresAnAccountFingerprintBeforePersistence(t *testing.T) {
	t.Parallel()
	repository := &memoryCapabilitySnapshotRepository{}
	result := CapabilityProbeResult{CapabilityCode: "organization.write", Status: ProbeSupported,
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeSupported}}
	evidence := CapabilitySnapshotEvidence{Level: "field-write", Region: "cn", Deployment: "cn-public-cloud", VerifiedAt: time.Now().UTC()}
	if err := PersistCapabilityProbeResults(context.Background(), repository, connector.Instance{ID: 7, ProjectID: 11}, []CapabilityProbeResult{result}, evidence); err == nil {
		t.Fatal("field-write evidence without an account fingerprint was accepted")
	}
	evidence.AccountFingerprint = strings.Repeat("a", 64)
	if err := PersistCapabilityProbeResults(context.Background(), repository, connector.Instance{ID: 7, ProjectID: 11}, []CapabilityProbeResult{result}, evidence); err != nil {
		t.Fatal(err)
	}
	if len(repository.saved) != 1 || repository.saved[0].AccountFingerprint != evidence.AccountFingerprint {
		t.Fatalf("account-scoped field acceptance was not persisted: %#v", repository.saved)
	}
}

func TestEveryFlightHubWorkerWriteGateChecksTheCurrentAccount(t *testing.T) {
	t.Parallel()
	for _, filename := range []string{
		"control_command.go", "control_session.go", "device_admin_action.go", "flight_action.go",
		"geospatial_action.go", "live_action.go", "live_session.go", "management_write.go", "model_delete.go",
	} {
		source, err := os.ReadFile(filename)
		if err != nil {
			t.Fatal(err)
		}
		if !strings.Contains(string(source), "capability.account_fingerprint=adapter.discovery_scope_json->>'accountFingerprint'") {
			t.Fatalf("%s does not bind field acceptance to the current connector account", filename)
		}
		if !strings.Contains(string(source), "capability.region='cn' and capability.deployment='cn-public-cloud'") {
			t.Fatalf("%s does not bind field acceptance to the connector deployment", filename)
		}
	}
}

func (repository *memoryCapabilitySnapshotRepository) SaveCapabilityAccountFingerprint(context.Context, connector.Instance, string) error {
	return nil
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
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "dock-model", FirmwareVersion: "01.02.0300",
		VerifiedAt: now.Add(-time.Hour), ExpiresAt: &expiresAt,
	}
	current := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: now,
	})[0]
	if current.Status != ProbeSupported || current.Layers.Acceptance != ProbeSupported || current.Reason != "field_acceptance_current" {
		t.Fatalf("current field acceptance was not applied: %#v", current)
	}
	notImplementedBaseline := append([]CapabilityProbeResult(nil), baseline...)
	notImplementedBaseline[0].Layers.Implementation = ProbeUnverified
	notImplemented := ApplyCapabilitySnapshots(notImplementedBaseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: now,
	})[0]
	if notImplemented.Status != ProbeUnverified || notImplemented.Reason != "implementation_unavailable" {
		t.Fatalf("field acceptance bypassed implementation layer: %#v", notImplemented)
	}
	changed := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "dock-model", FirmwareVersion: "01.03.0000", Now: now,
	})[0]
	if changed.Status != ProbeUnverified || changed.Layers.Acceptance != ProbeUnverified || changed.Reason != "firmware_acceptance_changed" {
		t.Fatalf("old firmware acceptance widened new firmware: %#v", changed)
	}
	expired := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{snapshot}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: expiresAt,
	})[0]
	if expired.Status != ProbeUnverified || expired.Layers.Acceptance != ProbeUnverified || expired.Reason != "field_acceptance_expired" {
		t.Fatalf("expired field acceptance remained enabled: %#v", expired)
	}
	generic := snapshot
	generic.DeviceModel, generic.FirmwareVersion = "", ""
	notInherited := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{generic}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "dock-model", FirmwareVersion: "01.02.0300", Now: now,
	})[0]
	if notInherited.Status != ProbeUnverified || notInherited.Layers.Acceptance != ProbeUnverified {
		t.Fatalf("generic evidence widened firmware-bound action: %#v", notInherited)
	}
}

func TestCameraCapabilityNeverInheritsAnotherModelOrFirmwareAcceptance(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	expiresAt := now.Add(time.Hour)
	baseline := []CapabilityProbeResult{{
		CapabilityCode: "device.lens.change", Status: ProbeUnverified, Reason: "acceptance_required",
		Layers: CapabilityProbeLayers{Contract: ProbeSupported, Deployment: ProbeSupported, Account: ProbeSupported, Implementation: ProbeSupported, Acceptance: ProbeUnverified},
	}}
	evidence := connector.CapabilitySnapshot{CapabilityCode: "device.lens.change", Status: "supported", EvidenceLevel: "field-write",
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "matrice4td", FirmwareVersion: "10.01", VerifiedAt: now.Add(-time.Minute), ExpiresAt: &expiresAt}
	for _, scope := range []CapabilityEvaluationScope{
		{Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "matrice3td", FirmwareVersion: "10.01", Now: now},
		{Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "matrice4td", FirmwareVersion: "10.02", Now: now},
		{Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "", FirmwareVersion: "", Now: now},
	} {
		result := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{evidence}, scope)[0]
		if result.Status == ProbeSupported {
			t.Fatalf("scope %#v inherited incompatible evidence", scope)
		}
	}
	matched := ApplyCapabilitySnapshots(baseline, []connector.CapabilitySnapshot{evidence}, CapabilityEvaluationScope{
		Region: "cn", Deployment: "cn-public-cloud", AccountFingerprint: strings.Repeat("a", 64), DeviceModel: "matrice4td", FirmwareVersion: "10.01", Now: now,
	})[0]
	if matched.Status != ProbeSupported {
		t.Fatalf("exact evidence was not applied: %#v", matched)
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
