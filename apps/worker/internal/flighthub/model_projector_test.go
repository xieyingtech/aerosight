package flighthub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

func modelProjectionFixture(now time.Time) ModelCatalogPoll {
	return ModelCatalogPoll{
		Models: []ModelSummary{{
			ID: 41, Name: "脱敏二维模型", FileType: ModelFile2D, ShowOnMap: true,
			Size: 2048, CreatedAt: now.Add(-time.Hour).UnixMilli(), UpdatedAt: now.UnixMilli(),
		}},
		OpenModels: []OpenModel{{
			ResourceUUID: "RESOURCE_REDACTED_01", ModelUUID: "MODEL_REDACTED_01", ModelType: OpenModel3D,
			ModelStatus: OpenModelReconstructionExecuting, ModelSize: 1024, ReconstructionProgress: 42,
			ZipStatus: OpenModelZipRunning, ZipProgress: 12,
		}},
		Resources: []OpenModelResource{{
			ResourceUUID: "RESOURCE_REDACTED_01", Status: 1, Size: 8192,
			FileNames: []string{"DJI_SYNTHETIC_0001.JPG", "DJI_SYNTHETIC_0002.JPG"},
		}},
		ModelComplete: true, OpenModelsComplete: true, ResourcesComplete: false, ReceivedAt: now,
	}
}

func TestModelRemoteResourcesAreIdempotentVersionedAndSecretMinimal(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	poll := modelProjectionFixture(now)
	first, err := modelRemoteResources(poll.Models)
	if err != nil || len(first) != 1 {
		t.Fatalf("models=%#v err=%v", first, err)
	}
	second, err := modelRemoteResources(poll.Models)
	if err != nil || second[0].RemoteID != first[0].RemoteID || second[0].RemoteVersion != first[0].RemoteVersion {
		t.Fatalf("model projection is not idempotent: first=%#v second=%#v err=%v", first, second, err)
	}
	changed := append([]ModelSummary(nil), poll.Models...)
	changed[0].Size++
	updated, err := modelRemoteResources(changed)
	if err != nil || updated[0].RemoteVersion == first[0].RemoteVersion {
		t.Fatalf("model version did not change: before=%#v after=%#v err=%v", first, updated, err)
	}
	if _, err := modelRemoteResources([]ModelSummary{poll.Models[0], poll.Models[0]}); !IsSafeCode(err, "schema_incompatible") {
		t.Fatalf("duplicate model error=%v", err)
	}

	openResources, err := openModelRemoteResources(poll.OpenModels, poll.Resources)
	if err != nil || len(openResources) != 2 {
		t.Fatalf("open resources=%#v err=%v", openResources, err)
	}
	serialized := fmt.Sprintf("%#v", []map[string]any{openResources[0].Summary, openResources[1].Summary})
	if strings.Contains(serialized, poll.Resources[0].FileNames[0]) || strings.Contains(serialized, poll.OpenModels[0].ResourceUUID) {
		t.Fatalf("model summary retained a raw resource identity or file name")
	}
}

func TestModelCatalogSinkKeepsRunningViewIncompleteAndLinksProjector(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 30, 0, 0, time.UTC)
	resources := &remoteResourceWriterFixture{}
	projector := &flightCatalogProjectorFixture{}
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, resources, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	poll := modelProjectionFixture(now)
	if err := sink.ApplyModelCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
		t.Fatal(err)
	}
	if len(resources.batches) != 2 || resources.batches[0].Kind != "model" || !resources.batches[0].CompleteSnapshot ||
		resources.batches[1].Kind != "model-resource" || resources.batches[1].CompleteSnapshot || len(projector.modelPolls) != 1 {
		t.Fatalf("batches=%#v projector=%#v", resources.batches, projector.modelPolls)
	}
}

func TestModelCatalogStreamProjectsCompletedRunningAndPartialStates(t *testing.T) {
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	poll := modelProjectionFixture(now)
	model := poll.OpenModels[0]
	client := &resourceClientFixture{
		models: poll.Models, openModels: []OpenModel{model},
		openModelDetails: map[string]OpenModel{model.ModelUUID: model}, openModelDetailErrors: map[string]error{},
		openResources: map[string]OpenModelResource{model.ResourceUUID: poll.Resources[0]}, openResourceErrors: map[string]error{},
	}
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := coordinator.pollModels(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if err != nil || cursor["complete"] != true || cursor["models"] != 1 || cursor["openModels"] != 1 || cursor["resources"] != 1 || len(sink.modelPolls) != 1 {
		t.Fatalf("cursor=%#v polls=%#v err=%v", cursor, sink.modelPolls, err)
	}
	if !sink.modelPolls[0].ModelComplete || !sink.modelPolls[0].OpenModelsComplete || sink.modelPolls[0].ResourcesComplete {
		t.Fatalf("model completeness=%#v", sink.modelPolls[0])
	}

	client.openResourceErrors[model.ResourceUUID] = &APIError{SafeCode: "upstream_unavailable", Retryable: true}
	cursor, _, err = coordinator.pollModels(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	partial := sink.modelPolls[len(sink.modelPolls)-1]
	if !IsSafeCode(err, "upstream_unavailable") || cursor["complete"] != false || !partial.ModelComplete ||
		!partial.OpenModelsComplete || partial.ResourcesComplete || len(partial.Models) != 1 || len(partial.OpenModels) != 1 || len(partial.Resources) != 0 {
		t.Fatalf("partial cursor=%#v poll=%#v err=%v", cursor, partial, err)
	}
}

type modelDownloadClientFixture struct {
	calls       int
	projectUUID string
	fileID      string
	download    ModelDownload
	err         error
}

func (fixture *modelDownloadClientFixture) GetModelDownloadURL(_ context.Context, _, projectUUID, fileID string) (ModelDownload, error) {
	fixture.calls++
	fixture.projectUUID = projectUUID
	fixture.fileID = fileID
	return fixture.download, fixture.err
}

func TestSQLModelAssetsAreIdempotentVersionedAndProjectScoped(t *testing.T) {
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
	var teamID, projectID, otherProjectID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-model-assets-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-model-assets-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-model-assets-other-%d", suffix)).Scan(&otherProjectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,credential_envelope_json
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5,'{}'::jsonb) returning id`, projectID, teamID,
		fmt.Sprintf("flighthub-model-assets-%d", suffix), definitionID, fmt.Sprintf(`{"projectUuid":"%s","projectName":"脱敏项目"}`, runtimeProjectUUID)).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 9, 2, 9, 30, 0, 0, time.UTC)
	projector := NewSQLFlightCatalogProjector(database, &telemetryIngestorFixture{}, func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	repository := connector.NewSQLResourceRepository(database)
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, repository, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	poll := modelProjectionFixture(clock)
	for iteration := 0; iteration < 2; iteration++ {
		if err := sink.ApplyModelCatalog(ctx, instance, poll); err != nil {
			t.Fatal(err)
		}
	}
	var assetCount, modelResourceCount, referenceCount int
	if err := database.QueryRowContext(ctx, `select count(*) from assets where project_id=$1 and metadata_json->>'source'='dji-flighthub-openapi' and metadata_json->>'sourceKind' in('model','model-resource')`, projectID).Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and resource_kind in('model','model-resource') and canonical_target_type='asset'`, projectID, adapterID).Scan(&modelResourceCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_asset_access_refs where project_id=$1 and connector_instance_id=$2 and access_kind='model'`, projectID, adapterID).Scan(&referenceCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 3 || modelResourceCount != 3 || referenceCount != 1 {
		t.Fatalf("assets=%d resources=%d references=%d", assetCount, modelResourceCount, referenceCount)
	}

	var assetID int
	var beforeVersion string
	if err := database.QueryRowContext(ctx, `select id,object_version from assets where project_id=$1 and metadata_json->>'sourceKind'='model'`, projectID).Scan(&assetID, &beforeVersion); err != nil {
		t.Fatal(err)
	}
	poll.Models[0].Size++
	poll.Models[0].UpdatedAt++
	if err := sink.ApplyModelCatalog(ctx, instance, poll); err != nil {
		t.Fatal(err)
	}
	var afterAssetID int
	var afterVersion string
	if err := database.QueryRowContext(ctx, `select id,object_version from assets where project_id=$1 and metadata_json->>'sourceKind'='model'`, projectID).Scan(&afterAssetID, &afterVersion); err != nil {
		t.Fatal(err)
	}
	if afterAssetID != assetID || afterVersion == beforeVersion {
		t.Fatalf("asset id/version before=%d/%s after=%d/%s", assetID, beforeVersion, afterAssetID, afterVersion)
	}

	downloadClient := &modelDownloadClientFixture{download: ModelDownload{
		ID: poll.Models[0].ID, URL: "https://objects.vendor.example/model.zip?Signature=REDACTED",
		ExpiresAt: clock.Add(10 * time.Minute), Ready: true,
	}}
	access, err := NewModelAssetAccessService(database, downloadClient, tokenResolverFixture{token: "TOKEN_REDACTED"}, flightProjectorTestSecret, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	download, err := access.RefreshDownload(ctx, instance, assetID)
	if err != nil || !download.Ready || downloadClient.calls != 1 || downloadClient.projectUUID != runtimeProjectUUID || downloadClient.fileID != strconv.FormatInt(poll.Models[0].ID, 10) {
		t.Fatalf("download=%#v client=%#v err=%v", download, downloadClient, err)
	}
	_, err = access.RefreshDownload(ctx, connector.Instance{ID: adapterID, ProjectID: otherProjectID}, assetID)
	if !errors.Is(err, connector.ErrRemoteResourceUnavailable) || downloadClient.calls != 1 {
		t.Fatalf("cross-project err=%v calls=%d", err, downloadClient.calls)
	}
}
