package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/flighthub"
	"context"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strings"
)

type fhJoinCodeClient interface {
	GetJoinCodeInfo(context.Context, string, flighthub.JoinCodeQuery) (flighthub.JoinCodeInfo, error)
}

func (s *Server) fhJoinCode(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store, max-age=0")
	fail := func(code string) { c.JSON(403, gin.H{"error": code}) }
	pid, e := projectID(c)
	if e != nil {
		c.JSON(400, gin.H{"error": "INPUT_INVALID"})
		return
	}
	cid, e := connectorID(c)
	if e != nil {
		c.JSON(400, gin.H{"error": "INPUT_INVALID"})
		return
	}
	var body map[string]any
	if json.NewDecoder(c.Request.Body).Decode(&body) != nil || body == nil {
		fail("INPUT_INVALID")
		return
	}
	for k := range body {
		if k != "projectCode" && k != "fastJoinCode" && k != "associationDroneSN" {
			fail("INPUT_INVALID")
			return
		}
	}
	for _, k := range []string{"projectCode", "fastJoinCode", "associationDroneSN"} {
		v, exists := body[k]
		if k == "associationDroneSN" && !exists {
			continue
		}
		s, ok := v.(string)
		s = strings.TrimSpace(s)
		if !ok || !fhPayloadIndex.MatchString(s) {
			fail("INPUT_INVALID")
			return
		}
		body[k] = s
	}
	ctx := c.Request.Context()
	a, e := s.projectAccess(ctx, s.queries, currentUser(c).ID, pid, "project:view")
	if e != nil {
		fail("FLIGHTHUB_MANAGEMENT_PERMISSION_DENIED")
		return
	}
	raw, e := s.queries.FHJoinCodeAccess(ctx, sqlcgen.FHJoinCodeAccessParams{P1: currentUser(c).ID, P2: pid, P3: cid})
	row, e := fhFirstRow(raw, e, "FLIGHTHUB_MANAGEMENT_NOT_FOUND")
	if e != nil {
		fail("FLIGHTHUB_MANAGEMENT_NOT_FOUND")
		return
	}
	if row["role"] != "owner" && row["role"] != "admin" {
		fail("FLIGHTHUB_MANAGEMENT_PERMISSION_DENIED")
		return
	}
	if row["connectorProjectId"] != float64(pid) || row["connectorTeamId"] != float64(a.TeamID) {
		fail("FLIGHTHUB_MANAGEMENT_SCOPE_MISMATCH")
		return
	}
	if row["connectorStatus"] != "connected" {
		fail("FLIGHTHUB_MANAGEMENT_CONNECTOR_OFFLINE")
		return
	}
	if row["managementCapabilityVerified"] != true {
		fail("FLIGHTHUB_MANAGEMENT_CAPABILITY_REQUIRED")
		return
	}
	if s.flightHub == nil {
		fail("FLIGHTHUB_MANAGEMENT_LOOKUP_FAILED")
		return
	}
	client, ok := s.flightHub.client.(fhJoinCodeClient)
	if !ok {
		fail("FLIGHTHUB_MANAGEMENT_LOOKUP_FAILED")
		return
	}
	rawJSON, _ := json.Marshal(row["credentialEnvelope"])
	var envelope credentials.Envelope
	var credential struct {
		Token string `json:"token"`
	}
	if json.Unmarshal(rawJSON, &envelope) != nil || credentials.DecryptJSON(envelope, s.flightHub.secret, credentials.AAD("device-adapter", cid, pid), &credential) != nil || strings.TrimSpace(credential.Token) == "" {
		fail("FLIGHTHUB_MANAGEMENT_CREDENTIAL_UNAVAILABLE")
		return
	}
	result, e := client.GetJoinCodeInfo(ctx, strings.TrimSpace(credential.Token), flighthub.JoinCodeQuery{ProjectID: fhString(body["projectCode"]), FastJoinCode: fhString(body["fastJoinCode"]), AssociationDroneSN: fhString(body["associationDroneSN"])})
	if e != nil {
		fail("FLIGHTHUB_MANAGEMENT_LOOKUP_FAILED")
		return
	}
	if result.ProjectUUID != strings.ToLower(fhString(row["projectUuid"])) || result.OrganizationUUID != strings.ToLower(fhString(row["organizationUuid"])) {
		fail("FLIGHTHUB_MANAGEMENT_JOIN_CODE_SCOPE_MISMATCH")
		return
	}
	c.JSON(200, gin.H{"projectName": result.ProjectName, "organizationName": result.OrganizationName, "userInOrganization": result.UserInOrganization, "recommendedUserCallsign": result.RecommendUserProjectCallsign, "recommendedDroneCallsign": result.RecommendAssociationDroneCallsign})
}
