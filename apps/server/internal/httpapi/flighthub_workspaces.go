package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
)

func (s *Server) fhWorkspace(kind string) gin.HandlerFunc {
	return func(c *gin.Context) {
		fail := func(status int, code string) {
			if kind == "Diagnostics" {
				c.Header("Cache-Control", "private, no-store, max-age=0")
				safe := "access_denied"
				status = 403
				if code == "NOT_FOUND" {
					safe = "connector_not_found"
					status = 404
				}
				c.JSON(status, gin.H{"error": gin.H{"code": safe}})
				return
			}
			if kind == "Management" {
				if !strings.HasPrefix(code, "FLIGHTHUB_MANAGEMENT_") {
					code = "FLIGHTHUB_MANAGEMENT_ACCESS_DENIED"
				}
				c.JSON(403, gin.H{"error": code})
				return
			}
			s.failure(c, status, "QUERY_FAILED")
		}
		cid := int64(0)
		if kind == "Diagnostics" || kind == "Management" {
			var err error
			cid, err = strconv.ParseInt(c.Param("connectorId"), 10, 64)
			if err != nil || cid <= 0 {
				fail(404, "NOT_FOUND")
				return
			}
		}
		s.scopedReadError(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			d, err := loadFHWorkspace(c.Request.Context(), q, kind, currentUser(c).ID, a.ProjectID, a.TeamID, cid)
			if err != nil {
				return nil, err
			}
			switch kind {
			case "FlightOps":
				return presentFHFlightOps(d), nil
			case "Geo":
				return presentFHGeo(a.ProjectID, d, time.Now()), nil
			case "Models":
				return presentFHModels(a.ProjectID, d), nil
			case "Diagnostics":
				return presentFHDiagnostics(d), nil
			case "Management":
				row := d["access"][0]
				if row["role"] != "owner" && row["role"] != "admin" || row["connectorStatus"] != "connected" || row["managementCapabilityVerified"] != true {
					return nil, errors.New("FLIGHTHUB_MANAGEMENT_PERMISSION_DENIED")
				}
				return presentFHManagement(d), nil
			}
			return nil, errors.New("invalid workspace")
		}, fail)
	}
}
