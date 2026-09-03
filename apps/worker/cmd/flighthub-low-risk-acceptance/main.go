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

const confirmationFlag = "confirm-low-risk-write-acceptance"

func main() {
	projectID := flag.Int("project-id", 0, "AeroSight project id")
	connectorID := flag.Int64("connector-id", 0, "FlightHub connector instance id")
	confirmed := flag.Bool(confirmationFlag, false, "allow only temporary credential issuance; never upload, start a task, or control a device")
	persistEvidence := flag.Bool("persist-evidence", false, "persist sanitized, account-bound temporary-credential field evidence locally")
	flag.Parse()
	if !validInvocation(*confirmed, *projectID, *connectorID, flag.Args()) {
		exitWithSafeResult("opt_in_required")
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	authSecret := os.Getenv("AUTH_SECRET")
	if databaseURL == "" || len(authSecret) < 16 {
		exitWithSafeResult("configuration_unavailable")
	}
	workerConfig, err := workerconfig.Load()
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
	instance, err := loadInstance(ctx, database, *projectID, *connectorID)
	if err != nil {
		exitWithSafeResult("connector_unavailable")
	}
	token, err := (flighthub.EncryptedTokenResolver{AuthSecret: authSecret}).ResolveToken(ctx, instance)
	if err != nil {
		exitWithSafeResult("credential_unavailable")
	}
	defer func() { token = "" }()
	client, err := flighthub.NewChinaClient(flighthub.Config{
		Timeout: workerConfig.FlightHubHTTPTimeout, MaxRetries: 0, MaxConcurrent: 1,
		RequestsPerSecond: 1, RequestBurst: 1, RequestID: acceptanceRequestID,
		MaxResponseBytes: workerConfig.FlightHubMaxResponseBytes, AllowedLinkHosts: workerConfig.FlightHubAllowedLinkHosts,
	})
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	scope, err := flighthub.LoadReadOnlySmokeContext(ctx, database, instance)
	if err != nil {
		exitWithSafeResult("scope_unavailable")
	}
	scope, err = flighthub.HydrateReadOnlySmokeContext(ctx, client, token, scope)
	if err != nil {
		exitWithSafeResult("scope_unavailable")
	}
	guard := newAcceptanceGuard(database, instance, scope.ProjectUUID, scope.AccountFingerprint)
	results := flighthub.RunTemporaryCredentialAcceptance(ctx, client, token, scope.ProjectUUID, acceptanceFileUUID(), guard)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(true)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			exitWithSafeResult("output_unavailable")
		}
	}
	if *persistEvidence {
		repository := connector.NewSQLResourceRepository(database)
		if err := flighthub.PersistTemporaryCredentialAcceptanceEvidence(ctx, repository, instance, results, scope.AccountFingerprint, time.Now().UTC(), 24*time.Hour); err != nil {
			exitWithSafeResult(safeAcceptanceError(err))
		}
	}
}

func newAcceptanceGuard(database *sql.DB, instance connector.Instance, projectUUID, accountFingerprint string) flighthub.TemporaryCredentialAcceptanceGuard {
	credentialDigest := sha256.Sum256(instance.CredentialEnvelope)
	return func(ctx context.Context, endpoint string) error {
		if database == nil || (endpoint != "454273351e0" && endpoint != "458069518e0") {
			return &flighthub.APIError{SafeCode: "acceptance_guard_failed"}
		}
		var credential, rawScope []byte
		err := database.QueryRowContext(ctx, `select adapter.credential_envelope_json,adapter.discovery_scope_json
			from device_adapters adapter join connector_definitions definition on definition.id=adapter.connector_definition_id
			where adapter.id=$1 and adapter.project_id=$2 and adapter.status in('connecting','connected','degraded')
			  and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'`, instance.ID, instance.ProjectID).Scan(&credential, &rawScope)
		if err != nil || sha256.Sum256(credential) != credentialDigest {
			return &flighthub.APIError{SafeCode: "connector_changed"}
		}
		var current struct {
			ProjectUUID        string `json:"projectUuid"`
			AccountFingerprint string `json:"accountFingerprint"`
		}
		if json.Unmarshal(rawScope, &current) != nil || !strings.EqualFold(strings.TrimSpace(current.ProjectUUID), strings.TrimSpace(projectUUID)) ||
			(strings.TrimSpace(current.AccountFingerprint) != "" && strings.TrimSpace(current.AccountFingerprint) != strings.TrimSpace(accountFingerprint)) {
			return &flighthub.APIError{SafeCode: "connector_changed"}
		}
		return nil
	}
}

func validInvocation(confirmed bool, projectID int, connectorID int64, arguments []string) bool {
	return confirmed && projectID > 0 && connectorID > 0 && len(arguments) == 0
}

func loadInstance(ctx context.Context, database *sql.DB, projectID int, connectorID int64) (connector.Instance, error) {
	var instance connector.Instance
	var credential, scope []byte
	err := database.QueryRowContext(ctx, `select adapter.id,adapter.project_id,definition.connector_key,definition.version,
		adapter.credential_envelope_json,adapter.discovery_scope_json
		from device_adapters adapter join connector_definitions definition on definition.id=adapter.connector_definition_id
		where adapter.id=$1 and adapter.project_id=$2 and adapter.status in('connecting','connected','degraded')
		  and definition.connector_key='dji.flighthub2' and definition.version='1.0.0'`, connectorID, projectID).Scan(
		&instance.ID, &instance.ProjectID, &instance.ConnectorKey, &instance.Version, &credential, &scope,
	)
	if err != nil {
		return connector.Instance{}, err
	}
	instance.CredentialEnvelope = json.RawMessage(credential)
	instance.DiscoveryScope = json.RawMessage(scope)
	return instance, nil
}

func acceptanceRequestID() string {
	return randomHex("acceptance-request-unavailable")
}

func acceptanceFileUUID() string {
	return "aerosight-acceptance-" + randomHex("unavailable")
}

func randomHex(fallback string) string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return fallback
	}
	return hex.EncodeToString(value[:])
}

func safeAcceptanceError(err error) string {
	if flighthub.IsSafeCode(err, "acceptance_incomplete") {
		return "acceptance_incomplete"
	}
	return "evidence_unavailable"
}

func exitWithSafeResult(category string) {
	result := flighthub.TemporaryCredentialAcceptanceResult{Endpoint: "command", Category: category, Fields: []string{}}
	_ = json.NewEncoder(os.Stderr).Encode(result)
	os.Exit(2)
}
