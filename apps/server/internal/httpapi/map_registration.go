package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"github.com/gin-gonic/gin"
	"strings"
)

func (s *Server) mapRegistrationRoutes() {
	g := s.router.Group("/api/projects/:id", s.requireUser, s.timeout)
	g.POST("/map-regions", func(c *gin.Context) { s.registerMapObject(c, true) })
	g.POST("/devices/register", func(c *gin.Context) { s.registerMapObject(c, false) })
}

type mapRegistrationInput struct {
	Name            string          `json:"name"`
	RegistrationKey string          `json:"registrationKey"`
	Geometry        json.RawMessage `json:"geometry,omitempty"`
	DeviceTypeKey   string          `json:"deviceTypeKey,omitempty"`
	Position        *struct {
		Longitude *float64 `json:"longitude"`
		Latitude  *float64 `json:"latitude"`
	} `json:"position,omitempty"`
}

func validRegionGeometry(raw json.RawMessage) bool {
	var geo struct {
		Type        string          `json:"type"`
		Coordinates json.RawMessage `json:"coordinates"`
	}
	if json.Unmarshal(raw, &geo) != nil {
		return false
	}
	var polys [][][][]float64
	switch geo.Type {
	case "Polygon":
		var polygon [][][]float64
		if json.Unmarshal(geo.Coordinates, &polygon) != nil {
			return false
		}
		polys = append(polys, polygon)
	case "MultiPolygon":
		if json.Unmarshal(geo.Coordinates, &polys) != nil {
			return false
		}
	default:
		return false
	}
	if len(polys) == 0 || len(polys) > 100 {
		return false
	}
	count := 0
	for _, poly := range polys {
		if len(poly) == 0 {
			return false
		}
		for _, ring := range poly {
			if len(ring) < 4 {
				return false
			}
			for _, p := range ring {
				count++
				if count > 10000 || len(p) != 2 || p[0] < -180 || p[0] > 180 || p[1] < -90 || p[1] > 90 {
					return false
				}
			}
			if ring[0][0] != ring[len(ring)-1][0] || ring[0][1] != ring[len(ring)-1][1] {
				return false
			}
		}
	}
	return true
}

func (s *Server) registerMapObject(c *gin.Context, region bool) {
	var input mapRegistrationInput
	if strictJSON(c, &input) != nil || strings.TrimSpace(input.Name) == "" || utf16Length(input.Name) > 100 || strings.TrimSpace(input.RegistrationKey) == "" || len(input.RegistrationKey) > 100 {
		s.failure(c, 400, "MAP_REGISTRATION_INPUT_INVALID")
		return
	}
	if region {
		if !validRegionGeometry(input.Geometry) {
			s.failure(c, 400, "MAP_REGION_INPUT_INVALID")
			return
		}
	} else if input.Position == nil || input.Position.Longitude == nil || input.Position.Latitude == nil || *input.Position.Longitude < -180 || *input.Position.Longitude > 180 || *input.Position.Latitude < -90 || *input.Position.Latitude > 90 {
		s.failure(c, 400, "MAP_DEVICE_INPUT_INVALID")
		return
	}
	a, err := s.adapterManager(c)
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	action, resource := "device.register", "device"
	if region {
		action, resource = "map_region.register", "map_region"
	}
	audit := adapterAudit(c, a, action, 0, input)
	audit.ResourceType = resource
	id, err := database.AuditedWrite(c.Request.Context(), s.db, audit, s.authorizeWrite(currentUser(c).ID, a.ProjectID, a.TeamID, "device:configure", true), func(w *database.WriteTx) (int64, error) {
		if region {
			return w.Queries.RegisterMapRegion(c.Request.Context(), sqlcgen.RegisterMapRegionParams{ProjectID: a.ProjectID, TeamID: a.TeamID, RegistrationKey: input.RegistrationKey, Name: input.Name, GeometryJson: string(input.Geometry)})
		}
		config, _ := json.Marshal(gin.H{"registeredPosition": input.Position})
		id, e := w.Queries.RegisterMapDevice(c.Request.Context(), sqlcgen.RegisterMapDeviceParams{ProjectID: a.ProjectID, RegistrationKey: sql.NullString{String: input.RegistrationKey, Valid: true}, Name: input.Name, TypeKey: input.DeviceTypeKey, ConfigJson: config})
		return int64(id), e
	})
	if err != nil {
		s.failure(c, 400, "MAP_REGISTRATION_FAILED")
		return
	}
	c.JSON(201, gin.H{"id": id, "projectId": a.ProjectID})
}

func applyRegisteredPosition(row gin.H) {
	position, ok := row["registeredPosition"].(map[string]any)
	delete(row, "registeredPosition")
	if !ok || row["pose"] != nil {
		return
	}
	row["pose"] = position
	row["positionStatus"] = "registered"
	row["positionReason"] = "手动登记位置"
	row["positionSource"] = "manual-registration"
}
