package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/device"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"math"
	"strconv"
	"strings"
	"time"
)

type djiSetupInput struct {
	Name, NTPHost string
	NTPPort       int
	Serials       []string
	Secrets       map[string]string
	Profile       device.NetworkProfile
	Audit         gin.H
}

func parseDJISetup(c *gin.Context) (djiSetupInput, error) {
	var out djiSetupInput
	bad := errors.New("DJI_ADAPTER_SETUP_INVALID")
	var body map[string]any
	if strictJSON(c, &body) != nil {
		return out, bad
	}
	var ok bool
	out.Name, ok = adapterString(body, "name", 1, 100)
	if !ok {
		return out, bad
	}
	out.NTPHost, ok = adapterString(body, "ntpServerHost", 1, 253)
	if !ok {
		return out, bad
	}
	out.Profile.Mode, _ = body["mode"].(string)
	if out.Profile.Mode != "lan" && out.Profile.Mode != "public" {
		return out, bad
	}
	out.Profile.TLSRequired, ok = body["tlsRequired"].(bool)
	if !ok {
		return out, bad
	}
	if v, exists := body["mqttAnonymous"]; exists {
		out.Profile.MQTTAnonymous, ok = v.(bool)
		if !ok {
			return out, bad
		}
	}
	out.Profile.CredentialProvided = true
	out.Profile.Endpoints = map[string]string{}
	for _, key := range device.NetworkEndpointFields {
		v, ok := adapterString(body, key, 1, 500)
		if !ok {
			return out, bad
		}
		out.Profile.Endpoints[key] = v
	}
	out.Secrets = map[string]string{}
	for key, limit := range djiCredentialLimits {
		v, ok := body[key].(string)
		if limit == 255 {
			v = strings.TrimSpace(v)
		}
		if !ok || utf16Length(v) < 1 || utf16Length(v) > limit {
			return out, bad
		}
		out.Secrets[key] = v
	}
	values, ok := body["gatewaySerials"].([]any)
	if !ok || len(values) < 1 || len(values) > 100 {
		return out, bad
	}
	for _, value := range values {
		v, ok := value.(string)
		v = strings.TrimSpace(v)
		if !ok || utf16Length(v) < 1 || utf16Length(v) > 100 {
			return out, bad
		}
		out.Serials = append(out.Serials, v)
	}
	port := float64(123)
	if v, present := body["ntpServerPort"]; present {
		// Match z.coerce.number() for JSON scalar inputs.
		switch n := v.(type) {
		case float64:
			port = n
		case string:
			var e error
			port, e = strconv.ParseFloat(strings.TrimSpace(n), 64)
			if e != nil {
				return out, bad
			}
		case bool:
			port = 0
			if n {
				port = 1
			}
		case nil:
			port = 0
		default:
			return out, bad
		}
	}
	if math.IsNaN(port) || port < 1 || port > 65535 || math.Trunc(port) != port {
		return out, bad
	}
	out.NTPPort = int(port)
	out.Audit = gin.H{"name": out.Name, "mode": out.Profile.Mode, "tlsRequired": out.Profile.TLSRequired, "mqttAnonymous": out.Profile.MQTTAnonymous, "ntpServerHost": out.NTPHost, "ntpServerPort": out.NTPPort, "gatewaySerials": out.Serials}
	for key, value := range out.Profile.Endpoints {
		out.Audit[key] = value
	}
	return out, nil
}
func (s *Server) createDJISetup(c *gin.Context) {
	fail := func(code string) {
		status := 400
		if code == "PROJECT_ACCESS_DENIED" {
			status = 403
		}
		s.failure(c, status, code)
	}
	a, err := s.adapterManager(c)
	if err != nil {
		fail("PROJECT_ACCESS_DENIED")
		return
	}
	input, err := parseDJISetup(c)
	if err != nil {
		fail("DJI_ADAPTER_SETUP_INVALID")
		return
	}
	validation := device.ValidateNetworkProfile(c.Request.Context(), input.Profile, s.networkResolver)
	if !validation.Valid {
		c.JSON(400, gin.H{"error": "NETWORK_PROFILE_INVALID", "issues": validation.Issues})
		return
	}
	topics := []string{}
	for _, serial := range input.Serials {
		topics = append(topics, "sys/product/"+serial+"/status")
		for _, suffix := range []string{"state", "osd", "events", "requests", "services_reply"} {
			topics = append(topics, "thing/product/"+serial+"/"+suffix)
		}
	}
	clientID := "aerosight-" + uuid.NewString()
	audit := adapterAudit(c, a, "device_adapter.dji_setup", 0, input.Audit)
	audit.PolicyResult["networkPolicy"] = "valid"
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		p := input.Profile
		raw, _ := json.Marshal(gin.H{"mqttAnonymous": p.MQTTAnonymous})
		endpoint := func(key string) sql.NullString { return sql.NullString{String: p.Endpoints[key], Valid: true} }
		profileID, e := w.Queries.InsertDJINetworkProfile(c.Request.Context(), sqlcgen.InsertDJINetworkProfileParams{ProjectID: a.ProjectID, TeamID: a.TeamID, Name: input.Name + " Network", Mode: p.Mode, MqttEndpoint: endpoint("mqttEndpoint"), ApiPublicBaseUrl: endpoint("apiPublicBaseUrl"), WebsocketPublicUrl: endpoint("websocketPublicUrl"), MediaIngestBaseUrl: endpoint("mediaIngestBaseUrl"), MediaPlaybackBaseUrl: endpoint("mediaPlaybackBaseUrl"), TlsRequired: p.TLSRequired, ConfigJson: raw})
		if e != nil {
			return nil, e
		}
		raw, _ = json.Marshal(gin.H{"clientId": clientID, "gatewaySerials": input.Serials, "topics": topics, "djiConfiguration": gin.H{"ntpServerHost": input.NTPHost, "ntpServerPort": input.NTPPort}})
		rowRaw, e := w.Queries.InsertDJISetupAdapter(c.Request.Context(), sqlcgen.InsertDJISetupAdapterParams{ProjectID: a.ProjectID, TeamID: a.TeamID, Name: input.Name, ConfigJson: raw, NetworkProfileID: sql.NullInt64{Int64: profileID, Valid: true}})
		if e != nil {
			return nil, e
		}
		var row gin.H
		if e = json.Unmarshal(rowRaw, &row); e != nil {
			return nil, e
		}
		id, e := strconv.ParseInt(row["id"].(string), 10, 64)
		if e != nil {
			return nil, e
		}
		envelope, e := credentials.EncryptJSON(input.Secrets, s.credentialSecret, credentials.AAD("device-adapter", id, a.ProjectID))
		if e != nil {
			return nil, e
		}
		raw, e = json.Marshal(envelope)
		if e != nil {
			return nil, e
		}
		if e = w.Queries.StoreDeviceAdapterEnvelope(c.Request.Context(), sqlcgen.StoreDeviceAdapterEnvelopeParams{ProjectID: a.ProjectID, ID: id, Envelope: raw}); e != nil {
			return nil, e
		}
		date, e := time.Parse(time.RFC3339Nano, row["updatedAt"].(string))
		if e != nil {
			return nil, e
		}
		row["updatedAt"] = date
		row["network"] = gin.H{"mode": p.Mode, "status": "unverified"}
		row["configurationSummary"] = gin.H{"gateway_sn": input.Serials, "mqtt_broker": gin.H{"address": p.Endpoints["mqttEndpoint"], "client_id": clientID, "username": "[ENCRYPTED]", "password": "[ENCRYPTED]", "enable_tls": strings.HasPrefix(p.Endpoints["mqttEndpoint"], "mqtts://")}, "config": gin.H{"app_id": "[ENCRYPTED]", "app_key": "[ENCRYPTED]", "app_license": "[ENCRYPTED]", "ntp_server_host": input.NTPHost, "ntp_server_port": input.NTPPort}}
		return row, nil
	})
	if err != nil {
		if err.Error() == "PROJECT_ACCESS_DENIED" {
			fail("PROJECT_ACCESS_DENIED")
		} else {
			fail("DJI_ADAPTER_SETUP_INVALID")
		}
		return
	}
	result["updatedAt"] = timestamp(result["updatedAt"].(time.Time))
	c.JSON(201, result)
}
