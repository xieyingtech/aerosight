package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/device"
	"database/sql"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (s *Server) testDeviceAdapterConnection(c *gin.Context) {
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
	id, err := strconv.ParseInt(c.Param("adapterId"), 10, 64)
	if err != nil || id <= 0 {
		fail("DEVICE_ADAPTER_NOT_FOUND")
		return
	}
	row, err := s.queries.GetAdapterNetworkProfile(c.Request.Context(), sqlcgen.GetAdapterNetworkProfileParams{ProjectID: a.ProjectID, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		fail("DEVICE_ADAPTER_NOT_FOUND")
		return
	}
	if err != nil {
		fail("DEVICE_ADAPTER_TEST_FAILED")
		return
	}
	profile := device.NetworkProfile{Mode: row.Mode.String, TLSRequired: row.TlsRequired.Bool, CredentialProvided: row.HasCredential, Endpoints: map[string]string{"mqttEndpoint": row.MqttEndpoint.String, "apiPublicBaseUrl": row.ApiPublicBaseUrl.String, "websocketPublicUrl": row.WebsocketPublicUrl.String, "mediaIngestBaseUrl": row.MediaIngestBaseUrl.String, "mediaPlaybackBaseUrl": row.MediaPlaybackBaseUrl.String}}
	var config map[string]any
	if row.ConfigJson.Valid {
		if err = json.Unmarshal(row.ConfigJson.RawMessage, &config); err != nil {
			fail("DEVICE_ADAPTER_TEST_FAILED")
			return
		}
	}
	profile.MQTTAnonymous = config["mqttAnonymous"] == true
	complete := row.NetworkProfileID.Valid && profile.Mode != ""
	for _, value := range profile.Endpoints {
		if value == "" {
			complete = false
		}
	}
	var health any
	status := ""
	switch {
	case !complete && row.AdapterType == "simulator":
		health = gin.H{"ok": true, "code": "SIMULATOR_READY", "serverVerification": "not_applicable", "deviceVerification": "pending"}
	case !complete:
		health = gin.H{"ok": false, "code": "NETWORK_PROFILE_REQUIRED", "serverVerification": "failed", "deviceVerification": "pending"}
	default:
		result := device.CheckNetworkConnection(c.Request.Context(), profile, s.networkResolver, s.networkProbe)
		status = result.Status
		health = result
	}
	audit := adapterAudit(c, a, "device_adapter.test_connection", id, gin.H{})
	delete(audit.PolicyResult, "role")
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (any, error) {
		raw, e := json.Marshal(health)
		if e != nil {
			return nil, e
		}
		if row.NetworkProfileID.Valid && status != "" {
			if e = w.Queries.RecordNetworkValidation(c.Request.Context(), sqlcgen.RecordNetworkValidationParams{ProjectID: a.ProjectID, ID: row.NetworkProfileID.Int64, Status: status, LastValidationJson: raw}); e != nil {
				return nil, e
			}
		}
		if e = w.Queries.RecordAdapterHealth(c.Request.Context(), sqlcgen.RecordAdapterHealthParams{ProjectID: a.ProjectID, ID: id, LastHealthJson: raw}); e != nil {
			return nil, e
		}
		return health, nil
	})
	if err != nil {
		if err.Error() == "PROJECT_ACCESS_DENIED" {
			fail("PROJECT_ACCESS_DENIED")
		} else {
			fail("DEVICE_ADAPTER_TEST_FAILED")
		}
		return
	}
	c.JSON(200, result)
}
