package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"strings"
	"time"

	workerconfig "aerosight/worker/internal/config"
	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/flighthub"
	_ "github.com/jackc/pgx/v5/stdlib"
)

type acceptanceTarget struct {
	DeviceID        int
	Serial          string
	CameraIndex     string
	DeviceModel     string
	FirmwareVersion string
	Instance        connector.Instance
}

func main() {
	os.Args = normalizeCLIArgs(os.Args)
	projectID := flag.Int("project-id", 0, "AeroSight project id")
	connectorID := flag.Int64("connector-id", 0, "FlightHub connector instance id")
	deviceID := flag.Int("device-id", 0, "exact AeroSight Dock device id")
	cameraIndex := flag.String("camera-index", "", "exact projected Dock camera index")
	confirmed := flag.Bool("confirm-dock-live-acceptance", false, "allow one Dock live start; never start aircraft live or flight")
	persistEvidence := flag.Bool("persist-evidence", false, "persist scoped live.control evidence after a confirmed real success")
	enableLiveControl := flag.Bool("enable-live-control", false, "enable only the project live.control feature after evidence is persisted")
	flag.Parse()
	if !*confirmed || !*persistEvidence || !*enableLiveControl || *projectID <= 0 || *connectorID <= 0 || *deviceID <= 0 || strings.TrimSpace(*cameraIndex) == "" || len(flag.Args()) != 0 {
		exitWithSafeResult("opt_in_required")
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	authSecret := os.Getenv("AUTH_SECRET")
	if databaseURL == "" || len(authSecret) < 16 {
		exitWithSafeResult("configuration_unavailable")
	}
	config, err := workerconfig.Load()
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		exitWithSafeResult("database_unavailable")
	}
	target, err := loadTarget(ctx, database, *projectID, *connectorID, *deviceID, strings.TrimSpace(*cameraIndex))
	if err != nil {
		exitWithSafeResult("dock_scope_unavailable")
	}
	token, err := (flighthub.EncryptedTokenResolver{AuthSecret: authSecret}).ResolveToken(ctx, target.Instance)
	if err != nil {
		exitWithSafeResult("credential_unavailable")
	}
	defer func() { token = "" }()
	client, err := flighthub.NewChinaClient(flighthub.Config{
		Timeout: config.FlightHubHTTPTimeout, MaxRetries: 0, MaxConcurrent: 1,
		RequestsPerSecond: 1, RequestBurst: 1, RequestID: acceptanceRunID,
		MaxResponseBytes: config.FlightHubMaxResponseBytes, AllowedLinkHosts: config.FlightHubAllowedLinkHosts,
	})
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	registry, err := flighthub.NewDefaultLiveSupplierRegistry(client)
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	scope, err := flighthub.LoadReadOnlySmokeContext(ctx, database, target.Instance)
	if err != nil {
		exitWithSafeResult("scope_unavailable")
	}
	scope, err = flighthub.HydrateReadOnlySmokeContext(ctx, client, token, scope)
	if err != nil {
		exitWithSafeResult("scope_unavailable")
	}
	runID := acceptanceRunID()
	guard := newAcceptanceGuard(database, target, scope.ProjectUUID, scope.AccountFingerprint)
	result := flighthub.RunLiveControlAcceptance(ctx, client, registry, token, scope.ProjectUUID, target.Serial, target.CameraIndex, guard)
	target.Serial = ""
	if result.Category == "succeeded" {
		repository := connector.NewSQLResourceRepository(database)
		if err := flighthub.PersistLiveControlAcceptanceEvidence(ctx, repository, target.Instance, result, scope.AccountFingerprint,
			target.DeviceModel, target.FirmwareVersion, target.CameraIndex, runID, time.Now().UTC(), 24*time.Hour); err != nil {
			exitWithSafeResult("evidence_unavailable")
		}
		if err := enableOnlyLiveControl(ctx, database, *projectID); err != nil {
			exitWithSafeResult("feature_flag_unavailable")
		}
	}
	if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
		exitWithSafeResult("output_unavailable")
	}
	if result.Category != "succeeded" {
		os.Exit(2)
	}
}

func normalizeCLIArgs(arguments []string) []string {
	if len(arguments) > 1 && arguments[1] == "--" {
		return append(arguments[:1:1], arguments[2:]...)
	}
	return arguments
}

func loadTarget(ctx context.Context, database *sql.DB, projectID int, connectorID int64, deviceID int, cameraIndex string) (acceptanceTarget, error) {
	var target acceptanceTarget
	var credential, scope []byte
	err := database.QueryRowContext(ctx, `select device.id,identity.identity_json->'attributes'->>'serialNumber',channel.channel_key,
		coalesce(device.device_model,''),coalesce(device.firmware_version,''),adapter.id,adapter.project_id,
		definition.connector_key,definition.version,adapter.credential_envelope_json,adapter.discovery_scope_json
		from devices device
		join device_adapters adapter on adapter.id=device.adapter_id and adapter.project_id=device.project_id
		join connector_definitions definition on definition.id=adapter.connector_definition_id
		join device_external_identities identity on identity.project_id=device.project_id and identity.adapter_id=adapter.id and identity.device_id=device.id
		join device_stream_channels channel on channel.project_id=device.project_id and channel.device_id=device.id
		where device.project_id=$1 and adapter.id=$2 and device.id=$3 and channel.channel_key=$4
		  and device.type='dock' and device.status in('online','degraded') and adapter.status in('connected','degraded')
		  and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'
		  and channel.data_type='video' and channel.availability='available'
		limit 1`, projectID, connectorID, deviceID, cameraIndex).Scan(
		&target.DeviceID, &target.Serial, &target.CameraIndex, &target.DeviceModel, &target.FirmwareVersion,
		&target.Instance.ID, &target.Instance.ProjectID, &target.Instance.ConnectorKey, &target.Instance.Version, &credential, &scope,
	)
	if err != nil || strings.TrimSpace(target.Serial) == "" || strings.TrimSpace(target.DeviceModel) == "" || strings.TrimSpace(target.FirmwareVersion) == "" {
		return acceptanceTarget{}, &flighthub.APIError{SafeCode: "scope_forbidden"}
	}
	target.Instance.CredentialEnvelope = json.RawMessage(credential)
	target.Instance.DiscoveryScope = json.RawMessage(scope)
	return target, nil
}

func newAcceptanceGuard(database *sql.DB, expected acceptanceTarget, projectUUID, accountFingerprint string) flighthub.LiveControlAcceptanceGuard {
	credentialDigest := sha256.Sum256(expected.Instance.CredentialEnvelope)
	return func(ctx context.Context) error {
		current, err := loadTarget(ctx, database, expected.Instance.ProjectID, expected.Instance.ID, expected.DeviceID, expected.CameraIndex)
		if err != nil || current.Serial != expected.Serial || current.DeviceModel != expected.DeviceModel || current.FirmwareVersion != expected.FirmwareVersion ||
			sha256.Sum256(current.Instance.CredentialEnvelope) != credentialDigest {
			return &flighthub.APIError{SafeCode: "connector_changed"}
		}
		var scope struct {
			ProjectUUID        string `json:"projectUuid"`
			AccountFingerprint string `json:"accountFingerprint"`
		}
		if json.Unmarshal(current.Instance.DiscoveryScope, &scope) != nil || !strings.EqualFold(strings.TrimSpace(scope.ProjectUUID), strings.TrimSpace(projectUUID)) ||
			strings.TrimSpace(scope.AccountFingerprint) != strings.TrimSpace(accountFingerprint) {
			return &flighthub.APIError{SafeCode: "connector_changed"}
		}
		return nil
	}
}

func enableOnlyLiveControl(ctx context.Context, database *sql.DB, projectID int) error {
	_, err := database.ExecContext(ctx, `insert into project_feature_flags(project_id,flighthub_action_flags_json)
		values($1,'{"live.control":true}'::jsonb)
		on conflict(project_id) do update set flighthub_action_flags_json=
			coalesce(project_feature_flags.flighthub_action_flags_json,'{}'::jsonb) || '{"live.control":true}'::jsonb`, projectID)
	return err
}

func acceptanceRunID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "acceptance-run-unavailable"
	}
	return hex.EncodeToString(value[:])
}

func exitWithSafeResult(category string) {
	_ = json.NewEncoder(os.Stderr).Encode(flighthub.LiveControlAcceptanceResult{Endpoint: "command", Category: category, Fields: []string{}})
	os.Exit(2)
}
