package flighthub

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

const maxGeospatialPages = 100

// MapElementSnapshot is an internal write-through projection. FlightHub's
// released contract does not expose a map-element list endpoint, so callers
// must never claim a complete map-element snapshot without equivalent evidence.
type MapElementSnapshot struct {
	ID            string
	Name          string
	Status        int
	Display       int
	Content       GeoJSONFeature
	RemoteVersion string
	UpdatedTime   int64
}

type GeospatialCatalogPoll struct {
	MapElements         []MapElementSnapshot
	FlightAreas         []FlightArea
	Devices             []connector.ManagedConnectorDevice
	AirSenseWarnings    []DeviceAirSenseWarnings
	OfflineMaps         []OfflineMap
	MapElementsComplete bool
	FlightAreasComplete bool
	AirSenseComplete    bool
	OfflineMapsComplete bool
	ReceivedAt          time.Time
}

type AirSensePoll struct {
	Devices          []connector.ManagedConnectorDevice
	Warnings         []DeviceAirSenseWarnings
	CompleteSnapshot bool
	ReceivedAt       time.Time
}

func (coordinator *ResourceStreamCoordinator) pollGeospatial(ctx context.Context, instance connector.Instance, token string) (map[string]any, time.Duration, error) {
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return nil, 0, err
	}
	areas, pages, err := coordinator.listAllProjectFlightAreas(ctx, token, scope.ProjectUUID)
	if err != nil {
		return map[string]any{"flightAreas": len(areas), "pages": pages, "flightAreasComplete": false, "mapElementsComplete": false, "complete": false}, 0, err
	}
	devices, err := coordinator.store.ListManagedDevices(ctx, instance)
	if err != nil {
		return map[string]any{"flightAreas": len(areas), "pages": pages, "flightAreasComplete": true, "airSenseComplete": false, "mapElementsComplete": false, "complete": false}, 0, err
	}
	warnings, warningErr := coordinator.client.ListWorkspaceAirSenseWarnings(ctx, token, scope.ProjectUUID)
	offlineMapDetails, offlineMapErr := coordinator.client.GetWorkspaceOfflineMap(ctx, token, scope.ProjectUUID)
	offlineMaps := []OfflineMap{}
	if offlineMapErr == nil && offlineMapDetails.OfflineMap != nil {
		offlineMaps = append(offlineMaps, *offlineMapDetails.OfflineMap)
	}
	poll := GeospatialCatalogPoll{
		FlightAreas: areas, FlightAreasComplete: true,
		Devices: devices, AirSenseWarnings: warnings, AirSenseComplete: warningErr == nil,
		OfflineMaps: offlineMaps, OfflineMapsComplete: offlineMapErr == nil,
		// There is no released GET/list contract for map elements. Keeping this
		// false prevents a flight-area read from erasing write-through elements.
		MapElementsComplete: false,
		ReceivedAt:          coordinator.config.Now().UTC(),
	}
	if err := coordinator.sink.ApplyGeospatialCatalog(ctx, instance, poll); err != nil {
		return map[string]any{"flightAreas": len(areas), "pages": pages, "airSenseWarnings": len(warnings), "offlineMaps": len(offlineMaps), "flightAreasComplete": false, "airSenseComplete": false, "offlineMapsComplete": false, "mapElementsComplete": false, "complete": false}, 0, errors.Join(warningErr, offlineMapErr, err)
	}
	cursor := map[string]any{
		"flightAreas": len(areas), "pages": pages, "flightAreasComplete": true,
		"airSenseWarnings": len(warnings), "airSenseComplete": warningErr == nil,
		"offlineMaps": len(offlineMaps), "offlineMapsComplete": offlineMapErr == nil,
		"mapElementsComplete": false, "complete": warningErr == nil && offlineMapErr == nil,
	}
	return cursor, coordinator.config.CatalogInterval, errors.Join(warningErr, offlineMapErr)
}

func (coordinator *ResourceStreamCoordinator) listAllProjectFlightAreas(ctx context.Context, token, projectUUID string) ([]FlightArea, int, error) {
	items := make([]FlightArea, 0)
	seen := make(map[string]struct{})
	total := -1
	for page := 1; page <= maxGeospatialPages; page++ {
		result, err := coordinator.client.ListProjectFlightAreas(ctx, token, projectUUID, FlightAreaListOptions{
			PageOptions: PageOptions{Page: page, PageSize: geospatialPageSize},
		})
		if err != nil {
			return items, page - 1, err
		}
		if result.Pagination.Page != page || result.Pagination.PageSize != geospatialPageSize {
			return items, page - 1, schemaError()
		}
		if total < 0 {
			total = result.Pagination.Total
		} else if total != result.Pagination.Total {
			return items, page - 1, schemaError()
		}
		for index := range result.List {
			item := result.List[index]
			if !validateFlightArea(&item) {
				return items, page - 1, schemaError()
			}
			if _, duplicate := seen[item.ID]; duplicate {
				return items, page - 1, schemaError()
			}
			seen[item.ID] = struct{}{}
			items = append(items, item)
		}
		if len(items) == total {
			return items, page, nil
		}
		if len(items) > total || len(result.List) == 0 || len(result.List) < geospatialPageSize {
			return items, page, schemaError()
		}
	}
	return items, maxGeospatialPages, &APIError{SafeCode: "snapshot_incomplete", Retryable: true}
}

func (sink *SQLResourceStreamSink) ApplyGeospatialCatalog(ctx context.Context, instance connector.Instance, poll GeospatialCatalogPoll) error {
	if err := sink.resources.AssertWritable(ctx, instance); err != nil {
		return err
	}
	if instance.ID <= 0 || instance.ProjectID <= 0 || poll.ReceivedAt.IsZero() {
		return errors.New("FlightHub geospatial catalog projection scope is invalid")
	}
	if poll.AirSenseComplete && poll.AirSenseWarnings == nil {
		poll.AirSenseWarnings = []DeviceAirSenseWarnings{}
	}
	elements, err := mapElementRemoteResources(poll.MapElements)
	if err != nil {
		return err
	}
	areas, err := flightAreaRemoteResources(poll.FlightAreas)
	if err != nil {
		return err
	}
	airSense, err := airSenseRemoteResources(poll.Devices, poll.AirSenseWarnings)
	if err != nil {
		return err
	}
	offlineMaps, err := offlineMapRemoteResources(poll.OfflineMaps)
	if err != nil {
		return err
	}
	batches := []connector.RemoteResourceBatch{
		{Kind: "map-element", Resources: elements, CompleteSnapshot: poll.MapElementsComplete},
		{Kind: "flight-area", Resources: areas, CompleteSnapshot: poll.FlightAreasComplete},
		{Kind: "air-sense-warning", Resources: airSense, CompleteSnapshot: poll.AirSenseComplete},
		{Kind: "offline-map", Resources: offlineMaps, CompleteSnapshot: poll.OfflineMapsComplete},
	}
	var applyErrors []error
	for _, batch := range batches {
		if _, applyErr := sink.resources.ApplyRemoteResources(ctx, instance, batch); applyErr != nil {
			applyErrors = append(applyErrors, applyErr)
		}
	}
	if len(applyErrors) == 0 && poll.AirSenseWarnings != nil {
		if applyErr := sink.flights.ApplyAirSense(ctx, instance, AirSensePoll{
			Devices: poll.Devices, Warnings: poll.AirSenseWarnings, CompleteSnapshot: poll.AirSenseComplete, ReceivedAt: poll.ReceivedAt,
		}); applyErr != nil {
			applyErrors = append(applyErrors, applyErr)
		}
	}
	return errors.Join(applyErrors...)
}

func offlineMapRemoteResources(items []OfflineMap) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	seen := make(map[int64]struct{}, len(items))
	for index := range items {
		item := items[index]
		if !validateOfflineMap(&item) {
			return nil, schemaError()
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, schemaError()
		}
		seen[item.ID] = struct{}{}
		version, err := alertDigest(map[string]any{
			"updatedTime": item.UpdatedTime, "percent": item.Percent, "status": item.Status,
			"result": item.Result, "models": item.Models,
		})
		if err != nil {
			return nil, err
		}
		updatedAt := time.UnixMilli(item.UpdatedTime).UTC()
		modelNames := make([]string, 0, len(item.Models))
		for _, model := range item.Models {
			modelNames = append(modelNames, model.Name)
		}
		resources = append(resources, connector.RemoteResource{
			RemoteID: strconv.FormatInt(item.ID, 10), RemoteVersion: version, RemoteUpdatedAt: &updatedAt,
			Summary: map[string]any{"status": item.Status, "result": item.Result, "percent": item.Percent,
				"modelCount": len(item.Models), "modelNames": modelNames, "source": "dji-flighthub-openapi"},
		})
	}
	return resources, nil
}

func airSenseRemoteID(deviceSN, icao string) string {
	return secureRemoteKey(strings.TrimSpace(deviceSN) + ":" + strings.TrimSpace(icao))
}

func airSenseRemoteResources(devices []connector.ManagedConnectorDevice, warnings []DeviceAirSenseWarnings) ([]connector.RemoteResource, error) {
	bySerial := make(map[string]int, len(devices))
	for _, device := range devices {
		if device.DeviceID <= 0 || strings.TrimSpace(device.Serial) == "" {
			return nil, schemaError()
		}
		if _, duplicate := bySerial[device.Serial]; duplicate {
			return nil, schemaError()
		}
		bySerial[device.Serial] = device.DeviceID
	}
	resources := make([]connector.RemoteResource, 0)
	seen := make(map[string]struct{})
	for warningIndex := range warnings {
		warning := warnings[warningIndex]
		deviceID := bySerial[warning.DeviceSN]
		if deviceID <= 0 || warning.CapturedAt.IsZero() || !warning.ExpiresAt.After(warning.CapturedAt) {
			return nil, errors.New("FlightHub AirSense warning is outside managed scope")
		}
		for eventIndex := range warning.Events {
			event := warning.Events[eventIndex]
			if !validateAirSenseEvent(&event) {
				return nil, schemaError()
			}
			remoteID := airSenseRemoteID(warning.DeviceSN, event.ICAO)
			if _, duplicate := seen[remoteID]; duplicate {
				return nil, schemaError()
			}
			seen[remoteID] = struct{}{}
			version, err := alertDigest(map[string]any{
				"capturedAt": warning.CapturedAt.UnixMilli(), "enabled": warning.Enabled, "event": event,
			})
			if err != nil {
				return nil, err
			}
			capturedAt := warning.CapturedAt.UTC()
			resources = append(resources, connector.RemoteResource{
				RemoteID: remoteID, RemoteVersion: version, RemoteUpdatedAt: &capturedAt,
				Summary: map[string]any{
					"deviceId": deviceID, "warningLevel": event.WarningLevel,
					"latitude": event.Latitude, "longitude": event.Longitude, "altitude": event.Altitude,
					"heading": event.Heading, "relativeAltitude": event.RelativeAltitude,
					"verticalTrend": event.VerticalTrend, "distance": event.Distance,
					"capturedAt": warning.CapturedAt, "expiresAt": warning.ExpiresAt,
					"expired": warning.Expired, "coordinateReference": "unverified",
				},
			})
		}
	}
	return resources, nil
}

func mapElementRemoteResources(items []MapElementSnapshot) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index := range items {
		item := items[index]
		if !validGeospatialString(item.ID, 256, false) || !validGeospatialString(item.Name, 256, false) ||
			item.Status < 0 || item.Display < 0 || item.Display > 1 ||
			!validGeospatialString(item.RemoteVersion, 256, false) || item.UpdatedTime <= 0 ||
			!validateMapElementGeoJSONFeature(&item.Content) {
			return nil, schemaError()
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, schemaError()
		}
		seen[item.ID] = struct{}{}
		updatedAt := time.UnixMilli(item.UpdatedTime).UTC()
		resources = append(resources, connector.RemoteResource{
			RemoteID: item.ID, RemoteVersion: item.RemoteVersion, RemoteUpdatedAt: &updatedAt,
			Summary: map[string]any{
				"name": item.Name, "status": item.Status, "display": item.Display,
				"geometry": item.Content.Geometry, "properties": safeGeoJSONProperties(item.Content.Properties),
				"coordinateReference": "unverified", "updatedTime": item.UpdatedTime,
			},
		})
	}
	return resources, nil
}

func flightAreaRemoteResources(items []FlightArea) ([]connector.RemoteResource, error) {
	resources := make([]connector.RemoteResource, 0, len(items))
	seen := make(map[string]struct{}, len(items))
	for index := range items {
		item := items[index]
		if !validateFlightArea(&item) {
			return nil, schemaError()
		}
		if _, duplicate := seen[item.ID]; duplicate {
			return nil, schemaError()
		}
		seen[item.ID] = struct{}{}
		updatedAt := time.UnixMilli(item.UpdatedTime).UTC()
		resources = append(resources, connector.RemoteResource{
			RemoteID: item.ID, RemoteVersion: item.AreaHash, RemoteUpdatedAt: &updatedAt,
			Summary: map[string]any{
				"name": item.Name, "status": item.Status, "areaType": item.Type,
				"geometry": item.Content.Geometry, "properties": safeGeoJSONProperties(item.Content.Properties),
				"coordinateReference": "unverified", "createdTime": item.CreatedTime, "updatedTime": item.UpdatedTime,
			},
		})
	}
	return resources, nil
}

func safeGeoJSONProperties(raw json.RawMessage) map[string]any {
	properties := map[string]any{}
	if len(raw) == 0 || string(raw) == "null" {
		return properties
	}
	var source map[string]any
	if json.Unmarshal(raw, &source) != nil {
		return properties
	}
	for _, key := range []string{"color", "fillColor", "clampToGround", "opacity", "width", "radius", "radius_m", "radiusMeters"} {
		if value, ok := source[key]; ok {
			properties[key] = value
		}
	}
	return properties
}
