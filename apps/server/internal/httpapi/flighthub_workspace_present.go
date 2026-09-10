package httpapi

import (
	"crypto/sha256"
	"encoding/hex"
	"math"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/gin-gonic/gin"
)

var fhCodePattern = regexp.MustCompile(`^[A-Za-z0-9_.:-]{1,128}$`)
var fhSecretText = regexp.MustCompile(`(?i)https?://|(?:^|[?&])(token|signature|credential|secret|x-amz-[^=]*)=|\b[0-9a-f]{8}-[0-9a-f]{4}-[1-5][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}\b`)
var fhControlText = regexp.MustCompile(`[\x00-\x1f\x7f]`)
var fhUUIDPattern = regexp.MustCompile(`(?i)^[0-9a-f]{8}-[0-9a-f]{4}-[1-8][0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
var fhMimePattern = regexp.MustCompile(`(?i)^[a-z0-9.+_-]+/[a-z0-9.*+_-]+$`)

func fhString(v any) string { s, _ := v.(string); return s }
func fhLabel(v any, fallback string) any {
	s := strings.TrimSpace(fhString(v))
	if s == "" || len(utf16.Encode([]rune(s))) > 200 || fhControlText.MatchString(s) || fhSecretText.MatchString(s) {
		return fallback
	}
	return s
}
func fhCode(v any, fallback any) any {
	s := strings.TrimSpace(fhString(v))
	if !fhCodePattern.MatchString(s) {
		return fallback
	}
	return s
}
func fhNumber(v any, integer bool) any {
	var n float64
	switch x := v.(type) {
	case float64:
		n = x
	case string:
		var err error
		n, err = strconv.ParseFloat(strings.TrimSpace(x), 64)
		if err != nil {
			return nil
		}
	default:
		return nil
	}
	if math.IsNaN(n) || math.IsInf(n, 0) || (integer && (n < 0 || n > 9007199254740991 || math.Trunc(n) != n)) {
		return nil
	}
	return n
}
func fhTime(v any) any {
	if s, ok := v.(string); ok {
		if t, e := time.Parse(time.RFC3339Nano, s); e == nil {
			return timestamp(t)
		}
	}
	return nil
}
func fhBool(v any) any {
	switch x := v.(type) {
	case bool:
		return x
	case string:
		if x == "true" || x == "1" {
			return true
		}
		if x == "false" || x == "0" {
			return false
		}
	}
	return nil
}
func fhTransform(v any, kind string) any {
	switch kind {
	case "code":
		return fhCode(v, "unknown")
	case "optional":
		return fhCode(v, nil)
	case "labelnullable":
		if v == nil || v == "" {
			return nil
		}
		return fhLabel(v, "设备")
	case "signed":
		n := fhNumber(v, false)
		if n == nil || math.Abs(n.(float64)) > 9007199254740991 || math.Trunc(n.(float64)) != n.(float64) {
			return nil
		}
		return n
	case "int":
		return fhNumber(v, true)
	case "zero":
		if n := fhNumber(v, true); n != nil {
			return n
		}
		return float64(0)
	case "number":
		return fhNumber(v, false)
	case "time":
		return fhTime(v)
	case "bool":
		return fhBool(v)
	case "uuid":
		if fhUUIDPattern.MatchString(fhString(v)) {
			return v
		}
		return nil
	case "mime":
		if fhMimePattern.MatchString(fhString(v)) {
			return v
		}
		return nil
	case "codes", "ids", "labels":
		result := []any{}
		items, _ := v.([]any)
		for _, item := range items {
			var safe any
			if kind == "codes" {
				safe = fhCode(item, nil)
			} else if kind == "ids" {
				safe = fhNumber(item, true)
				if safe == float64(0) {
					safe = nil
				}
			} else {
				safe = fhLabel(item, "")
				if safe == "" {
					safe = nil
				}
			}
			if safe != nil {
				result = append(result, safe)
			}
			if kind != "ids" && len(result) >= 32 {
				break
			}
		}
		return result
	}
	if strings.HasPrefix(kind, "label=") {
		return fhLabel(v, strings.TrimPrefix(kind, "label="))
	}
	return v
}

// Field allowlists deliberately exclude remote IDs, credentials and raw metadata.
func fhFields(row gin.H, spec string) gin.H {
	out := gin.H{}
	for _, entry := range strings.Fields(spec) {
		parts := strings.SplitN(entry, ":", 2)
		kind := ""
		if len(parts) > 1 {
			kind = parts[1]
		}
		out[parts[0]] = fhTransform(row[parts[0]], kind)
	}
	return out
}
func fhMap(rows []gin.H, spec string) []gin.H {
	out := make([]gin.H, 0, len(rows))
	for _, r := range rows {
		out = append(out, fhFields(r, spec))
	}
	return out
}
func presentFHFlightOps(d map[string][]gin.H) gin.H {
	can := d["access"][0]["canOperate"] == true
	mode := "read-only"
	if can {
		mode = "operator"
	}
	connectors := fhMap(d["connectors"], "id name:label=司空连接器 status:code lastCheckedAt:time actionEnabled actionVerified")
	for _, r := range connectors {
		ready := can && r["status"] == "connected" && r["actionEnabled"] == true && r["actionVerified"] == true
		r["actionReady"] = ready
		r["availableActions"] = []string{}
		if ready {
			r["availableActions"] = []string{"flight-task-create", "flight-task-status", "flight-task-resumption"}
		}
	}
	actions := fhMap(d["actions"], "id:uuid connectorId taskRunId deviceId waylineResourceId targetResourceId resultResourceId action:code status:code attemptCount reconciliationCount lastErrorCode:optional acceptedAt:time reconciledAt:time unknownAt:time completedAt:time createdAt:time updatedAt:time")
	for _, r := range actions {
		if r["id"] == nil {
			r["id"] = ""
		}
		r["final"] = r["status"] == "succeeded" || r["status"] == "failed" || r["status"] == "blocked"
	}
	alerts := fhMap(d["alerts"], "id connectorId kind:code taskRunId:int perceptionEventId:uuid issueId:int title:label=司空飞行告警 severity:optional status:optional alertCount:int confidence:number hasMedia occurredAt:time lastSeenAt:time")
	for i, r := range alerts {
		r["hasMedia"] = r["hasMedia"] == true
		if r["kind"] == "ai-alert" {
			r["title"] = fhLabel(d["alerts"][i]["title"], "司空 AI 告警")
		}
	}
	return gin.H{"access": gin.H{"mode": mode, "canOperate": can}, "connectors": connectors,
		"waylines":      fhMap(d["waylines"], "id connectorId taskId:int name:label=未命名航线 status:code deviceModelKey:optional templateTypes:codes payloadCount:int sizeBytes:int remoteUpdatedAt:time lastSeenAt:time"),
		"taskRuns":      fhMap(d["task-runs"], "id connectorId taskRunId taskId taskName:label=司空飞行任务 deviceId:int deviceName:labelnullable status:code stateReason:optional taskType:optional mediaUploadStatus:optional resumableStatus:optional breakPointResume currentWaypoint:int totalWaypoints:int exceptionCount:int startedAt:time finishedAt:time remoteUpdatedAt:time"),
		"tracks":        fhMap(d["tracks"], "id connectorId taskRunId taskName:label=司空飞行任务 deviceId deviceName:label=设备 pointCount:zero firstCapturedAt:time lastCapturedAt:time"),
		"media":         fhMap(d["flight-media"], "id connectorId assetId taskRunId:int deviceId:int name:label=飞行媒体 kind:code mimeType:mime status:code fileType:optional suffix:optional sizeBytes:int capturedAt:time createdAt:time"),
		"flightRecords": fhMap(d["flight-record"], "id connectorId assetId taskRunId:int name:label=飞行记录 kind:code mimeType:mime status:code contentType:optional progress:number fileTypes:codes failedReasonCode:optional capturedAt:time createdAt:time"), "alerts": alerts, "actions": actions}
}
func fhScoped(rows []gin.H, pid int32) []gin.H {
	out := []gin.H{}
	for _, r := range rows {
		if fhNumber(r["projectId"], true) == float64(pid) {
			out = append(out, r)
		}
	}
	return out
}
func presentFHModels(pid int32, d map[string][]gin.H) gin.H {
	access := d["access"][0]
	can := access["canOperate"] == true
	mode := "read-only"
	if can {
		mode = "operator"
	}
	scoped := fhScoped(d["connectors"], pid)
	connectors := fhMap(scoped, "id name:label=司空连接器 status:code lastCheckedAt:time")
	ids := map[any]bool{}
	for i, r := range connectors {
		ids[r["id"]] = true
		actions := []gin.H{}
		for _, a := range fhScoped(d["actions"], pid) {
			if a["connectorId"] != r["id"] {
				continue
			}
			item := fhFields(a, "action:code flagEnabled capabilityVerified")
			status := scoped[i]["status"]
			item["available"] = can && (status == "connecting" || status == "connected" || status == "degraded") && a["flagEnabled"] == true && a["capabilityVerified"] == true
			actions = append(actions, item)
		}
		r["actions"] = actions
	}
	filter := func(rows []gin.H) []gin.H {
		out := []gin.H{}
		for _, r := range fhScoped(rows, pid) {
			if ids[r["connectorId"]] {
				out = append(out, r)
			}
		}
		return out
	}
	models, resources := []gin.H{}, []gin.H{}
	for _, r := range filter(d["resources"]) {
		var item gin.H
		if r["kind"] == "model" {
			item = fhFields(r, "id connectorId name:label=司空模型 status:code fileType:optional showOnMap:bool sizeBytes:signed assetId:signed assetStatus:optional assetFailureCode:optional remoteUpdatedAt:time lastSeenAt:time")
			item["showOnMap"] = item["showOnMap"] == true
			models = append(models, item)
		} else if r["kind"] == "model-resource" {
			item = fhFields(r, "id connectorId status:code modelType:signed modelStatus:signed reconstructionProgress:signed errorCode:signed zipStatus:signed zipProgress:signed resourceStatus:signed fileCount:signed sizeBytes:signed assetId:signed assetKind:optional assetStatus:optional assetFailureCode:optional lastSeenAt:time")
			resources = append(resources, item)
		}
		if item != nil {
			item["source"] = "dji-flighthub-openapi"
		}
	}
	return gin.H{"projectId": pid, "source": "dji-flighthub-openapi", "access": gin.H{"role": fhCode(access["role"], "unknown"), "canOperate": can, "mode": mode}, "connectors": connectors,
		"syncStates": fhMap(filter(d["sync"]), "connectorId status:code lastErrorCode:optional lastSucceededAt:time nextAttemptAt:time"), "models": models, "resources": resources,
		"jobs": fhMap(filter(d["jobs"]), "id connectorId jobType:code action:code status:code progress:signed stage:optional attemptCount:zero reconciliationCount:zero lastErrorCode:optional assetIds:ids createdAt:time updatedAt:time")}
}
func fhGeometry(value any) (any, any) {
	candidate, _ := value.(map[string]any)
	kind := fhCode(candidate["type"], nil)
	coordinates := candidate["coordinates"]
	position := func(v any) bool {
		a, ok := v.([]any)
		if !ok || len(a) < 2 || len(a) > 3 {
			return false
		}
		for i, v := range a {
			n := fhNumber(v, false)
			if n == nil {
				return false
			}
			f := n.(float64)
			a[i] = f
			if i == 0 && (f < -180 || f > 180) || i == 1 && (f < -90 || f > 90) {
				return false
			}
		}
		return true
	}
	line := func(v any) bool {
		a, ok := v.([]any)
		if !ok || len(a) < 2 || len(a) > 10000 {
			return false
		}
		for _, p := range a {
			if !position(p) {
				return false
			}
		}
		return true
	}
	switch kind {
	case "Point":
		if position(coordinates) {
			return gin.H{"type": "Point", "coordinates": coordinates}, kind
		}
	case "Polyline", "LineString":
		if line(coordinates) {
			return gin.H{"type": "LineString", "coordinates": coordinates}, kind
		}
	case "Polygon":
		rings, ok := coordinates.([]any)
		valid := ok && len(rings) > 0 && len(rings) <= 1024
		for _, v := range rings {
			ring, ok := v.([]any)
			if !ok || !line(v) || len(ring) < 4 {
				valid = false
				break
			}
			first := ring[0].([]any)
			last := ring[len(ring)-1].([]any)
			if len(first) != len(last) {
				valid = false
				break
			}
			for i := range first {
				if fhNumber(first[i], false) != fhNumber(last[i], false) {
					valid = false
				}
			}
		}
		if valid {
			return gin.H{"type": "Polygon", "coordinates": coordinates}, kind
		}
	case "Circle":
		a, ok := coordinates.([]any)
		if ok && len(a) > 0 {
			center := a[0]
			if _, ok := center.([]any); !ok {
				if len(a) >= 2 {
					center = a[:2]
				}
			}
			if position(center) {
				return gin.H{"type": "Point", "coordinates": center}, kind
			}
		}
	}
	return nil, kind
}
func presentFHGeo(pid int32, d map[string][]gin.H, now time.Time) gin.H {
	result := gin.H{"projectId": pid, "access": gin.H{"role": d["access"][0]["role"], "mode": "read-only"}, "source": "dji-flighthub-openapi",
		"connectors":  fhMap(fhScoped(d["connectors"], pid), "id name:label=司空连接器 status:code lastCheckedAt:time"),
		"syncStates":  fhMap(fhScoped(d["sync-states"], pid), "connectorId status:code attemptCount:zero lastErrorCode:optional lastStartedAt:time lastSucceededAt:time nextAttemptAt:time"),
		"mapElements": []gin.H{}, "flightAreas": []gin.H{}, "offlineMaps": []gin.H{}, "airSenseWarnings": []gin.H{}}
	for _, r := range fhScoped(d["resources"], pid) {
		kind := r["kind"]
		key, label, spec := "", "", ""
		switch kind {
		case "map-element":
			key = "mapElements"
			label = "司空地图标注"
			spec = "stateCode:optional display:bool"
		case "flight-area":
			key = "flightAreas"
			label = "司空飞行区"
			spec = "areaType:optional stateCode:optional"
		case "offline-map":
			key = "offlineMaps"
			label = "司空离线地图"
			spec = "progress:number resultCode:optional modelCount:zero modelNames:labels stateCode:optional"
		case "air-sense-warning":
			key = "airSenseWarnings"
			label = "AirSense 空域目标"
			spec = "warningLevel:int deviceId:int issueId:int expiresAt:time"
		default:
			continue
		}
		item := fhFields(r, "id connectorId status:code coordinateReference:optional remoteUpdatedAt:time lastSeenAt:time missingAt:time "+spec)
		item["name"] = fhLabel(r["name"], label)
		item["source"] = "dji-flighthub-openapi"
		item["versionFingerprint"] = nil
		if version := strings.TrimSpace(fhString(r["remoteVersion"])); version != "" {
			hash := sha256.Sum256([]byte(version))
			item["versionFingerprint"] = "v1-" + hex.EncodeToString(hash[:])[:12]
		}
		item["geometry"], item["geometryType"] = fhGeometry(r["geometry"])
		fresh := "unknown"
		switch r["status"] {
		case "missing", "deleted":
			fresh = "missing"
		case "failed":
			fresh = "stale"
		case "active":
			if seen, err := time.Parse(time.RFC3339Nano, fhString(r["lastSeenAt"])); err == nil {
				limit := 24 * time.Hour
				if kind == "air-sense-warning" {
					limit = 5 * time.Minute
				}
				fresh = "stale"
				if now.Sub(seen) <= limit {
					fresh = "fresh"
				}
			}
			if kind == "air-sense-warning" {
				expires, err := time.Parse(time.RFC3339Nano, fhString(r["expiresAt"]))
				if fhBool(r["expired"]) == true || err == nil && !expires.After(now) {
					fresh = "stale"
				}
			}
		}
		item["freshness"] = fresh
		result[key] = append(result[key].([]gin.H), item)
	}
	return result
}

func fhCapabilityStatus(v any) any {
	switch v {
	case "supported", "empty", "forbidden", "not_applicable", "unverified", "degraded", "failed":
		return v
	}
	return "failed"
}
func presentFHDiagnostics(d map[string][]gin.H) gin.H {
	connector := fhFields(d["access"][0], "id name status lastErrorCode:optional lastCheckedAt")
	watermarks := fhMap(d["watermarks"], "resourceKind:code status:code attemptCount lastErrorCode:optional lastStartedAt lastSucceededAt nextAttemptAt")
	capabilities := []gin.H{}
	for _, r := range d["capabilities"] {
		item := fhFields(r, "capabilityCode:code evidenceLevel:code region:code deployment:code deviceModel:optional firmwareVersion:optional endpointId:optional verifiedAt expiresAt")
		item["evidenceLevel"] = fhCode(r["evidenceLevel"], "documented")
		item["status"] = fhCapabilityStatus(r["status"])
		item["reason"] = fhCode(r["reason"], "diagnostic_unavailable")
		if r["expired"] == true {
			item["status"] = "unverified"
			item["reason"] = "evidence_expired"
		}
		layers := gin.H{}
		source, _ := r["layers"].(map[string]any)
		for _, key := range []string{"contract", "deployment", "account", "implementation", "acceptance"} {
			if value, ok := source[key]; ok {
				layers[key] = fhCapabilityStatus(value)
			}
		}
		item["layers"] = layers
		capabilities = append(capabilities, item)
	}
	return gin.H{"connector": connector, "resourceWatermarks": watermarks, "capabilities": capabilities}
}

var fhManagementSummaryFields = map[string]string{
	"organization":            "name status industryType industrySubtype measureUnits temperatureUnits mfaEnabled currentUserRole source",
	"organization-user":       "scope account accountSecond nickname role sourceType projectCount organizationName mfaEnabled source",
	"organization-role":       "name description roleType preset addToOrganization permissionCount source",
	"organization-permission": "sourceScope name description permissionType level visible basic childCount parentReference source",
	"project-user":            "account nickname callsign projectRole organizationRole callsignUpdated phoneFilled emailFilled source",
	"project-member":          "account projectCallsign projectRole organizationCallsign organizationRole online pendingOffline platform source",
}

func presentFHManagement(d map[string][]gin.H) gin.H {
	access := d["access"][0]
	var state any
	if len(d["state"]) > 0 {
		state = d["state"][0]
	}
	resources := []gin.H{}
	for _, row := range d["resources"] {
		fields, ok := fhManagementSummaryFields[fhString(row["kind"])]
		if !ok {
			continue
		}
		summary := gin.H{}
		source, _ := row["summary"].(map[string]any)
		for _, key := range strings.Fields(fields) {
			v, exists := source[key]
			if !exists {
				continue
			}
			switch v := v.(type) {
			case string:
				runes := utf16.Encode([]rune(v))
				if len(runes) > 512 {
					runes = runes[:512]
				}
				summary[key] = string(utf16.Decode(runes))
			case float64, bool, nil:
				summary[key] = v
			}
		}
		item := fhFields(row, "id connectorId kind status lastSeenAt missingAt")
		item["summary"] = summary
		resources = append(resources, item)
	}
	return gin.H{"connector": gin.H{"id": access["connectorId"], "name": access["connectorName"], "status": access["connectorStatus"]}, "managementCapabilityVerified": true, "syncState": state, "resources": resources}
}
