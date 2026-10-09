package httpapi

import (
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"regexp"
	"strconv"

	"github.com/gin-gonic/gin"
)

var deviceTypeIconName = regexp.MustCompile(`^[a-z][a-z0-9]*(-[a-z0-9]+)*$`)

func validDeviceTypeIcon(icon string) bool {
	return len(icon) <= 80 && deviceTypeIconName.MatchString(icon)
}

func (s *Server) deviceTypePresentationRoutes() {
	g := s.router.Group("/api/admin/device-types", s.requireUser, s.requireAdmin, s.timeout)
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
