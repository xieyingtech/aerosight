package main

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"flag"
	"os"
	"path/filepath"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/flighthub"
	_ "github.com/jackc/pgx/v5/stdlib"
)

const confirmationFlag = "confirm-read-only-smoke"

func main() {
	projectID := flag.Int("project-id", 0, "AeroSight project id")
	connectorID := flag.Int64("connector-id", 0, "FlightHub connector instance id")
	confirmed := flag.Bool(confirmationFlag, false, "explicitly allow released GET requests to the configured FlightHub account")
	persistEvidence := flag.Bool("persist-evidence", false, "persist sanitized live-read capability evidence locally")
	manifestPath := flag.String("manifest", "", "optional endpoint manifest path")
	flag.Parse()
	if !validInvocation(*confirmed, *projectID, *connectorID, flag.Args()) {
		exitWithSafeResult("opt_in_required")
	}
	databaseURL := strings.TrimSpace(os.Getenv("DATABASE_URL"))
	authSecret := os.Getenv("AUTH_SECRET")
	if databaseURL == "" || len(authSecret) < 16 {
		exitWithSafeResult("configuration_unavailable")
	}
	database, err := sql.Open("pgx", databaseURL)
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	defer database.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
	defer cancel()
	if err := database.PingContext(ctx); err != nil {
		exitWithSafeResult("database_unavailable")
	}
	instance, err := loadInstance(ctx, database, *projectID, *connectorID)
	if err != nil {
		exitWithSafeResult("connector_unavailable")
	}
	manifest, err := openManifest(*manifestPath)
	if err != nil {
		exitWithSafeResult("manifest_unavailable")
	}
	defer manifest.Close()
	endpoints, err := flighthub.LoadReadOnlySmokeManifest(manifest)
	if err != nil {
		exitWithSafeResult("manifest_invalid")
	}
	token, err := (flighthub.EncryptedTokenResolver{AuthSecret: authSecret}).ResolveToken(ctx, instance)
	if err != nil {
		exitWithSafeResult("credential_unavailable")
	}
	defer func() { token = "" }()
	client, err := flighthub.NewChinaClient(flighthub.Config{Timeout: 8 * time.Second, MaxRetries: 1, MaxConcurrent: 2, RequestsPerSecond: 2, RequestBurst: 2, RequestID: smokeRequestID})
	if err != nil {
		exitWithSafeResult("configuration_unavailable")
	}
	scope, err := flighthub.LoadReadOnlySmokeContext(ctx, database, instance)
	if err != nil {
		exitWithSafeResult("scope_unavailable")
	}
	hydratedScope, hydrationErr := flighthub.HydrateReadOnlySmokeContext(ctx, client, token, scope)
	if hydrationErr == nil {
		scope = hydratedScope
	} else if *persistEvidence {
		exitWithSafeResult("scope_unavailable")
	}
	results := flighthub.RunReadOnlySmoke(ctx, client, token, endpoints, scope)
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetEscapeHTML(true)
	for _, result := range results {
		if err := encoder.Encode(result); err != nil {
			exitWithSafeResult("output_unavailable")
		}
	}
	if *persistEvidence {
		repository := connector.NewSQLResourceRepository(database)
		if err := flighthub.PersistReadOnlySmokeEvidence(ctx, repository, instance, endpoints, results, scope, time.Now().UTC(), 15*time.Minute); err != nil {
			exitWithSafeResult("evidence_unavailable")
		}
	}
}

func smokeRequestID() string {
	var value [16]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "smoke-request-unavailable"
	}
	return hex.EncodeToString(value[:])
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

func openManifest(explicit string) (*os.File, error) {
	if explicit = strings.TrimSpace(explicit); explicit != "" {
		return os.Open(filepath.Clean(explicit))
	}
	for _, candidate := range []string{
		"contracts/dji-flighthub/v2/endpoints.tsv",
		"../../contracts/dji-flighthub/v2/endpoints.tsv",
	} {
		file, err := os.Open(candidate)
		if err == nil {
			return file, nil
		}
	}
	return nil, os.ErrNotExist
}

func exitWithSafeResult(category string) {
	result := flighthub.ReadOnlySmokeResult{Endpoint: "command", Category: category, Fields: []string{}}
	_ = json.NewEncoder(os.Stderr).Encode(result)
	os.Exit(2)
}
