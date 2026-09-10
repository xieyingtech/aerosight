package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"github.com/gin-gonic/gin"
	"strconv"
)

func (s *Server) fhLifecycleMain(c *gin.Context) {
	a, ok := s.flightHubManager(c)
	if !ok {
		return
	}
	id, err := connectorID(c)
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	reconnect := c.Request.Method == "PUT"
	action, trigger := "connector.flighthub.capability.probe", "capability-probe"
	input := gin.H{"upstreamMethods": []string{"GET"}}
	if reconnect {
		action, trigger = "connector.flighthub.reconnect", "reconnect"
		input = gin.H{"operation": "enable"}
	}
	audit := flightHubAudit(c, a, action, id, input)
	if reconnect {
		audit.PolicyResult["credentialsUnchanged"] = true
		audit.PolicyResult["readOnlySync"] = true
	} else {
		audit.PolicyResult["readOnlyUpstream"] = true
	}
	result, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (gin.H, error) {
		ctx, q := c.Request.Context(), w.Queries
		locked, e := q.LockFlightHubConnector(ctx, sqlcgen.LockFlightHubConnectorParams{ID: id, ProjectID: a.ProjectID})
		if e != nil {
			return nil, e
		}
		count := int64(0)
		if reconnect {
			if locked.Status != "disabled" {
				return nil, &flightHubError{code: "connector_not_disabled"}
			}
			if e = q.FHReconnect(ctx, sqlcgen.FHReconnectParams{ID: id, ProjectID: a.ProjectID}); e != nil {
				return nil, e
			}
			count, e = q.FHRestoreBindings(ctx, sqlcgen.FHRestoreBindingsParams{ProjectID: a.ProjectID, ConnectorInstanceID: id})
			if e != nil {
				return nil, e
			}
		} else {
			if locked.Status == "disabled" {
				return nil, &flightHubError{code: "connector_disabled"}
			}
			if e = q.FHExpireReadCapabilities(ctx, sqlcgen.FHExpireReadCapabilitiesParams{ProjectID: a.ProjectID, ConnectorInstanceID: id}); e != nil {
				return nil, e
			}
		}
		out, e := queueFlightHubSync(ctx, w, a, id, trigger)
		if e != nil {
			return nil, e
		}
		out["id"] = strconv.FormatInt(id, 10)
		if reconnect {
			out["status"], out["reconnected"], out["restoredBindingCount"], out["syncQueued"] = "connecting", true, count, true
		} else {
			out["probeQueued"], out["readOnlyUpstream"] = true, true
		}
		return out, nil
	})
	if err != nil {
		s.flightHubFailure(c, err, false)
		return
	}
	status := 202
	if reconnect {
		status = 200
	}
	c.JSON(status, result)
}
