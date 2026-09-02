package flighthub

import (
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strings"
	"time"

	"aerosight/worker/internal/connector"
)

const ReadOnlySmokeEndpointCount = 59

type ReadOnlySmokeEndpoint struct {
	ID         string
	Path       string
	Domain     string
	Scope      string
	Pagination string
}

type ReadOnlySmokeContext struct {
	ProjectUUID      string
	OrganizationUUID string
	Values           map[string]string
}

type ReadOnlySmokeResult struct {
	Endpoint   string   `json:"endpoint"`
	Category   string   `json:"category"`
	Count      int      `json:"count"`
	Fields     []string `json:"fields"`
	DurationMS int64    `json:"durationMs"`
}

func LoadReadOnlySmokeManifest(reader io.Reader) ([]ReadOnlySmokeEndpoint, error) {
	csvReader := csv.NewReader(reader)
	csvReader.Comma = '\t'
	csvReader.FieldsPerRecord = 11
	rows, err := csvReader.ReadAll()
	if err != nil || len(rows) < 2 {
		return nil, errors.New("DJI_FLIGHTHUB_SMOKE_MANIFEST_INVALID")
	}
	wantHeader := []string{"id", "method", "path", "status", "title", "domain", "scope", "risk", "pagination", "deployment", "verification"}
	if strings.Join(rows[0], "\x00") != strings.Join(wantHeader, "\x00") {
		return nil, errors.New("DJI_FLIGHTHUB_SMOKE_MANIFEST_INVALID")
	}
	endpoints := make([]ReadOnlySmokeEndpoint, 0, ReadOnlySmokeEndpointCount)
	seen := make(map[string]struct{}, ReadOnlySmokeEndpointCount)
	for _, row := range rows[1:] {
		if row[1] != http.MethodGet || row[3] != "released" {
			continue
		}
		if row[7] != "low" || row[9] != "cn-public-cloud" || strings.TrimSpace(row[0]) == "" ||
			!strings.HasPrefix(row[2], "/openapi/v2.0/") || strings.Contains(row[2], "://") {
			return nil, errors.New("DJI_FLIGHTHUB_SMOKE_MANIFEST_INVALID")
		}
		if _, duplicate := seen[row[0]]; duplicate {
			return nil, errors.New("DJI_FLIGHTHUB_SMOKE_MANIFEST_INVALID")
		}
		seen[row[0]] = struct{}{}
		endpoints = append(endpoints, ReadOnlySmokeEndpoint{ID: row[0], Path: row[2], Domain: row[5], Scope: row[6], Pagination: row[8]})
	}
	if len(endpoints) != ReadOnlySmokeEndpointCount {
		return nil, errors.New("DJI_FLIGHTHUB_SMOKE_MANIFEST_COVERAGE_INVALID")
	}
	return endpoints, nil
}

func LoadReadOnlySmokeContext(ctx context.Context, database *sql.DB, instance connector.Instance) (ReadOnlySmokeContext, error) {
	if database == nil || instance.ID <= 0 || instance.ProjectID <= 0 {
		return ReadOnlySmokeContext{}, errors.New("DJI_FLIGHTHUB_SMOKE_SCOPE_INVALID")
	}
	scope, err := parseScope(instance.DiscoveryScope)
	if err != nil {
		return ReadOnlySmokeContext{}, err
	}
	values := map[string]string{"workspace_id": scope.ProjectUUID}
	var serial sql.NullString
	err = database.QueryRowContext(ctx, `select identity.identity_json#>>'{attributes,serialNumber}'
		from device_external_identities identity
		where identity.project_id=$1 and identity.adapter_id=$2 and identity.discovery_status='managed'
		  and nullif(identity.identity_json#>>'{attributes,serialNumber}','') is not null
		order by identity.last_seen_at desc limit 1`, instance.ProjectID, instance.ID).Scan(&serial)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return ReadOnlySmokeContext{}, err
	}
	if serial.Valid {
		values["device_sn"], values["sn"] = serial.String, serial.String
	}
	rows, err := database.QueryContext(ctx, `select resource_kind,remote_id from connector_remote_resources
		where project_id=$1 and connector_instance_id=$2 and status='active'
		  and resource_kind in('flight-task','wayline','model','model-resource')
		order by last_seen_at desc,id desc`, instance.ProjectID, instance.ID)
	if err != nil {
		return ReadOnlySmokeContext{}, err
	}
	defer rows.Close()
	for rows.Next() {
		var kind, remoteID string
		if err := rows.Scan(&kind, &remoteID); err != nil {
			return ReadOnlySmokeContext{}, err
		}
		remoteID = strings.TrimSpace(remoteID)
		switch {
		case kind == "flight-task" && values["task_uuid"] == "":
			values["task_uuid"] = remoteID
		case kind == "wayline" && values["wayline_id"] == "":
			values["wayline_id"] = remoteID
		case kind == "model" && values["model_id"] == "":
			values["model_id"] = remoteID
		case kind == "model-resource" && strings.HasPrefix(remoteID, "model:") && values["model_uuid"] == "":
			values["model_uuid"] = strings.TrimPrefix(remoteID, "model:")
		case kind == "model-resource" && strings.HasPrefix(remoteID, "resource:") && values["resource_uuid"] == "":
			values["resource_uuid"] = strings.TrimPrefix(remoteID, "resource:")
		case kind == "model-resource" && strings.HasPrefix(remoteID, "file:") && values["file_id"] == "":
			values["file_id"] = strings.TrimPrefix(remoteID, "file:")
		}
	}
	if err := rows.Err(); err != nil {
		return ReadOnlySmokeContext{}, err
	}
	return ReadOnlySmokeContext{ProjectUUID: scope.ProjectUUID, OrganizationUUID: scope.OrganizationUUID, Values: values}, nil
}

func RunReadOnlySmoke(ctx context.Context, client *Client, token string, endpoints []ReadOnlySmokeEndpoint, scope ReadOnlySmokeContext) []ReadOnlySmokeResult {
	results := make([]ReadOnlySmokeResult, 0, len(endpoints))
	for _, endpoint := range endpoints {
		started := time.Now()
		result := ReadOnlySmokeResult{Endpoint: endpoint.ID, Category: "request_invalid", Fields: []string{}}
		path, ok := resolveReadOnlySmokePath(endpoint.Path, scope)
		if !ok {
			result.Category = "prerequisite_missing"
			result.DurationMS = elapsedMilliseconds(started)
			results = append(results, result)
			continue
		}
		projectUUID := scope.ProjectUUID
		if endpoint.Scope == "global" || endpoint.Scope == "organization" || endpoint.Scope == "organization-device" {
			projectUUID = ""
		}
		profile := ""
		if endpoint.Path == "/openapi/v2.0/live-shares" {
			profile = "live-share-list"
		} else if strings.HasPrefix(endpoint.Path, "/openapi/v2.0/live-shares/") {
			profile = "live-share-detail"
		}
		payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path, Profile: profile})
		result.DurationMS = elapsedMilliseconds(started)
		if err != nil {
			var apiError *APIError
			if errors.As(err, &apiError) && safeSmokeCategory(apiError.SafeCode) {
				result.Category = apiError.SafeCode
			} else if ctx.Err() != nil {
				result.Category = "cancelled"
			} else {
				result.Category = "upstream_error"
			}
			results = append(results, result)
			continue
		}
		result.Category, result.Count, result.Fields = summarizeSmokePayload(payload)
		results = append(results, result)
	}
	return results
}

func resolveReadOnlySmokePath(template string, scope ReadOnlySmokeContext) (string, bool) {
	values := make(map[string]string, len(scope.Values)+1)
	for name, value := range scope.Values {
		values[name] = value
	}
	values["uuid"] = scope.OrganizationUUID
	parameters := map[string]string{}
	for remainder := template; ; {
		start := strings.IndexByte(remainder, '{')
		if start < 0 {
			break
		}
		end := strings.IndexByte(remainder[start+1:], '}')
		if end < 0 {
			return "", false
		}
		name := remainder[start+1 : start+1+end]
		value := strings.TrimSpace(values[name])
		if value == "" {
			return "", false
		}
		parameters[name] = value
		remainder = remainder[start+end+2:]
	}
	path, err := resolvePathTemplate(template, parameters)
	return path, err == nil
}

func summarizeSmokePayload(payload envelope) (string, int, []string) {
	if payload.Empty || len(payload.Data) == 0 || string(payload.Data) == "null" {
		return "empty", 0, []string{}
	}
	var value any
	if json.Unmarshal(payload.Data, &value) != nil {
		return "schema_incompatible", 0, []string{}
	}
	count, fields := smokeShape(value)
	if count == 0 {
		return "empty", 0, fields
	}
	return "succeeded", count, fields
}

func smokeShape(value any) (int, []string) {
	switch typed := value.(type) {
	case []any:
		if len(typed) == 0 {
			return 0, []string{}
		}
		_, fields := smokeShape(typed[0])
		return len(typed), fields
	case map[string]any:
		for _, collectionKey := range []string{"list", "items", "records", "rows"} {
			if collection, ok := typed[collectionKey].([]any); ok {
				if len(collection) == 0 {
					return 0, sortedSmokeFields(typed)
				}
				_, fields := smokeShape(collection[0])
				return len(collection), fields
			}
		}
		return 1, sortedSmokeFields(typed)
	default:
		return 1, []string{}
	}
}

func sortedSmokeFields(value map[string]any) []string {
	fields := make([]string, 0, min(len(value), 128))
	for field := range value {
		field = strings.TrimSpace(field)
		if field != "" && len(field) <= 128 {
			fields = append(fields, field)
		}
	}
	sort.Strings(fields)
	if len(fields) > 128 {
		fields = fields[:128]
	}
	return fields
}

func elapsedMilliseconds(started time.Time) int64 {
	return max(0, time.Since(started).Milliseconds())
}

func safeSmokeCategory(value string) bool {
	switch value {
	case "credential_invalid", "scope_forbidden", "scope_not_found", "rate_limited", "upstream_unavailable",
		"request_timeout", "response_too_large", "schema_incompatible", "request_invalid", "upstream_error",
		"parameter_required", "not_applicable", "empty":
		return true
	default:
		return false
	}
}
