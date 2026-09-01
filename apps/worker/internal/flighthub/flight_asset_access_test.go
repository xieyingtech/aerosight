package flighthub

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
	"aerosight/worker/internal/telemetry"
)

func TestSQLFlightAssetsAreIdempotentProjectScopedAndRefreshExpiredURLs(t *testing.T) {
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
	var teamID, projectID, otherProjectID, dockID int
	var adapterID, definitionID int64
	if err := database.QueryRowContext(ctx, `insert into teams(name) values($1) returning id`, fmt.Sprintf("flighthub-assets-%d", suffix)).Scan(&teamID); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = database.ExecContext(context.Background(), `delete from teams where id=$1`, teamID) })
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-assets-%d", suffix)).Scan(&projectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into projects(team_id,name) values($1,$2) returning id`, teamID, fmt.Sprintf("flighthub-assets-other-%d", suffix)).Scan(&otherProjectID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select id from connector_definitions where connector_key='dji.flighthub2' and version='1.0.0'`).Scan(&definitionID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into device_adapters(
		project_id,team_id,name,adapter_type,connector_definition_id,protocol_version,status,discovery_scope_json,credential_envelope_json
	) values($1,$2,$3,'dji-flighthub2',$4,'2','connected',$5,'{}'::jsonb) returning id`, projectID, teamID,
		fmt.Sprintf("flighthub-assets-%d", suffix), definitionID, fmt.Sprintf(`{"projectUuid":"%s","projectName":"脱敏项目"}`, runtimeProjectUUID)).Scan(&adapterID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `insert into devices(project_id,adapter_id,device_type_id,name,type,status)
		select $1,$2,id,'asset test dock','dock','unknown' from device_types
		 where type_key='dji.dock2' and status='active' order by version desc limit 1 returning id`, projectID, adapterID).Scan(&dockID); err != nil {
		t.Fatal(err)
	}
	dockSN := fmt.Sprintf("DOCK_ASSET_SECRET_%d", suffix)
	if _, err := database.ExecContext(ctx, `insert into device_external_identities(
		project_id,team_id,adapter_id,device_id,external_device_id,external_device_type,identity_json,discovery_status,bound_at
	) values($1,$2,$3,$4,$5,'dji.dock2',jsonb_build_object('attributes',jsonb_build_object('serialNumber',$6::text)),'managed',now())`,
		projectID, teamID, adapterID, dockID, secureRemoteKey(dockSN), dockSN); err != nil {
		t.Fatal(err)
	}

	clock := time.Date(2026, 9, 1, 10, 0, 0, 0, time.UTC)
	ingestor := telemetry.NewIngestor(database)
	projector := NewSQLFlightCatalogProjector(database, ingestor, func() time.Time { return clock }, 30*time.Minute, flightProjectorTestSecret)
	repository := connector.NewSQLResourceRepository(database)
	sink, err := NewSQLResourceStreamSink(ingestor, repository, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	instance := connector.Instance{ID: adapterID, ProjectID: projectID}
	waylineID := fmt.Sprintf("WAYLINE_ASSET_SECRET_%d", suffix)
	taskUUID := fmt.Sprintf("TASK_ASSET_SECRET_%d", suffix)
	if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "wayline", CompleteSnapshot: true, ReceivedAt: clock, Waylines: []WaylineSummary{{
		ID: waylineID, Name: "资产航线", DeviceModelKey: "0-91-1", TemplateTypes: []string{"waypoint"}, UpdatedAt: clock.UnixMilli(), SizeBytes: 100,
	}}}); err != nil {
		t.Fatal(err)
	}
	if err := sink.ApplyCatalog(ctx, instance, CatalogPoll{Kind: "flight-task", CompleteSnapshot: true, ReceivedAt: clock, FlightTasks: []FlightTaskSummary{{
		UUID: taskUUID, Name: "资产任务", TaskType: "immediate", Status: "success", SN: dockSN, WaylineUUID: waylineID,
		MediaUploadStatus: "uploaded", BeginAt: clock.Add(-time.Hour).Format(time.RFC3339Nano), CompletedAt: clock.Format(time.RFC3339Nano),
	}}}); err != nil {
		t.Fatal(err)
	}
	targets, err := projector.ListArtifactTargets(ctx, instance, 25)
	if err != nil || len(targets) != 1 || !targets[0].NeedMedia || !targets[0].MediaUploadFinal {
		t.Fatalf("asset targets=%#v err=%v", targets, err)
	}
	mediaUUID := fmt.Sprintf("MEDIA_ASSET_SECRET_%d", suffix)
	mediaURL := fmt.Sprintf("https://objects.vendor.example/media/%d?auth_key=URL_SECRET_%d", suffix, suffix)
	media := FlightTaskMedia{
		UUID: mediaUUID, Name: "脱敏媒体", FileType: "image", Suffix: "jpg", SizeBytes: 4096,
		PreviewURL: mediaURL, OriginalURL: mediaURL, CreatedAt: clock.Add(-time.Minute).Format(time.RFC3339Nano), UpdatedAt: clock.Format(time.RFC3339Nano),
	}
	mediaItems := []FlightTaskMedia{media, media}
	for iteration := 0; iteration < 2; iteration++ {
		if err := projector.ApplyFlightArtifacts(ctx, instance, FlightArtifactPoll{Target: targets[0], Media: &mediaItems, ReceivedAt: clock}); err != nil {
			t.Fatal(err)
		}
	}
	exportUUID := fmt.Sprintf("EXPORT_ASSET_SECRET_%d", suffix)
	objectKey := fmt.Sprintf("exports/PROJECT_SECRET_%d/record.csv", suffix)
	exportTime := clock.Format(time.RFC3339Nano)
	record := FlightExportRecord{
		UUID: exportUUID, CreatedAt: clock.Add(-time.Minute).Format(time.RFC3339Nano), ExportTime: &exportTime,
		ContentType: "details", Status: "export_complete", Progress: 100, FileName: "脱敏记录", FileTypes: []string{"CSV"}, ObjectKey: objectKey,
	}
	exportPoll := FlightExportPoll{Records: []FlightExportRecord{record, record}, CompleteSnapshot: true, ReceivedAt: clock}
	for iteration := 0; iteration < 2; iteration++ {
		if err := projector.ApplyFlightExports(ctx, instance, exportPoll); err != nil {
			t.Fatal(err)
		}
	}

	var assetCount, mediaResourceCount, recordResourceCount, referenceCount int
	if err := database.QueryRowContext(ctx, `select count(*) from assets where project_id=$1 and metadata_json->>'source'='dji-flighthub-openapi'`, projectID).Scan(&assetCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-media' and canonical_target_type='asset'`, projectID, adapterID).Scan(&mediaResourceCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_remote_resources where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-record' and canonical_target_type='asset'`, projectID, adapterID).Scan(&recordResourceCount); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select count(*) from connector_asset_access_refs where project_id=$1 and connector_instance_id=$2`, projectID, adapterID).Scan(&referenceCount); err != nil {
		t.Fatal(err)
	}
	if assetCount != 2 || mediaResourceCount != 1 || recordResourceCount != 1 || referenceCount != 2 {
		t.Fatalf("assets=%d media=%d records=%d references=%d", assetCount, mediaResourceCount, recordResourceCount, referenceCount)
	}
	var mediaAssetID, recordAssetID int
	if err := database.QueryRowContext(ctx, `select canonical_target_id::int from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-media' and remote_id=$3`, projectID, adapterID, mediaUUID).Scan(&mediaAssetID); err != nil {
		t.Fatal(err)
	}
	if err := database.QueryRowContext(ctx, `select canonical_target_id::int from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and resource_kind='flight-record' and remote_id=$3`, projectID, adapterID, exportUUID).Scan(&recordAssetID); err != nil {
		t.Fatal(err)
	}
	var persisted string
	if err := database.QueryRowContext(ctx, `select concat_ws(' ',
		coalesce((select string_agg(storage_key||' '||logical_key||' '||metadata_json::text,' ') from assets where project_id=$1),''),
		coalesce((select string_agg(summary_json::text,' ') from connector_remote_resources where project_id=$1 and resource_kind in('flight-media','flight-record')),''),
		coalesce((select string_agg(payload_json::text,' ') from project_events where project_id=$1),''),
		coalesce((select string_agg(credential_envelope_json::text,' ') from connector_asset_access_refs where project_id=$1),''))`, projectID).Scan(&persisted); err != nil {
		t.Fatal(err)
	}
	for _, secret := range []string{taskUUID, mediaUUID, exportUUID, objectKey, mediaURL} {
		if strings.Contains(persisted, secret) {
			t.Fatalf("asset projection leaked protected reference %q", secret)
		}
	}

	mediaCalls, recordCalls := 0, 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.Header.Get("X-Project-Uuid") == "" {
			t.Fatalf("unexpected refresh request %s", request.URL)
		}
		switch request.URL.Path {
		case "/openapi/v2.0/flight-task/" + taskUUID + "/media":
			mediaCalls++
			expires := clock.Add(-time.Minute).Unix()
			if mediaCalls > 1 {
				expires = clock.Add(10 * time.Minute).Unix()
			}
			body := fmt.Sprintf(`{"code":0,"message":"","data":{"list":[{"uuid":%q,"name":"脱敏媒体","file_type":"image","suffix":"jpg","size":4096,"preview_url":"","original_url":"https://objects.vendor.example/media?auth_key=%d-0-0-redacted","create_at":"2026-09-01T09:59:00Z","update_at":"2026-09-01T10:00:00Z"}]}}`, mediaUUID, expires)
			return response(http.StatusOK, []byte(body), nil), nil
		case "/openapi/v2.0/flight-task/oss-url-info/get":
			recordCalls++
			if request.URL.Query().Get("object_key") != objectKey {
				t.Fatalf("unexpected record object key")
			}
			body := fmt.Sprintf(`{"code":0,"message":"","data":"https://objects.vendor.example/record?auth_key=%d-0-0-redacted"}`, clock.Add(10*time.Minute).Unix())
			return response(http.StatusOK, []byte(body), nil), nil
		default:
			t.Fatalf("unexpected refresh request %s", request.URL)
			return nil, nil
		}
	}), func(config *Config) {
		config.Now = func() time.Time { return clock }
		config.AllowedLinkHosts = []string{"objects.vendor.example"}
	})
	service, err := NewFlightAssetAccessService(database, client, tokenResolverFixture{token: "TOKEN_REDACTED"}, flightProjectorTestSecret, func() time.Time { return clock })
	if err != nil {
		t.Fatal(err)
	}
	if _, err := service.RefreshDownload(ctx, connector.Instance{ID: adapterID, ProjectID: otherProjectID}, mediaAssetID); !errors.Is(err, connector.ErrRemoteResourceUnavailable) || mediaCalls != 0 || recordCalls != 0 {
		t.Fatalf("cross-project access err=%v mediaCalls=%d recordCalls=%d", err, mediaCalls, recordCalls)
	}
	mediaDownload, err := service.RefreshDownload(ctx, instance, mediaAssetID)
	if err != nil || mediaCalls != 2 || mediaDownload.URL == "" || !mediaDownload.ExpiresAt.Equal(clock.Add(10*time.Minute)) {
		t.Fatalf("refreshed media=%#v calls=%d err=%v", mediaDownload, mediaCalls, err)
	}
	recordDownload, err := service.RefreshDownload(ctx, instance, recordAssetID)
	if err != nil || recordCalls != 1 || recordDownload.URL == "" || !recordDownload.ExpiresAt.Equal(clock.Add(10*time.Minute)) {
		t.Fatalf("refreshed record=%#v calls=%d err=%v", recordDownload, recordCalls, err)
	}
	var leakedURLCount int
	if err := database.QueryRowContext(ctx, `select count(*) from assets where project_id=$1 and (metadata_json::text like '%auth_key=%' or storage_key like '%auth_key=%')`, projectID).Scan(&leakedURLCount); err != nil {
		t.Fatal(err)
	}
	if leakedURLCount != 0 {
		t.Fatal("temporary URL was persisted after refresh")
	}
}
