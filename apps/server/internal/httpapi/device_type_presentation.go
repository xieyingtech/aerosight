package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"sort"
	"strconv"

	"github.com/gin-gonic/gin"
)

var deviceTypeIconName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

func validDeviceTypeIcon(icon string) bool {
	return len(icon) <= 80 && deviceTypeIconName.MatchString(icon)
}

func (s *Server) deviceTypePresentationRoutes() {
	g := s.router.Group("/api/admin/device-types", s.requireUser, s.requireAdmin, s.timeout)
	g.GET("/catalog", s.devicePlatformCatalog)
	g.GET("", func(c *gin.Context) {
		rows, err := s.queries.ListDeviceTypePresentation(c.Request.Context())
		if err != nil {
			s.failure(c, 500, "DEVICE_TYPES_FAILED")
			return
		}
		result := make([]json.RawMessage, 0, len(rows))
		for _, row := range rows {
			result = append(result, json.RawMessage(row))
		}
		c.JSON(200, result)
	})
	g.PATCH("/:typeId", func(c *gin.Context) {
		id, err := strconv.ParseInt(c.Param("typeId"), 10, 64)
		if err != nil || id <= 0 {
			s.failure(c, 400, "DEVICE_TYPE_ID_INVALID")
			return
		}
		var input struct {
			Icon string `json:"icon"`
		}
		if err := c.ShouldBindJSON(&input); err != nil || !validDeviceTypeIcon(input.Icon) {
			s.failure(c, 400, "DEVICE_TYPE_ICON_INVALID")
			return
		}
		_, err = s.queries.UpdateDeviceTypeIcon(c.Request.Context(), sqlcgen.UpdateDeviceTypeIconParams{ID: id, Icon: input.Icon})
		if errors.Is(err, sql.ErrNoRows) {
			s.failure(c, 404, "DEVICE_TYPE_NOT_FOUND")
			return
		}
		if err != nil {
			s.failure(c, 500, "DEVICE_TYPE_UPDATE_FAILED")
			return
		}
		c.JSON(200, gin.H{"id": strconv.FormatInt(id, 10), "icon": input.Icon})
	})
}

// Definitions describe supported keys, not a particular device's live state.
func (s *Server) devicePlatformCatalog(c *gin.Context) {
	raw, err := s.queries.ListDeviceCatalogDrivers(c.Request.Context())
	if err != nil {
		s.failure(c, 500, "DEVICE_CATALOG_FAILED")
		return
	}
	drivers, err := decodeSnapshotRows(raw)
	if err != nil {
		s.failure(c, 500, "DEVICE_CATALOG_FAILED")
		return
	}
	capabilities, streams := []gin.H{}, []gin.H{}
	for _, d := range drivers {
		manifest, _ := d["manifest"].(map[string]any)
		appendDefinition := func(value map[string]any, target *[]gin.H) {
			row := gin.H{}
			for key, v := range value {
				row[key] = v
			}
			row["driverKey"] = d["driverKey"]
			row["driverVersion"] = d["version"]
			row["driverStatus"] = d["status"]
			*target = append(*target, row)
		}
		switch defs := manifest["capabilities"].(type) {
		case []any:
			for _, def := range defs {
				if v, ok := def.(map[string]any); ok {
					appendDefinition(v, &capabilities)
				}
			}
		case map[string]any:
			keys := []string{}
			for key := range defs {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				v := map[string]any{"code": key}
				if params, ok := defs[key].(map[string]any); ok {
					for k, value := range params {
						v[k] = value
					}
				}
				v["code"] = key
				appendDefinition(v, &capabilities)
			}
		}
		if defs, ok := manifest["streams"].([]any); ok {
			for _, def := range defs {
				if v, ok := def.(map[string]any); ok {
					appendDefinition(v, &streams)
				}
			}
		}
	}
	var catalog map[string][]gin.H
	if json.Unmarshal(actionCatalog, &catalog) != nil {
		s.failure(c, 500, "DEVICE_CATALOG_FAILED")
		return
	}
	actions := []gin.H{}
	keys := []string{}
	for key := range catalog {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		for _, definition := range catalog[key] {
			row := gin.H{"capabilityCode": key}
			for k, v := range definition {
				row[k] = v
			}
			actions = append(actions, row)
		}
	}
	c.JSON(200, gin.H{"drivers": drivers, "capabilities": capabilities, "streams": streams, "actions": actions})
}
