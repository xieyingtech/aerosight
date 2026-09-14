package flighthub

import (
	"aerosight/server/internal/algorithm"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"aerosight/server/internal/connector"
	"aerosight/server/internal/inspection"
	"aerosight/server/internal/migrations"
	"aerosight/server/internal/mission"
	"aerosight/server/internal/telemetry"
	"aerosight/server/internal/testdb"
)

func TestSQLFlightAssetsAreIdempotentProjectScopedAndRefreshExpiredURLs(t *testing.T) {
	databaseURL := os.Getenv("AEROSIGHT_TEST_DATABASE_URL")
	var database *sql.DB
	var err error
	ctx := context.Background()
	if databaseURL == "" {
		database = testdb.New(t)
		if _, err = migrations.Embedded(ctx, database); err != nil {
			t.Fatal(err)
		}
	} else {
		database, err = sql.Open("pgx", databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { database.Close() })
	}
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
	gatewayBody := []byte("remote image fixture")
	gatewayReads := 0
	client := testClient(t, roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Hostname() == "objects.vendor.example" {
			gatewayReads++
			if request.Header.Get("X-Project-Uuid") != "" || request.Header.Get("Authorization") != "" {
				t.Fatal("media request leaked credentials")
			}
			return response(http.StatusOK, gatewayBody, nil), nil
		}
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
	if _, err := service.RefreshInspectionDownload(ctx, instance, mediaAssetID, "wrong-flight"); !errors.Is(err, connector.ErrRemoteResourceUnavailable) || mediaCalls != 0 {
		t.Fatal("cross-flight request reached provider", err)
	}
	if _, err := service.RefreshInspectionDownload(ctx, instance, recordAssetID, taskUUID); !errors.Is(err, connector.ErrRemoteResourceUnavailable) || recordCalls != 0 {
		t.Fatal("record accepted as flight image", err)
	}
	mediaDownload, err := service.RefreshInspectionDownload(ctx, instance, mediaAssetID, taskUUID)
	if err != nil || mediaCalls != 2 || mediaDownload.URL == "" || !mediaDownload.ExpiresAt.Equal(clock.Add(10*time.Minute)) {
		t.Fatalf("refreshed media=%#v calls=%d err=%v", mediaDownload, mediaCalls, err)
	}
	recordDownload, err := service.RefreshDownload(ctx, instance, recordAssetID)
	if err != nil || recordCalls != 1 || recordDownload.URL == "" || !recordDownload.ExpiresAt.Equal(clock.Add(10*time.Minute)) {
		t.Fatalf("refreshed record=%#v calls=%d err=%v", recordDownload, recordCalls, err)
	}
	if _, err := service.RefreshInspectionVersionDownload(ctx, instance, mediaAssetID, taskUUID, "obsolete-version"); !IsSafeCode(err, "media_version_changed") {
		t.Fatal("refresh switched media version", err)
	}
	if _, err := service.RefreshInspectionVersionDownload(ctx, instance, mediaAssetID, taskUUID, inspectionMediaVersion(media)); err != nil {
		t.Fatal("same version refresh failed", err)
	}
	// Exercise the actual signed HTTP handler with encrypted remote references.
	signer := algorithm.NewAssetURLSigner(strings.Repeat("s", 32), "https://worker.example")
	sum := sha256.Sum256(gatewayBody)
	pinned, err := signer.IssuePinnedAssetURL(projectID, mediaAssetID, 1, hex.EncodeToString(sum[:]), time.Now().Add(time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	gateway := algorithm.NewAssetAccessHandler(database, nil, signer).WithRemoteReader(service.ReadAlgorithmAsset)
	gatewayServer := httptest.NewServer(gateway)
	defer gatewayServer.Close()
	serve := func(raw string, want int) {
		t.Helper()
		res, err := gatewayServer.Client().Get(strings.Replace(raw, "https://worker.example", gatewayServer.URL, 1))
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		body, err := io.ReadAll(res.Body)
		if err != nil {
			t.Fatal(err)
		}
		if res.StatusCode != want {
			t.Fatalf("gateway: got %d want %d: %s", res.StatusCode, want, body)
		}
		if want == http.StatusOK && string(body) != string(gatewayBody) {
			t.Fatalf("wrong remote bytes: %q", body)
		}
		if want != http.StatusOK && strings.Contains(string(body), "remote image") {
			t.Fatal("changed content disclosed")
		}
	}
	serve(pinned, http.StatusOK)
	gatewayBody = []byte("changed remote image fixture")
	serve(pinned, http.StatusConflict)
	beforeReads := gatewayReads
	wrongProject, _ := signer.IssuePinnedAssetURL(otherProjectID, mediaAssetID, 1, hex.EncodeToString(sum[:]), time.Now().Add(time.Minute))
	serve(wrongProject, http.StatusNotFound)
	wrongVersion, _ := signer.IssuePinnedAssetURL(projectID, mediaAssetID, 2, hex.EncodeToString(sum[:]), time.Now().Add(time.Minute))
	serve(wrongVersion, http.StatusNotFound)
	if gatewayReads != beforeReads {
		t.Fatal("invalid scope fetched remote bytes")
	}
	if _, handled, err := service.ReadAlgorithmAsset(ctx, projectID, recordAssetID, 1); !handled || err == nil {
		t.Fatal("flight record accepted as algorithm image")
	}
	if _, err := database.ExecContext(ctx, "update device_adapters set status='disabled' where id=$1", adapterID); err != nil {
		t.Fatal(err)
	}
	serve(pinned, http.StatusNotFound)
	if gatewayReads != beforeReads {
		t.Fatal("disabled connector fetched remote bytes")
	}
	if _, err := database.ExecContext(ctx, "update device_adapters set status='connected' where id=$1", adapterID); err != nil {
		t.Fatal(err)
	}
	// Exercise the existing-flight observer against projected resources and the
	// real encrypted-reference refresh. Remote task/hash responses are protocol fixtures.
	createID := func(query string, args ...any) int64 {
		t.Helper()
		var id int64
		if err := database.QueryRowContext(ctx, query, args...).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	businessTask := createID("insert into tasks(project_id,team_id,name,trigger_type,script) values($1,$2,'inspection','manual','typed-task-v2') returning id", projectID, teamID)
	version := createID("insert into task_versions(project_id,team_id,task_id,version,status,script,dsl_version) values($1,$2,$3,1,'published','typed-task-v2','aerosight/v2') returning id", projectID, teamID, businessTask)
	run := createID("insert into task_runs(project_id,team_id,task_id,task_version_id,trigger_source,status) values($1,$2,$3,$4,'manual','running') returning id", projectID, teamID, businessTask, version)
	fixture := &inspectionFlightReadFixture{Client: client, task: FlightTask{UUID: taskUUID, Status: "success", BeginAt: clock.Add(-time.Hour).Format(time.RFC3339Nano), EndAt: clock.Format(time.RFC3339Nano), FolderInfo: FlightTaskFolderInfo{CountsKnown: true, ExpectedFileCount: 1, UploadedFileCount: 1}}}
	observer := NewInspectionFlightObserver(fixture, service, tokenResolverFixture{token: "TOKEN_REDACTED"})
	for _, tc := range []struct {
		name           string
		known, confirm bool
		ids            []int64
		wantError      bool
	}{
		{name: "complete", known: true}, {name: "unknown counters", wantError: true},
		{name: "confirmed finite scope", confirm: true, ids: []int64{int64(mediaAssetID)}},
		{name: "confirmation needs explicit selection", confirm: true, wantError: true},
		{name: "wrong flight selection", known: true, confirm: true, ids: []int64{int64(recordAssetID)}, wantError: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture.task.FolderInfo.CountsKnown = tc.known
			tx, err := database.BeginTx(ctx, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer tx.Rollback()
			observation, err := observer.Observe(ctx, tx, mission.PreparedStep{ProjectID: projectID, TeamID: teamID, RunID: int(run), StepID: 1, UserID: 1}, inspection.ObserveInput{Mode: "existing-flight", ConnectorID: adapterID, FlightUUID: taskUUID, AssetIDs: tc.ids, ConfirmLimitedScope: tc.confirm, ScopeDescription: "明确的测试图片范围"})
			if tc.wantError {
				if err == nil {
					t.Fatal("incomplete or invalid input accepted")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			want := inspection.Complete
			if tc.confirm {
				want = inspection.Partial
			}
			if observation.Completeness != want || len(observation.Assets) != 1 || observation.Assets[0].ChecksumSHA256 == "" || observation.Assets[0].ObjectVersion == "" || *observation.Assets[0].SourceRunID != int64(targets[0].TaskRunID) {
				t.Fatalf("bad observation %+v", observation)
			}
			if tc.confirm && observation.LimitedScopeConfirmedBy == nil {
				t.Fatal("confirmation identity missing")
			}
		})
	}
	var leakedURLCount int
	if err := database.QueryRowContext(ctx, `select count(*) from assets where project_id=$1 and (metadata_json::text like '%auth_key=%' or storage_key like '%auth_key=%')`, projectID).Scan(&leakedURLCount); err != nil {
		t.Fatal(err)
	}
	if leakedURLCount != 0 {
		t.Fatal("temporary URL was persisted after refresh")
	}
}

// The real download transport has separate bounded-body/redirect/expiry tests.
type inspectionFlightReadFixture struct {
	*Client
	task FlightTask
}

func (f *inspectionFlightReadFixture) GetFlightTask(context.Context, string, string, string) (FlightTask, error) {
	return f.task, nil
}
func (f *inspectionFlightReadFixture) HashInspectionMedia(context.Context, TemporaryDownload, int64) (string, error) {
	sum := sha256.Sum256([]byte("image fixture"))
	return hex.EncodeToString(sum[:]), nil
}
