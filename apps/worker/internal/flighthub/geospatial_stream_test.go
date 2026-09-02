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
	if len(resources.batches) != 4 || resources.batches[0].CompleteSnapshot || !resources.batches[1].CompleteSnapshot {
		t.Fatalf("geospatial snapshot flags=%#v", resources.batches)
	}
	firstElement, secondElement := resources.batches[0].Resources[0], resources.batches[2].Resources[0]
	firstArea, secondArea := resources.batches[1].Resources[0], resources.batches[3].Resources[0]
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
