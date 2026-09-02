package flighthub

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/connector"
)

func geoFeature(geometryType, coordinates string) GeoJSONFeature {
	return GeoJSONFeature{
		Type: "Feature", Properties: json.RawMessage(`{"color":"#00ff00","clampToGround":true}`),
		Geometry: GeoJSONGeometry{Type: geometryType, Coordinates: json.RawMessage(coordinates)},
	}
}

func geoFlightArea(id string, updatedAt int64) FlightArea {
	return FlightArea{
		ID: id, Name: "synthetic area", Status: "enable", Type: "dfence",
		Content:  geoFeature("Polygon", `[[[0.25,0.125],[0.25,0.25],[0.125,0.25],[0.25,0.125]]]`),
		AreaHash: "hash-" + id, CreatedTime: updatedAt - 1000, UpdatedTime: updatedAt,
	}
}

func TestGeoJSONGeometryValidationIsTypeAware(t *testing.T) {
	valid := []GeoJSONFeature{
		geoFeature("Point", `[0.25,0.125,15]`),
		geoFeature("LineString", `[[0.25,0.125],[0.5,0.25]]`),
		geoFeature("Polyline", `[[0.25,0.125],[0.5,0.25]]`),
		geoFeature("Polygon", `[[[0.25,0.125],[0.5,0.125],[0.5,0.25],[0.25,0.125]]]`),
		geoFeature("Circle", `[[0.25,0.125],50]`),
	}
	for _, feature := range valid {
		feature := feature
		if !validateGeoJSONFeature(&feature) {
			t.Fatalf("valid %s geometry rejected: %s", feature.Geometry.Type, feature.Geometry.Coordinates)
		}
	}

	invalid := []GeoJSONFeature{
		geoFeature("Point", `[[0.25,0.125]]`),
		geoFeature("Point", `[181,0]`),
		geoFeature("LineString", `[[0.25,0.125]]`),
		geoFeature("Polyline", `[[0.25,0.125],[0.5,91]]`),
		geoFeature("Polygon", `[[[0.25,0.125],[0.5,0.125],[0.5,0.25],[0.125,0.25]]]`),
		geoFeature("Circle", `[[0.25,0.125],0]`),
		geoFeature("Circle", `[[181,0],50]`),
	}
	for _, feature := range invalid {
		feature := feature
		if validateGeoJSONFeature(&feature) {
			t.Fatalf("invalid %s geometry accepted: %s", feature.Geometry.Type, feature.Geometry.Coordinates)
		}
	}
	circle := geoFeature("Circle", `[[0.25,0.125],50]`)
	lineString := geoFeature("LineString", `[[0.25,0.125],[0.5,0.25]]`)
	if validateMapElementGeoJSONFeature(&circle) || validateFlightAreaGeoJSONFeature(&lineString) {
		t.Fatal("domain-specific geometry types were accepted by the wrong FlightHub schema")
	}
}

func geospatialCoordinatorForTest(t *testing.T, now time.Time, client *resourceClientFixture, sink *resourceSinkFixture) *ResourceStreamCoordinator {
	t.Helper()
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"},
		&resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}}, sink, ResourceStreamConfig{
			OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
			CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
		})
	if err != nil {
		t.Fatal(err)
	}
	return coordinator
}

func TestGeospatialStreamProjectsOnlyCompleteValidatedSnapshots(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 0, 0, 0, time.UTC)
	pageOne := make([]FlightArea, geospatialPageSize)
	for index := range pageOne {
		pageOne[index] = geoFlightArea(fmt.Sprintf("AREA_REDACTED_%02d", index), now.UnixMilli()+int64(index))
	}
	pageTwo := []FlightArea{geoFlightArea("AREA_REDACTED_20", now.UnixMilli()+20)}
	client := &resourceClientFixture{flightAreaPages: map[int]FlightAreaPage{
		1: {Pagination: Pagination{Page: 1, PageSize: geospatialPageSize, Total: 21}, List: pageOne},
		2: {Pagination: Pagination{Page: 2, PageSize: geospatialPageSize, Total: 21}, List: pageTwo},
	}}
	sink := &resourceSinkFixture{}
	coordinator := geospatialCoordinatorForTest(t, now, client, sink)
	cursor, _, err := coordinator.pollGeospatial(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if err != nil || cursor["complete"] != true || cursor["flightAreas"] != 21 || cursor["pages"] != 2 {
		t.Fatalf("complete geospatial cursor=%#v err=%v", cursor, err)
	}
	if len(sink.geospatialPolls) != 1 || !sink.geospatialPolls[0].FlightAreasComplete ||
		sink.geospatialPolls[0].MapElementsComplete || len(sink.geospatialPolls[0].FlightAreas) != 21 {
		t.Fatalf("complete geospatial projection=%#v", sink.geospatialPolls)
	}

	client.flightAreaErrors = map[int]error{2: &APIError{SafeCode: "upstream_unavailable", Retryable: true}}
	cursor, _, err = coordinator.pollGeospatial(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if !IsSafeCode(err, "upstream_unavailable") || cursor["complete"] != false || len(sink.geospatialPolls) != 1 {
		t.Fatalf("partial response advanced snapshot cursor=%#v polls=%d err=%v", cursor, len(sink.geospatialPolls), err)
	}
}

func airSenseWarning(now time.Time, deviceSN, icao string) DeviceAirSenseWarnings {
	return DeviceAirSenseWarnings{
		DeviceSN: deviceSN, Timestamp: now.Add(-time.Minute).UnixMilli(), Enabled: true,
		CapturedAt: now.Add(-time.Minute), ExpiresAt: now.Add(4 * time.Minute),
		Events: []AirSenseWarningEvent{{
			ICAO: icao, WarningLevel: 2, Latitude: 30.25, Longitude: 120.5, Altitude: 120,
			AltitudeType: 1, Heading: 90, RelativeAltitude: 20, VerticalTrend: 0, Distance: 240,
		}},
	}
}

func TestAirSenseRemoteResourcesAreScopedStableAndSecretFree(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 15, 0, 0, time.UTC)
	deviceSN, icao := "DOCK_AIRSENSE_SECRET", "ICAO_AIRSENSE_SECRET"
	devices := []connector.ManagedConnectorDevice{{DeviceID: 17, TeamID: 3, Serial: deviceSN}}
	warnings := []DeviceAirSenseWarnings{airSenseWarning(now, deviceSN, icao)}
	first, err := airSenseRemoteResources(devices, warnings)
	if err != nil || len(first) != 1 {
		t.Fatalf("AirSense resources=%#v err=%v", first, err)
	}
	second, err := airSenseRemoteResources(devices, warnings)
	if err != nil || first[0].RemoteID != second[0].RemoteID || first[0].RemoteVersion != second[0].RemoteVersion {
		t.Fatalf("AirSense identity/version is unstable: first=%#v second=%#v err=%v", first, second, err)
	}
	if first[0].Summary["deviceId"] != 17 || first[0].Summary["coordinateReference"] != "unverified" {
		t.Fatalf("AirSense managed mapping=%#v", first[0].Summary)
	}
	serialized, _ := json.Marshal(first)
	for _, secret := range []string{deviceSN, icao} {
		if strings.Contains(string(serialized), secret) {
			t.Fatalf("AirSense resource leaked %q: %s", secret, serialized)
		}
	}

	changed := append([]DeviceAirSenseWarnings(nil), warnings...)
	changed[0].Events = append([]AirSenseWarningEvent(nil), warnings[0].Events...)
	changed[0].Events[0].WarningLevel = 3
	updated, err := airSenseRemoteResources(devices, changed)
	if err != nil || updated[0].RemoteID != first[0].RemoteID || updated[0].RemoteVersion == first[0].RemoteVersion {
		t.Fatalf("AirSense update identity/version=%#v err=%v", updated, err)
	}

	duplicate := append([]DeviceAirSenseWarnings(nil), warnings...)
	duplicate = append(duplicate, warnings[0])
	if _, err := airSenseRemoteResources(devices, duplicate); err == nil {
		t.Fatal("duplicate AirSense identity was accepted")
	}
	outsideScope := append([]DeviceAirSenseWarnings(nil), warnings...)
	outsideScope[0].DeviceSN = "UNMANAGED_AIRSENSE_SECRET"
	if _, err := airSenseRemoteResources(devices, outsideScope); err == nil {
		t.Fatal("AirSense warning outside managed scope was accepted")
	}
	duplicateDevices := append([]connector.ManagedConnectorDevice(nil), devices...)
	duplicateDevices = append(duplicateDevices, connector.ManagedConnectorDevice{DeviceID: 18, TeamID: 3, Serial: deviceSN})
	if _, err := airSenseRemoteResources(duplicateDevices, warnings); err == nil {
		t.Fatal("duplicate managed AirSense device identity was accepted")
	}
}

func TestGeospatialStreamIsolatesAirSenseFailureFromFlightAreaSnapshot(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 20, 0, 0, time.UTC)
	deviceSN := "DOCK_AIRSENSE_SECRET"
	device := connector.ManagedConnectorDevice{DeviceID: 17, TeamID: 3, Serial: deviceSN}
	client := &resourceClientFixture{
		flightAreaPages: map[int]FlightAreaPage{1: {
			Pagination: Pagination{Page: 1, PageSize: geospatialPageSize, Total: 1},
			List:       []FlightArea{geoFlightArea("AREA_REDACTED_01", now.UnixMilli())},
		}},
		airSenseWarnings: []DeviceAirSenseWarnings{airSenseWarning(now, deviceSN, "ICAO_AIRSENSE_SECRET")},
	}
	store := &resourceStoreFixture{states: map[string]connector.ResourceSyncUpdate{}, devices: []connector.ManagedConnectorDevice{device}}
	sink := &resourceSinkFixture{}
	coordinator, err := NewResourceStreamCoordinator(client, tokenResolverFixture{token: "TOKEN_REDACTED"}, store, sink, ResourceStreamConfig{
		OnlineInterval: 15 * time.Second, OfflineInterval: time.Minute, HealthInterval: 5 * time.Minute,
		CatalogInterval: 15 * time.Minute, MaxBackoff: 5 * time.Minute, Now: func() time.Time { return now },
	})
	if err != nil {
		t.Fatal(err)
	}
	cursor, _, err := coordinator.pollGeospatial(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if err != nil || cursor["complete"] != true || cursor["airSenseWarnings"] != 1 || len(sink.geospatialPolls) != 1 ||
		!sink.geospatialPolls[0].AirSenseComplete || len(sink.geospatialPolls[0].AirSenseWarnings) != 1 {
		t.Fatalf("complete AirSense geospatial cursor=%#v polls=%#v err=%v", cursor, sink.geospatialPolls, err)
	}

	client.airSenseWarnings = nil
	client.airSenseError = &APIError{SafeCode: "upstream_unavailable", Retryable: true}
	cursor, _, err = coordinator.pollGeospatial(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
	if !IsSafeCode(err, "upstream_unavailable") || cursor["complete"] != false || cursor["flightAreasComplete"] != true ||
		cursor["airSenseComplete"] != false || len(sink.geospatialPolls) != 2 || !sink.geospatialPolls[1].FlightAreasComplete || sink.geospatialPolls[1].AirSenseComplete {
		t.Fatalf("AirSense failure erased flight-area boundary cursor=%#v polls=%#v err=%v", cursor, sink.geospatialPolls, err)
	}
}

func TestGeospatialStreamRejectsDriftDuplicatesAndInvalidGeometryBeforeMissing(t *testing.T) {
	now := time.Date(2026, 9, 2, 8, 30, 0, 0, time.UTC)
	fullPage := make([]FlightArea, geospatialPageSize)
	for index := range fullPage {
		fullPage[index] = geoFlightArea(fmt.Sprintf("AREA_REDACTED_%02d", index), now.UnixMilli()+int64(index))
	}
	cases := map[string]map[int]FlightAreaPage{
		"total drift": {
			1: {Pagination: Pagination{Page: 1, PageSize: geospatialPageSize, Total: 21}, List: fullPage},
			2: {Pagination: Pagination{Page: 2, PageSize: geospatialPageSize, Total: 22}, List: []FlightArea{geoFlightArea("AREA_REDACTED_20", now.UnixMilli())}},
		},
		"duplicate id": {
			1: {Pagination: Pagination{Page: 1, PageSize: geospatialPageSize, Total: 21}, List: fullPage},
			2: {Pagination: Pagination{Page: 2, PageSize: geospatialPageSize, Total: 21}, List: []FlightArea{geoFlightArea("AREA_REDACTED_00", now.UnixMilli())}},
		},
	}
	invalid := geoFlightArea("AREA_REDACTED_INVALID", now.UnixMilli())
	invalid.Content.Geometry.Coordinates = json.RawMessage(`[[[0.25,0.125],[0.5,0.125],[0.5,0.25],[0.125,0.25]]]`)
	cases["invalid geojson"] = map[int]FlightAreaPage{
		1: {Pagination: Pagination{Page: 1, PageSize: geospatialPageSize, Total: 1}, List: []FlightArea{invalid}},
	}

	for name, pages := range cases {
		t.Run(name, func(t *testing.T) {
			sink := &resourceSinkFixture{}
			coordinator := geospatialCoordinatorForTest(t, now, &resourceClientFixture{flightAreaPages: pages}, sink)
			cursor, _, err := coordinator.pollGeospatial(context.Background(), resourceStreamInstance(), "TOKEN_REDACTED")
			if err == nil || cursor["complete"] != false || len(sink.geospatialPolls) != 0 {
				t.Fatalf("invalid batch reached missing projection cursor=%#v polls=%d err=%v", cursor, len(sink.geospatialPolls), err)
			}
		})
	}
}

func TestGeospatialSinkProjectsVersionsAndNeverCompletesUnreadMapElements(t *testing.T) {
	resources := &remoteResourceWriterFixture{}
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, resources, &freshnessProjectorFixture{}, &healthProjectorFixture{}, &flightCatalogProjectorFixture{})
	if err != nil {
		t.Fatal(err)
	}
	now := time.Date(2026, 9, 2, 9, 0, 0, 0, time.UTC)
	area := geoFlightArea("AREA_REDACTED_01", now.UnixMilli())
	element := MapElementSnapshot{
		ID: "ELEMENT_REDACTED_01", Name: "synthetic point", Status: 1, Display: 1,
		Content: geoFeature("Point", `[0.25,0.125,15]`), RemoteVersion: "version-redacted-1", UpdatedTime: now.UnixMilli(),
	}
	poll := GeospatialCatalogPoll{
		MapElements: []MapElementSnapshot{element}, FlightAreas: []FlightArea{area},
		MapElementsComplete: false, FlightAreasComplete: true, ReceivedAt: now,
	}
	for iteration := 0; iteration < 2; iteration++ {
		if err := sink.ApplyGeospatialCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
			t.Fatal(err)
		}
	}
	if len(resources.batches) != 6 || resources.batches[0].CompleteSnapshot || !resources.batches[1].CompleteSnapshot ||
		resources.batches[2].CompleteSnapshot || resources.batches[5].CompleteSnapshot {
		t.Fatalf("geospatial snapshot flags=%#v", resources.batches)
	}
	firstElement, secondElement := resources.batches[0].Resources[0], resources.batches[3].Resources[0]
	firstArea, secondArea := resources.batches[1].Resources[0], resources.batches[4].Resources[0]
	if firstElement.RemoteVersion != "version-redacted-1" || firstArea.RemoteVersion != area.AreaHash ||
		firstElement.RemoteVersion != secondElement.RemoteVersion || firstArea.RemoteVersion != secondArea.RemoteVersion ||
		firstArea.RemoteUpdatedAt == nil || !firstArea.RemoteUpdatedAt.Equal(now) {
		t.Fatalf("geospatial remote versions are not stable: element=%#v/%#v area=%#v/%#v", firstElement, secondElement, firstArea, secondArea)
	}
	summaries := []map[string]any{firstElement.Summary, secondElement.Summary, firstArea.Summary, secondArea.Summary}
	serialized, _ := json.Marshal(summaries)
	for _, forbidden := range []string{area.ID, element.ID, area.CreatedBy, area.UpdatedBy, "http://", "https://"} {
		if forbidden != "" && strings.Contains(string(serialized), forbidden) {
			t.Fatalf("geospatial summary persisted forbidden value %q: %s", forbidden, serialized)
		}
	}

	invalid := poll
	invalid.FlightAreas[0].Content.Geometry.Coordinates = json.RawMessage(`[[]]`)
	before := len(resources.batches)
	if err := sink.ApplyGeospatialCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, invalid); err == nil || len(resources.batches) != before {
		t.Fatalf("invalid geometry reached complete/missing projection batches=%d err=%v", len(resources.batches), err)
	}
}

func TestGeospatialSinkNormalizesSuccessfulEmptyAirSenseSnapshot(t *testing.T) {
	resources := &remoteResourceWriterFixture{}
	projector := &flightCatalogProjectorFixture{}
	sink, err := NewSQLResourceStreamSink(&telemetryIngestorFixture{}, resources, &freshnessProjectorFixture{}, &healthProjectorFixture{}, projector)
	if err != nil {
		t.Fatal(err)
	}
	poll := GeospatialCatalogPoll{
		MapElements: []MapElementSnapshot{}, FlightAreas: []FlightArea{}, Devices: []connector.ManagedConnectorDevice{},
		AirSenseComplete: true, ReceivedAt: time.Date(2026, 9, 2, 9, 30, 0, 0, time.UTC),
	}
	if err := sink.ApplyGeospatialCatalog(context.Background(), connector.Instance{ID: 7, ProjectID: 3}, poll); err != nil {
		t.Fatal(err)
	}
	if len(resources.batches) != 3 || !resources.batches[2].CompleteSnapshot || len(projector.airSensePolls) != 1 ||
		projector.airSensePolls[0].Warnings == nil || !projector.airSensePolls[0].CompleteSnapshot {
		t.Fatalf("successful empty AirSense snapshot was not projected safely: batches=%#v polls=%#v", resources.batches, projector.airSensePolls)
	}
}
