package flighthub

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/csv"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
	"strconv"
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
	ProjectUUID        string
	OrganizationUUID   string
	AccountFingerprint string
	Values             map[string]string
}

type ReadOnlySmokeResult struct {
	Endpoint   string   `json:"endpoint"`
	Category   string   `json:"category"`
	Count      int      `json:"count"`
	Fields     []string `json:"fields"`
	DurationMS int64    `json:"durationMs"`
}

type ReadOnlySmokeEvidenceRepository interface {
	SaveCapabilitySnapshot(context.Context, connector.Instance, connector.CapabilitySnapshot) error
	SaveCapabilityAccountFingerprint(context.Context, connector.Instance, string) error
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
	return ReadOnlySmokeContext{ProjectUUID: scope.ProjectUUID, OrganizationUUID: scope.OrganizationUUID, AccountFingerprint: scope.AccountFingerprint, Values: values}, nil
}

func HydrateReadOnlySmokeContext(ctx context.Context, client *Client, token string, scope ReadOnlySmokeContext) (ReadOnlySmokeContext, error) {
	if client == nil || strings.TrimSpace(token) == "" || !uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(scope.ProjectUUID))) {
		return ReadOnlySmokeContext{}, &APIError{SafeCode: "request_invalid"}
	}
	projects, err := client.ListProjects(ctx, token)
	if err != nil {
		return ReadOnlySmokeContext{}, err
	}
	organizationUUID := ""
	for _, project := range projects {
		if !strings.EqualFold(project.UUID, scope.ProjectUUID) {
			continue
		}
		if organizationUUID != "" || !uuidPattern.MatchString(strings.ToLower(strings.TrimSpace(project.OrganizationUUID))) {
			return ReadOnlySmokeContext{}, &APIError{SafeCode: "scope_forbidden"}
		}
		organizationUUID = strings.ToLower(strings.TrimSpace(project.OrganizationUUID))
	}
	if organizationUUID == "" || (scope.OrganizationUUID != "" && !strings.EqualFold(scope.OrganizationUUID, organizationUUID)) {
		return ReadOnlySmokeContext{}, &APIError{SafeCode: "scope_forbidden"}
	}
	currentRole, err := client.GetCurrentOrganizationRole(ctx, token, organizationUUID)
	if err != nil {
		return ReadOnlySmokeContext{}, err
	}
	if !strings.EqualFold(strings.TrimSpace(currentRole.OrganizationUUID), organizationUUID) {
		return ReadOnlySmokeContext{}, &APIError{SafeCode: "scope_forbidden"}
	}
	fingerprint, err := CapabilityAccountFingerprint(organizationUUID, currentRole.UserID)
	if err != nil {
		return ReadOnlySmokeContext{}, err
	}
	scope.OrganizationUUID = organizationUUID
	scope.AccountFingerprint = fingerprint
	if scope.Values == nil {
		scope.Values = map[string]string{}
	}
	return scope, nil
}

func RunReadOnlySmoke(ctx context.Context, client *Client, token string, endpoints []ReadOnlySmokeEndpoint, scope ReadOnlySmokeContext) []ReadOnlySmokeResult {
	endpoints = orderReadOnlySmokeEndpoints(endpoints)
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
		payload, err := client.request(ctx, token, projectUUID, requestSpec{Method: http.MethodGet, Path: path, Profile: profile, DataOptional: true})
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
		learnReadOnlySmokeValues(endpoint.ID, payload.Data, scope.Values)
		result.Category, result.Count, result.Fields = summarizeSmokePayload(payload)
		results = append(results, result)
	}
	return results
}

func orderReadOnlySmokeEndpoints(endpoints []ReadOnlySmokeEndpoint) []ReadOnlySmokeEndpoint {
	ordered := append([]ReadOnlySmokeEndpoint(nil), endpoints...)
	priority := map[string]int{
		"454273364e0": 1, // project list
		"456447011e0": 2, // organization list
		"456680822e0": 3, // project devices
		"456680824e0": 4, // wayline list
		"454273439e0": 5, // flight task list
		"458069507e0": 6, // model list
		"458069512e0": 7, // running open models
	}
	sort.SliceStable(ordered, func(left, right int) bool {
		leftPriority, leftKnown := priority[ordered[left].ID]
		rightPriority, rightKnown := priority[ordered[right].ID]
		if leftKnown != rightKnown {
			return leftKnown
		}
		return leftKnown && leftPriority < rightPriority
	})
	return ordered
}

func learnReadOnlySmokeValues(endpointID string, raw json.RawMessage, values map[string]string) {
	if values == nil || len(raw) == 0 {
		return
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.UseNumber()
	if decoder.Decode(&value) != nil {
		return
	}
	switch endpointID {
	case "456680824e0":
		setSmokeValue(values, "wayline_id", firstSmokeScalar(value, "id"))
	case "454273439e0", "454273433e0":
		setSmokeValue(values, "task_uuid", firstSmokeScalar(value, "uuid"))
	case "458069507e0":
		setSmokeValue(values, "model_id", firstSmokeScalar(value, "id"))
	case "458069512e0":
		setSmokeValue(values, "model_uuid", firstSmokeScalar(value, "model_uuid"))
		setSmokeValue(values, "resource_uuid", firstSmokeScalar(value, "resource_uuid"))
	case "458069510e0":
		setSmokeValue(values, "file_id", firstSmokeScalar(value, "file_id"))
	}
}

func setSmokeValue(values map[string]string, name, value string) {
	if strings.TrimSpace(values[name]) == "" && strings.TrimSpace(value) != "" {
		values[name] = value
	}
}

func firstSmokeScalar(value any, key string) string {
	switch typed := value.(type) {
	case []any:
		for _, item := range typed {
			if found := firstSmokeScalar(item, key); found != "" {
				return found
			}
		}
	case map[string]any:
		if raw, exists := typed[key]; exists {
			switch scalar := raw.(type) {
			case string:
				return strings.TrimSpace(scalar)
			case json.Number:
				if integer, err := strconv.ParseInt(scalar.String(), 10, 64); err == nil && integer >= 0 {
					return strconv.FormatInt(integer, 10)
				}
			}
		}
		for _, nestedKey := range []string{"list", "items", "records", "rows", "data"} {
			if nested, exists := typed[nestedKey]; exists {
				if found := firstSmokeScalar(nested, key); found != "" {
					return found
				}
			}
		}
	}
	return ""
}

func PersistReadOnlySmokeEvidence(
	ctx context.Context,
	repository ReadOnlySmokeEvidenceRepository,
	instance connector.Instance,
	endpoints []ReadOnlySmokeEndpoint,
	results []ReadOnlySmokeResult,
	scope ReadOnlySmokeContext,
	verifiedAt time.Time,
	ttl time.Duration,
) error {
	if repository == nil || instance.ID <= 0 || instance.ProjectID <= 0 || !validAccountFingerprint(scope.AccountFingerprint) ||
		verifiedAt.IsZero() || ttl <= 0 || len(endpoints) != len(results) || len(results) != ReadOnlySmokeEndpointCount {
		return &APIError{SafeCode: "request_invalid"}
	}
	endpointByID := make(map[string]ReadOnlySmokeEndpoint, len(endpoints))
	for _, endpoint := range endpoints {
		if _, duplicate := endpointByID[endpoint.ID]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		endpointByID[endpoint.ID] = endpoint
	}
	grouped := map[string][]ReadOnlySmokeResult{}
	seenResults := make(map[string]struct{}, len(results))
	for _, result := range results {
		endpoint, exists := endpointByID[result.Endpoint]
		if !exists || !safeSmokeCategory(result.Category) && result.Category != "succeeded" && result.Category != "prerequisite_missing" {
			return &APIError{SafeCode: "request_invalid"}
		}
		if _, duplicate := seenResults[result.Endpoint]; duplicate {
			return &APIError{SafeCode: "request_invalid"}
		}
		seenResults[result.Endpoint] = struct{}{}
		capabilityCode := readOnlySmokeCapability(endpoint)
		if capabilityCode != "" {
			grouped[capabilityCode] = append(grouped[capabilityCode], result)
		}
	}
	if len(seenResults) != len(endpointByID) {
		return &APIError{SafeCode: "request_invalid"}
	}
	if err := repository.SaveCapabilityAccountFingerprint(ctx, instance, scope.AccountFingerprint); err != nil {
		return err
	}
	expiresAt := verifiedAt.Add(ttl)
	capabilityCodes := make([]string, 0, len(grouped))
	for code := range grouped {
		capabilityCodes = append(capabilityCodes, code)
	}
	sort.Strings(capabilityCodes)
	for _, capabilityCode := range capabilityCodes {
		items := grouped[capabilityCode]
		safeItems := make([]map[string]any, 0, len(items))
		for _, item := range items {
			safeItems = append(safeItems, map[string]any{
				"endpointId": item.Endpoint, "category": item.Category, "count": item.Count,
				"fields": append([]string(nil), item.Fields...), "durationMs": item.DurationMS,
			})
		}
		status, reason := summarizeReadOnlySmokeEvidence(items)
		if err := repository.SaveCapabilitySnapshot(ctx, instance, connector.CapabilitySnapshot{
			CapabilityCode: capabilityCode, Status: status, EvidenceLevel: "live-read", Region: "cn", Deployment: "cn-public-cloud",
			AccountFingerprint: scope.AccountFingerprint, Details: map[string]any{"source": "read-only-smoke", "reason": reason, "endpoints": safeItems},
			VerifiedAt: verifiedAt, ExpiresAt: &expiresAt,
		}); err != nil {
			return err
		}
	}
	return nil
}

func readOnlySmokeCapability(endpoint ReadOnlySmokeEndpoint) string {
	switch endpoint.Domain {
	case "system":
		return "health.read"
	case "organization", "project":
		return "organization.read"
	case "device":
		switch endpoint.ID {
		case "458069501e0":
			return "state.read"
		case "458069499e0", "457048706e0":
			return "health.read"
		default:
			return "inventory.read"
		}
	case "flight":
		return "flight.read"
	case "live":
		return "live.read"
	case "geospatial":
		return "geospatial.read"
	case "model":
		return "model.read"
	case "control":
		if endpoint.ID == "454273421e0" {
			return "tca.status.read"
		}
		return "device.control"
	default:
		return ""
	}
}

func summarizeReadOnlySmokeEvidence(results []ReadOnlySmokeResult) (string, string) {
	counts := map[string]int{}
	for _, result := range results {
		counts[result.Category]++
	}
	if counts["succeeded"] > 0 {
		if counts["succeeded"] == len(results) {
			return "supported", "read_only_smoke_succeeded"
		}
		return "supported", "read_only_smoke_partial"
	}
	if counts["empty"] > 0 {
		return "empty", "read_only_smoke_empty"
	}
	if counts["scope_forbidden"] > 0 || counts["credential_invalid"] > 0 {
		return "forbidden", "read_only_smoke_forbidden"
	}
	if counts["rate_limited"] > 0 || counts["request_timeout"] > 0 || counts["upstream_unavailable"] > 0 || counts["cancelled"] > 0 {
		return "degraded", "read_only_smoke_degraded"
	}
	if counts["not_applicable"] > 0 {
		return "not_applicable", "read_only_smoke_not_applicable"
	}
	if counts["configuration_required"] > 0 || counts["parameter_required"] > 0 || counts["prerequisite_missing"] > 0 || counts["scope_not_found"] > 0 {
		return "unverified", "read_only_smoke_context_required"
	}
	return "failed", "read_only_smoke_failed"
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
		"parameter_required", "configuration_required", "not_applicable", "empty", "cancelled":
		return true
	default:
		return false
	}
}
