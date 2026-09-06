package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Configure before serving requests; the same key is used by the background drivers.
func (s *Server) AttachDeviceCredentials(secret string) { s.credentialSecret = secret }

func (s *Server) deviceAdapterRoutes() {
	g := s.router.Group("/api/projects/:id/device-adapters", s.requireUser, s.timeout)
	g.GET("", s.listDeviceAdapters)
	g.POST("", s.createDeviceAdapter)
	g.PATCH("/:adapterId", s.updateDeviceAdapter)
}

func (s *Server) adapterManager(c *gin.Context) (sqlcgen.GetProjectAccessRow, error) {
	var a sqlcgen.GetProjectAccessRow
	pid, err := projectID(c)
	if err != nil {
		return a, err
	}
	a, err = s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "device:configure")
	if err == nil && a.Role != "owner" && a.Role != "admin" {
		err = errors.New("PROJECT_ACCESS_DENIED")
	}
	return a, err
}
func (s *Server) listDeviceAdapters(c *gin.Context) {
	a, err := s.adapterManager(c)
	if err == nil {
		raw, e := s.queries.ListDeviceAdapters(c.Request.Context(), a.ProjectID)
		err = e
		if err == nil {
			rows, e := decodeSnapshotRows(raw)
			err = e
			if err == nil {
				c.JSON(200, rows)
				return
			}
		}
	}
	s.failure(c, 403, "Unable to access device adapters")
}

var inlineSecretKey = regexp.MustCompile(`(?i)authorization|cookie|credential|password|secret|token|api[-_]?key`)

func containsInlineSecret(value any) bool {
	switch v := value.(type) {
	case map[string]any:
		for key, item := range v {
			if inlineSecretKey.MatchString(key) || containsInlineSecret(item) {
				return true
			}
		}
	case []any:
		for _, item := range v {
			if containsInlineSecret(item) {
				return true
			}
		}
	}
	return false
}
func adapterString(body map[string]any, key string, min, max int) (string, bool) {
	value, ok := body[key].(string)
	value = strings.TrimSpace(value)
	return value, ok && utf16Length(value) >= min && utf16Length(value) <= max
}
func adapterAudit(c *gin.Context, a sqlcgen.GetProjectAccessRow, action string, id int64, input any) database.AuditContext {
	resourceID := ""
	if id > 0 {
		resourceID = strconv.FormatInt(id, 10)
	}
	return database.AuditContext{ProjectID: a.ProjectID, TeamID: a.TeamID, ActorUserID: currentUser(c).ID, RequestID: c.GetHeader("X-Request-ID"), Action: action, ResourceType: "device_adapter", ResourceID: resourceID, Input: input, PolicyResult: map[string]any{"permission": "device:configure", "role": a.Role}}
}

func (s *Server) createDeviceAdapter(c *gin.Context) {
	fail := func() { s.failure(c, 400, "Invalid or unauthorized device adapter request") }
	a, err := s.adapterManager(c)
	if err != nil {
		fail()
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil || body == nil {
		fail()
		return
	}
	name, ok := adapterString(body, "name", 1, 100)
	if !ok {
		fail()
		return
	}
	kind, _ := body["adapterType"].(string)
	if kind != "simulator" && kind != "dji" {
		fail()
		return
	}
	version := "1"
	if _, present := body["protocolVersion"]; present {
		version, ok = adapterString(body, "protocolVersion", 1, 50)
		if !ok {
			fail()
			return
		}
	}
	vendor := sql.NullString{}
	if _, present := body["vendor"]; present {
		v, ok := adapterString(body, "vendor", 0, 100)
		if !ok {
			fail()
			return
		}
		vendor = sql.NullString{String: v, Valid: true}
	}
	config := map[string]any{}
	if v, present := body["config"]; present {
		config, ok = v.(map[string]any)
		if !ok || config == nil {
			fail()
			return
		}
	}
	if containsInlineSecret(config) {
		fail()
		return
	}
	secrets := map[string]string{}
	if v, present := body["credentials"]; present {
		values, ok := v.(map[string]any)
		if !ok || values == nil {
			fail()
			return
		}
		for key, value := range values {
			text, ok := value.(string)
			if !ok || utf16Length(text) > 16384 {
				fail()
				return
			}
			secrets[key] = text
		}
	}
	input := gin.H{"name": name, "adapterType": kind, "protocolVersion": version, "config": config}
	if vendor.Valid {
		input["vendor"] = vendor.String
	}
	audit := adapterAudit(c, a, "device_adapter.create", 0, input)
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		rawConfig, e := json.Marshal(config)
		if e != nil {
			return nil, e
		}
		raw, e := w.Queries.InsertDeviceAdapter(c.Request.Context(), sqlcgen.InsertDeviceAdapterParams{ProjectID: a.ProjectID, TeamID: a.TeamID, Name: name, AdapterType: kind, Vendor: vendor, ProtocolVersion: version, ConfigJson: rawConfig})
		if e != nil {
			return nil, e
		}
		var row gin.H
		if e = json.Unmarshal(raw, &row); e != nil {
			return nil, e
		}
		id, e := strconv.ParseInt(row["id"].(string), 10, 64)
		if e != nil {
			return nil, e
		}
		if len(secrets) > 0 {
			envelope, e := credentials.EncryptJSON(secrets, s.credentialSecret, credentials.AAD("device-adapter", id, a.ProjectID))
			if e != nil {
				return nil, e
			}
			raw, e := json.Marshal(envelope)
			if e != nil {
				return nil, e
			}
			if e = w.Queries.StoreDeviceAdapterEnvelope(c.Request.Context(), sqlcgen.StoreDeviceAdapterEnvelopeParams{ProjectID: a.ProjectID, ID: id, Envelope: raw}); e != nil {
				return nil, e
			}
		}
		// Preserve the legacy audit hashing of a node-postgres Date as an empty object.
		date, e := time.Parse(time.RFC3339Nano, row["updatedAt"].(string))
		if e != nil {
			return nil, e
		}
		row["updatedAt"] = date
		return row, nil
	})
	if err != nil {
		fail()
		return
	}
	result["updatedAt"] = timestamp(result["updatedAt"].(time.Time))
	c.JSON(201, result)
}

var djiCredentialLimits = map[string]int{"mqttUsername": 255, "mqttPassword": 16384, "appId": 255, "appKey": 16384, "appLicense": 16384, "mediaPublishUser": 255, "mediaPublishPassword": 16384}

func (s *Server) updateDeviceAdapter(c *gin.Context) {
	fail := func() { s.failure(c, 400, "Invalid or unauthorized adapter update") }
	a, err := s.adapterManager(c)
	if err != nil {
		fail()
		return
	}
	id, err := strconv.ParseInt(c.Param("adapterId"), 10, 64)
	if err != nil || id <= 0 {
		fail()
		return
	}
	var body map[string]any
	if strictJSON(c, &body) != nil {
		fail()
		return
	}
	if value, present := body["credentials"]; present {
		values, ok := value.(map[string]any)
		if !ok || values == nil {
			fail()
			return
		}
		updates := map[string]string{}
		fields := []string{}
		for key, value := range values {
			limit, known := djiCredentialLimits[key]
			text, ok := value.(string)
			if !known || !ok || utf16Length(text) > limit {
				fail()
				return
			}
			if strings.TrimSpace(text) != "" {
				updates[key] = text
				fields = append(fields, key)
			}
		}
		// Empty credential fields mean "keep existing", including the historical no-op response.
		if len(updates) == 0 {
			c.JSON(200, gin.H{"id": id, "updated": false})
			return
		}
		sort.Strings(fields)
		audit := adapterAudit(c, a, "device_adapter.credentials.update", id, gin.H{"fieldsUpdated": fields})
		result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
			raw, e := w.Queries.LockDJIAdapterEnvelope(c.Request.Context(), sqlcgen.LockDJIAdapterEnvelopeParams{ProjectID: a.ProjectID, ID: id})
			if e != nil {
				return nil, e
			}
			merged := map[string]string{}
			aad := credentials.AAD("device-adapter", id, a.ProjectID)
			if raw.Valid {
				var envelope credentials.Envelope
				if e = json.Unmarshal(raw.RawMessage, &envelope); e != nil {
					return nil, e
				}
				if e = credentials.DecryptJSON(envelope, s.credentialSecret, aad, &merged); e != nil {
					return nil, e
				}
			}
			if merged == nil {
				merged = map[string]string{}
			}
			for key, value := range updates {
				merged[key] = value
			}
			envelope, e := credentials.EncryptJSON(merged, s.credentialSecret, aad)
			if e != nil {
				return nil, e
			}
			bytes, e := json.Marshal(envelope)
			if e != nil {
				return nil, e
			}
			if e = w.Queries.UpdateDJIAdapterEnvelope(c.Request.Context(), sqlcgen.UpdateDJIAdapterEnvelopeParams{ProjectID: a.ProjectID, ID: id, Envelope: bytes}); e != nil {
				return nil, e
			}
			return gin.H{"id": id, "updated": true}, nil
		})
		if err != nil {
			fail()
			return
		}
		c.JSON(200, result)
		return
	}
	enabled, ok := body["enabled"].(bool)
	if !ok {
		fail()
		return
	}
	audit := adapterAudit(c, a, "device_adapter.set_enabled", id, gin.H{"enabled": enabled})
	delete(audit.PolicyResult, "role")
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (sqlcgen.SetDeviceAdapterEnabledRow, error) {
		status := "disabled"
		if enabled {
			status = "connecting"
		}
		return w.Queries.SetDeviceAdapterEnabled(c.Request.Context(), sqlcgen.SetDeviceAdapterEnabledParams{ProjectID: a.ProjectID, ID: id, Status: status})
	})
	if err != nil {
		fail()
		return
	}
	c.JSON(200, result)
}
