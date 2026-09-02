package flighthub

import (
	"context"
	"encoding/json"
	"math"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

const (
	geospatialPageSize       = 20
	maxGeospatialStringBytes = 4096
	airSenseWarningTTL       = 5 * time.Minute
)

type GeoJSONGeometry struct {
	Type        string          `json:"type"`
	Coordinates json.RawMessage `json:"coordinates"`
}

type GeoJSONFeature struct {
	Type       string          `json:"type"`
	Properties json.RawMessage `json:"properties"`
	Geometry   GeoJSONGeometry `json:"geometry"`
}

type MapElementResource struct {
	Type    int            `json:"type"`
	Remark  string         `json:"remark,omitempty"`
	Content GeoJSONFeature `json:"content"`
}

type MapElementCreateRequest struct {
	ID          string             `json:"id,omitempty"`
	Name        string             `json:"name"`
	Source      *int               `json:"source,omitempty"`
	Description string             `json:"desc,omitempty"`
	Resource    MapElementResource `json:"resource"`
}

type MapElementUpdateRequest struct {
	Name                *string         `json:"name,omitempty"`
	Status              *int            `json:"status,omitempty"`
	Display             *int            `json:"display,omitempty"`
	Content             *GeoJSONFeature `json:"content,omitempty"`
	Remark              *string         `json:"remark,omitempty"`
	ElevationLoadStatus *int            `json:"elevation_load_status,omitempty"`
	TargetLayerID       *string         `json:"target_layer_id,omitempty"`
}

type MapElementMutationResult struct {
	ID string `json:"id"`
}

type MapElementTriState struct {
	GroupID  string `json:"group_id"`
	TriState string `json:"tri_state"`
}

type MapElementDeleteResult struct {
	ID                string               `json:"id"`
	AffectedTriStates []MapElementTriState `json:"affected_tri_states"`
}

type FlightAreaListOptions struct {
	PageOptions
	Name   string
	Type   string
	Status string
}

type FlightArea struct {
	ID              string         `json:"id"`
	Name            string         `json:"name"`
	Status          string         `json:"status"`
	Type            string         `json:"type"`
	Content         GeoJSONFeature `json:"content"`
	AreaHash        string         `json:"area_hash"`
	CreatedTime     int64          `json:"created_time"`
	CreatedBy       string         `json:"created_by"`
	CreatedNickname string         `json:"created_nickname"`
	UpdatedTime     int64          `json:"updated_time"`
	UpdatedBy       string         `json:"updated_by"`
	UpdatedNickname string         `json:"updated_nickname"`
}

type FlightAreaPage struct {
	Pagination Pagination   `json:"pagination"`
	List       []FlightArea `json:"list"`
}

type GeospatialFileDownload struct {
	Name      string    `json:"name"`
	URL       string    `json:"url"`
	Checksum  string    `json:"checksum"`
	Size      int64     `json:"size"`
	ExpiresAt time.Time `json:"-"`
}

type OfflineMapDownload struct {
	Enabled bool                    `json:"offline_map_enable"`
	File    *GeospatialFileDownload `json:"file"`
}

type OfflineMapModel struct {
	ID   int64  `json:"id"`
	Name string `json:"name"`
}

type OfflineMap struct {
	ID          int64             `json:"id"`
	UpdatedTime int64             `json:"updated_time"`
	Models      []OfflineMapModel `json:"models"`
	Percent     float64           `json:"percent"`
	Status      string            `json:"status"`
	Result      int               `json:"result"`
}

type OfflineMapDetails struct {
	Disabled   bool        `json:"disable"`
	OfflineMap *OfflineMap `json:"offline_map"`
}

type AirSenseWarningEvent struct {
	ICAO             string  `json:"icao"`
	WarningLevel     int     `json:"warning_level"`
	Latitude         float64 `json:"latitude"`
	Longitude        float64 `json:"longitude"`
	Altitude         int     `json:"altitude"`
	AltitudeType     int     `json:"altitude_type"`
	Heading          float64 `json:"heading"`
	RelativeAltitude float64 `json:"relative_altitude"`
	VerticalTrend    int     `json:"vert_trend"`
	Distance         float64 `json:"distance"`
}

type DeviceAirSenseWarnings struct {
	DeviceSN   string                 `json:"sn"`
	Timestamp  int64                  `json:"timestamp"`
	Enabled    bool                   `json:"enable_waring"`
	Events     []AirSenseWarningEvent `json:"waring_events"`
	CapturedAt time.Time              `json:"-"`
	ExpiresAt  time.Time              `json:"-"`
	Expired    bool                   `json:"-"`
}

func validGeospatialString(value string, maximum int, allowEmpty bool) bool {
	if value != strings.TrimSpace(value) || len(value) > maximum || strings.ContainsAny(value, "\x00\r\n") {
		return false
	}
	return allowEmpty || value != ""
}

func validFinite(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0)
}

func validGeoJSONCoordinateTree(raw json.RawMessage) bool {
	if len(raw) == 0 || len(raw) > 64<<10 {
		return false
	}
	var value any
	if json.Unmarshal(raw, &value) != nil {
		return false
	}
	count := 0
	var validate func(any, int) bool
	validate = func(item any, depth int) bool {
		if depth > 8 || count > 10000 {
			return false
		}
		switch typed := item.(type) {
		case float64:
			count++
			return validFinite(typed)
		case []any:
			if len(typed) == 0 {
				return false
			}
			for _, child := range typed {
				if !validate(child, depth+1) {
					return false
				}
			}
			return true
		default:
			return false
		}
	}
	return validate(value, 0) && count >= 2
}

func geoJSONPosition(value any) ([]float64, bool) {
	values, ok := value.([]any)
	if !ok || len(values) < 2 || len(values) > 3 {
		return nil, false
	}
	position := make([]float64, len(values))
	for index, value := range values {
		number, ok := value.(float64)
		if !ok || !validFinite(number) {
			return nil, false
		}
		position[index] = number
	}
	if position[0] < -180 || position[0] > 180 || position[1] < -90 || position[1] > 90 {
		return nil, false
	}
	return position, true
}

func sameGeoJSONPosition(left, right []float64) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func geoJSONLine(value any, minimum int) bool {
	positions, ok := value.([]any)
	if !ok || len(positions) < minimum || len(positions) > 10000 {
		return false
	}
	for _, value := range positions {
		if _, valid := geoJSONPosition(value); !valid {
			return false
		}
	}
	return true
}

func geoJSONPolygon(value any) bool {
	rings, ok := value.([]any)
	if !ok || len(rings) == 0 || len(rings) > 1024 {
		return false
	}
	positionCount := 0
	for _, value := range rings {
		ring, ok := value.([]any)
		if !ok || len(ring) < 4 {
			return false
		}
		positionCount += len(ring)
		if positionCount > 10000 {
			return false
		}
		first, valid := geoJSONPosition(ring[0])
		if !valid {
			return false
		}
		for _, point := range ring[1:] {
			if _, valid = geoJSONPosition(point); !valid {
				return false
			}
		}
		last, _ := geoJSONPosition(ring[len(ring)-1])
		if !sameGeoJSONPosition(first, last) {
			return false
		}
	}
	return true
}

func positiveGeoJSONProperty(properties map[string]json.RawMessage, names ...string) bool {
	for _, name := range names {
		var value float64
		if raw := properties[name]; len(raw) > 0 && json.Unmarshal(raw, &value) == nil && validFinite(value) && value > 0 {
			return true
		}
	}
	return false
}

func geoJSONCircle(value any, properties map[string]json.RawMessage) bool {
	coordinates, ok := value.([]any)
	if !ok {
		return false
	}
	// FlightHub deployments have returned both a GeoJSON-like center with a
	// radius property and the legacy [center, radius] tuple. Validate both
	// documented response shapes without accepting an unbounded coordinate tree.
	if len(coordinates) == 2 {
		if _, valid := geoJSONPosition(coordinates[0]); valid {
			radius, ok := coordinates[1].(float64)
			return ok && validFinite(radius) && radius > 0
		}
		if _, valid := geoJSONPosition(coordinates); valid {
			return positiveGeoJSONProperty(properties, "radius", "radius_m", "radiusMeters")
		}
	}
	if len(coordinates) == 3 {
		longitude, lonOK := coordinates[0].(float64)
		latitude, latOK := coordinates[1].(float64)
		radius, radiusOK := coordinates[2].(float64)
		return lonOK && latOK && radiusOK && validFinite(radius) && radius > 0 &&
			longitude >= -180 && longitude <= 180 && latitude >= -90 && latitude <= 90
	}
	return false
}

func validateGeoJSONFeature(feature *GeoJSONFeature) bool {
	if feature == nil || feature.Type != "Feature" ||
		!validEnum(feature.Geometry.Type, "Point", "LineString", "Polygon", "Circle", "Polyline") ||
		feature.Geometry.Type == "" || !validGeoJSONCoordinateTree(feature.Geometry.Coordinates) {
		return false
	}
	if len(feature.Properties) > 32<<10 {
		return false
	}
	if len(feature.Properties) == 0 || string(feature.Properties) == "null" {
		return true
	}
	var properties map[string]json.RawMessage
	if json.Unmarshal(feature.Properties, &properties) != nil || properties == nil || len(properties) > 128 {
		return false
	}
	for key := range properties {
		if !validGeospatialString(key, 128, false) {
			return false
		}
	}
	var coordinates any
	if json.Unmarshal(feature.Geometry.Coordinates, &coordinates) != nil {
		return false
	}
	switch feature.Geometry.Type {
	case "Point":
		_, valid := geoJSONPosition(coordinates)
		return valid
	case "LineString", "Polyline":
		return geoJSONLine(coordinates, 2)
	case "Polygon":
		return geoJSONPolygon(coordinates)
	case "Circle":
		return geoJSONCircle(coordinates, properties)
	default:
		return false
	}
}

func validateMapElementGeoJSONFeature(feature *GeoJSONFeature) bool {
	return feature != nil && validEnum(feature.Geometry.Type, "Point", "LineString", "Polygon") &&
		feature.Geometry.Type != "" && validateGeoJSONFeature(feature)
}

func validateFlightAreaGeoJSONFeature(feature *GeoJSONFeature) bool {
	return feature != nil && validEnum(feature.Geometry.Type, "Point", "Polyline", "Polygon", "Circle") &&
		feature.Geometry.Type != "" && validateGeoJSONFeature(feature)
}

func validateMapElementCreate(input *MapElementCreateRequest) error {
	if input == nil || !validGeospatialString(input.ID, 256, true) ||
		!validGeospatialString(input.Name, 256, false) ||
		!validGeospatialString(input.Description, maxGeospatialStringBytes, true) ||
		!validGeospatialString(input.Resource.Remark, maxGeospatialStringBytes, true) ||
		input.Resource.Type < 0 || input.Resource.Type > 2 || !validateMapElementGeoJSONFeature(&input.Resource.Content) {
		return &APIError{SafeCode: "request_invalid"}
	}
	if input.Source != nil && (*input.Source < 0 || *input.Source > 1) {
		return &APIError{SafeCode: "request_invalid"}
	}
	return nil
}

func validateMapElementUpdate(input *MapElementUpdateRequest) error {
	if input == nil || (input.Name == nil && input.Status == nil && input.Display == nil && input.Content == nil &&
		input.Remark == nil && input.ElevationLoadStatus == nil && input.TargetLayerID == nil) {
		return &APIError{SafeCode: "request_invalid"}
	}
	if input.Name != nil && !validGeospatialString(*input.Name, 256, false) {
		return &APIError{SafeCode: "request_invalid"}
	}
	if input.Remark != nil && !validGeospatialString(*input.Remark, maxGeospatialStringBytes, true) {
		return &APIError{SafeCode: "request_invalid"}
	}
	if input.TargetLayerID != nil && !validGeospatialString(*input.TargetLayerID, 256, false) {
		return &APIError{SafeCode: "request_invalid"}
	}
	if input.Status != nil && *input.Status < 0 || input.Display != nil && (*input.Display < 0 || *input.Display > 1) ||
		input.ElevationLoadStatus != nil && *input.ElevationLoadStatus < 0 ||
		input.Content != nil && !validateMapElementGeoJSONFeature(input.Content) {
		return &APIError{SafeCode: "request_invalid"}
	}
	return nil
}

func (client *Client) CreateMapElement(ctx context.Context, token, projectUUID string, input MapElementCreateRequest) (MapElementMutationResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return MapElementMutationResult{}, err
	}
	if err := validateMapElementCreate(&input); err != nil {
		return MapElementMutationResult{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{
		Method: http.MethodPost, Path: "/openapi/v2.0/map/element", Body: input, DisableRetry: true,
	})
	if err != nil {
		return MapElementMutationResult{}, err
	}
	var result MapElementMutationResult
	if json.Unmarshal(payload.Data, &result) != nil || !validGeospatialString(result.ID, 256, false) {
		return MapElementMutationResult{}, schemaError()
	}
	return result, nil
}

func geospatialWorkspacePath(template, workspaceID, elementID string) (string, error) {
	parameters := map[string]string{"workspace_id": workspaceID}
	if strings.Contains(template, "{id}") {
		parameters["id"] = elementID
	}
	return resolvePathTemplate(template, parameters)
}

func (client *Client) UpdateWorkspaceMapElement(ctx context.Context, token, workspaceID, elementID string, input MapElementUpdateRequest) (MapElementMutationResult, error) {
	path, err := geospatialWorkspacePath("/openapi/v2.0/workspaces/{workspace_id}/elements/{id}", workspaceID, elementID)
	if err != nil {
		return MapElementMutationResult{}, err
	}
	if err := validateMapElementUpdate(&input); err != nil {
		return MapElementMutationResult{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodPut, Path: path, Body: input, DisableRetry: true})
	if err != nil {
		return MapElementMutationResult{}, err
	}
	var result MapElementMutationResult
	if json.Unmarshal(payload.Data, &result) != nil || !validGeospatialString(result.ID, 256, false) {
		return MapElementMutationResult{}, schemaError()
	}
	return result, nil
}

func (client *Client) DeleteWorkspaceMapElement(ctx context.Context, token, projectUUID, workspaceID, elementID string) (MapElementDeleteResult, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return MapElementDeleteResult{}, err
	}
	path, err := geospatialWorkspacePath("/openapi/v2.0/workspaces/{workspace_id}/elements/{id}", workspaceID, elementID)
	if err != nil {
		return MapElementDeleteResult{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodDelete, Path: path, DisableRetry: true})
	if err != nil {
		return MapElementDeleteResult{}, err
	}
	var result MapElementDeleteResult
	if json.Unmarshal(payload.Data, &result) != nil || result.AffectedTriStates == nil {
		return MapElementDeleteResult{}, schemaError()
	}
	for _, state := range result.AffectedTriStates {
		if !validGeospatialString(state.GroupID, 256, false) || !validEnum(state.TriState, "all", "half", "none") || state.TriState == "" {
			return MapElementDeleteResult{}, schemaError()
		}
	}
	if result.ID != "" && !validGeospatialString(result.ID, 256, false) {
		return MapElementDeleteResult{}, schemaError()
	}
	return result, nil
}

func flightAreaQuery(options FlightAreaListOptions) (url.Values, error) {
	query, err := pageQuery(options.PageOptions)
	if err != nil {
		return nil, err
	}
	if err := addOptionalQuery(query, "name", options.Name); err != nil ||
		!validEnum(options.Type, "dfence", "nfz", "noland") ||
		!validEnum(options.Status, "enable", "disable") {
		return nil, &APIError{SafeCode: "request_invalid"}
	}
	if options.Type != "" {
		query.Set("type", options.Type)
	}
	if options.Status != "" {
		query.Set("status", options.Status)
	}
	return query, nil
}

func validateFlightArea(item *FlightArea) bool {
	if item == nil || !validGeospatialString(item.ID, 256, false) || !validGeospatialString(item.Name, 256, false) ||
		!validEnum(item.Status, "enable", "disable") || item.Status == "" ||
		!validEnum(item.Type, "dfence", "nfz", "noland") || item.Type == "" ||
		!validGeospatialString(item.AreaHash, 256, false) || item.CreatedTime <= 0 || item.UpdatedTime <= 0 ||
		!validGeospatialString(item.CreatedBy, 256, true) || !validGeospatialString(item.CreatedNickname, 256, true) ||
		!validGeospatialString(item.UpdatedBy, 256, true) || !validGeospatialString(item.UpdatedNickname, 256, true) {
		return false
	}
	return validateFlightAreaGeoJSONFeature(&item.Content)
}

func decodeFlightAreaPage(payload envelope, expected PageOptions) (FlightAreaPage, error) {
	var raw struct {
		List       json.RawMessage `json:"list"`
		Pagination *Pagination     `json:"pagination"`
	}
	if json.Unmarshal(payload.Data, &raw) != nil || raw.List == nil || raw.Pagination == nil ||
		raw.Pagination.Page != expected.Page || raw.Pagination.PageSize != expected.PageSize ||
		raw.Pagination.Total < 0 {
		return FlightAreaPage{}, schemaError()
	}
	items := []FlightArea{}
	if string(raw.List) != "null" {
		if json.Unmarshal(raw.List, &items) != nil || items == nil || len(items) > expected.PageSize {
			return FlightAreaPage{}, schemaError()
		}
	}
	for index := range items {
		if !validateFlightArea(&items[index]) {
			return FlightAreaPage{}, schemaError()
		}
	}
	return FlightAreaPage{Pagination: *raw.Pagination, List: items}, nil
}

func normalizedFlightAreaOptions(options FlightAreaListOptions) (FlightAreaListOptions, url.Values, error) {
	query, err := flightAreaQuery(options)
	if err != nil {
		return FlightAreaListOptions{}, nil, err
	}
	options.Page, _ = strconv.Atoi(query.Get("page"))
	options.PageSize, _ = strconv.Atoi(query.Get("page_size"))
	return options, query, nil
}

func (client *Client) ListProjectFlightAreas(ctx context.Context, token, projectUUID string, options FlightAreaListOptions) (FlightAreaPage, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return FlightAreaPage{}, err
	}
	options, query, err := normalizedFlightAreaOptions(options)
	if err != nil {
		return FlightAreaPage{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/flight-areas", Query: query})
	if err != nil {
		return FlightAreaPage{}, err
	}
	return decodeFlightAreaPage(payload, options.PageOptions)
}

func (client *Client) ListWorkspaceFlightAreas(ctx context.Context, token, workspaceID string, options FlightAreaListOptions) (FlightAreaPage, error) {
	path, err := geospatialWorkspacePath("/openapi/v2.0/workspaces/{workspace_id}/flight-areas", workspaceID, "")
	if err != nil {
		return FlightAreaPage{}, err
	}
	options, query, err := normalizedFlightAreaOptions(options)
	if err != nil {
		return FlightAreaPage{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path, Query: query})
	if err != nil {
		return FlightAreaPage{}, err
	}
	return decodeFlightAreaPage(payload, options.PageOptions)
}

func validChecksum(value string) bool {
	if len(value) != 32 && len(value) != 64 {
		return false
	}
	for _, character := range value {
		if character < '0' || character > '9' {
			if character < 'a' || character > 'f' {
				if character < 'A' || character > 'F' {
					return false
				}
			}
		}
	}
	return true
}

func (client *Client) validateGeospatialFile(file *GeospatialFileDownload) error {
	if file == nil || !validGeospatialString(file.Name, 512, false) || strings.ContainsAny(file.Name, "/\\") ||
		!validChecksum(file.Checksum) || file.Size < 0 {
		return schemaError()
	}
	download, err := client.validateDownload(file.URL, 0)
	if err != nil {
		return err
	}
	file.ExpiresAt = download.ExpiresAt
	return nil
}

func (client *Client) GetProjectFlightAreaFile(ctx context.Context, token, projectUUID string) (GeospatialFileDownload, error) {
	projectUUID, err := requireScope(projectUUID)
	if err != nil {
		return GeospatialFileDownload{}, err
	}
	payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: "/openapi/v2.0/flight-areas/url"})
	if err != nil {
		return GeospatialFileDownload{}, err
	}
	var result GeospatialFileDownload
	if json.Unmarshal(payload.Data, &result) != nil {
		return GeospatialFileDownload{}, schemaError()
	}
	if err := client.validateGeospatialFile(&result); err != nil {
		return GeospatialFileDownload{}, err
	}
	return result, nil
}

func (client *Client) GetWorkspaceOfflineMapDownload(ctx context.Context, token, workspaceID string) (OfflineMapDownload, error) {
	path, err := geospatialWorkspacePath("/openapi/v2.0/workspaces/{workspace_id}/offline-maps/url", workspaceID, "")
	if err != nil {
		return OfflineMapDownload{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return OfflineMapDownload{}, err
	}
	var raw struct {
		Enabled bool            `json:"offline_map_enable"`
		File    json.RawMessage `json:"file"`
	}
	if json.Unmarshal(payload.Data, &raw) != nil || raw.File == nil {
		return OfflineMapDownload{}, schemaError()
	}
	if !raw.Enabled {
		var empty map[string]json.RawMessage
		if json.Unmarshal(raw.File, &empty) != nil || len(empty) != 0 {
			return OfflineMapDownload{}, schemaError()
		}
		return OfflineMapDownload{Enabled: false}, nil
	}
	var file GeospatialFileDownload
	if json.Unmarshal(raw.File, &file) != nil {
		return OfflineMapDownload{}, schemaError()
	}
	if err := client.validateGeospatialFile(&file); err != nil {
		return OfflineMapDownload{}, err
	}
	return OfflineMapDownload{Enabled: true, File: &file}, nil
}

func validateOfflineMap(value *OfflineMap) bool {
	if value == nil || value.ID <= 0 || value.UpdatedTime <= 0 || value.Models == nil ||
		!validFinite(value.Percent) || value.Percent < 0 || value.Percent > 100 ||
		!validGeospatialString(value.Status, 128, false) {
		return false
	}
	for _, model := range value.Models {
		if model.ID <= 0 || !validGeospatialString(model.Name, 512, false) {
			return false
		}
	}
	return true
}

func (client *Client) GetWorkspaceOfflineMap(ctx context.Context, token, workspaceID string) (OfflineMapDetails, error) {
	path, err := geospatialWorkspacePath("/openapi/v2.0/workspaces/{workspace_id}/offline-maps", workspaceID, "")
	if err != nil {
		return OfflineMapDetails{}, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return OfflineMapDetails{}, err
	}
	var result OfflineMapDetails
	if json.Unmarshal(payload.Data, &result) != nil || result.Disabled != (result.OfflineMap == nil) ||
		result.OfflineMap != nil && !validateOfflineMap(result.OfflineMap) {
		return OfflineMapDetails{}, schemaError()
	}
	return result, nil
}

func validateAirSenseEvent(event *AirSenseWarningEvent) bool {
	return event != nil && validGeospatialString(event.ICAO, 64, false) &&
		event.WarningLevel >= 1 && event.WarningLevel <= 3 &&
		validFinite(event.Latitude) && event.Latitude >= -90 && event.Latitude <= 90 &&
		validFinite(event.Longitude) && event.Longitude >= -180 && event.Longitude <= 180 &&
		event.AltitudeType >= 0 && event.AltitudeType <= 1 &&
		validFinite(event.Heading) && event.Heading >= 0 && event.Heading <= 360 &&
		validFinite(event.RelativeAltitude) && event.VerticalTrend >= 0 && event.VerticalTrend <= 2 &&
		validFinite(event.Distance) && event.Distance >= 0
}

func (client *Client) ListWorkspaceAirSenseWarnings(ctx context.Context, token, workspaceID string) ([]DeviceAirSenseWarnings, error) {
	path, err := geospatialWorkspacePath("/openapi/v2.0/workspaces/{workspace_id}/air-sense-warnings", workspaceID, "")
	if err != nil {
		return nil, err
	}
	payload, err := client.request(ctx, token, "", requestSpec{Method: http.MethodGet, Path: path})
	if err != nil {
		return nil, err
	}
	var result []DeviceAirSenseWarnings
	if json.Unmarshal(payload.Data, &result) != nil || result == nil {
		return nil, schemaError()
	}
	now := client.now().UTC()
	for index := range result {
		item := &result[index]
		if !validGeospatialString(item.DeviceSN, 256, false) || item.Timestamp <= 0 || item.Events == nil {
			return nil, schemaError()
		}
		for eventIndex := range item.Events {
			if !validateAirSenseEvent(&item.Events[eventIndex]) {
				return nil, schemaError()
			}
		}
		item.CapturedAt = time.UnixMilli(item.Timestamp).UTC()
		item.ExpiresAt = item.CapturedAt.Add(airSenseWarningTTL)
		item.Expired = !item.ExpiresAt.After(now)
	}
	return result, nil
}
