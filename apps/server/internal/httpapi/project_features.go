package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"strconv"

	"github.com/gin-gonic/gin"
)

type featureNode struct {
	ID          string        `json:"id"`
	Label       string        `json:"label"`
	Description string        `json:"description,omitempty"`
	Children    []featureNode `json:"children,omitempty"`
}

// Groups are presentation and batch-selection boundaries, not inherited grants.
// Leaf IDs map to existing flags so upgrading preserves every saved setting.
var projectFeatureTree = []featureNode{
	{ID: "general", Label: "基础功能", Children: []featureNode{
		{ID: "operations.overview", Label: "运行总览"},
		{ID: "devices.commands", Label: "设备指令"},
		{ID: "storage.objects", Label: "文件存储"},
		{ID: "algorithms.external", Label: "外部算法"},
	}},
	{ID: "flighthub", Label: "大疆司空", Description: "功能启用后仍需通过连接、设备能力验证和操作条件检查。", Children: []featureNode{
		{ID: "live", Label: "直播", Children: []featureNode{
			{ID: "live.control", Label: "启动直播"},
			{ID: "flighthub.live.quality", Label: "调整画质"},
			{ID: "flighthub.live.recording", Label: "录制控制"},
			{ID: "flighthub.live.share", Label: "直播分享"},
			{ID: "converter", Label: "码流转换器", Children: []featureNode{
				{ID: "flighthub.live.converter.create", Label: "创建转换器"},
				{ID: "flighthub.live.converter.toggle", Label: "启停转换器"},
				{ID: "flighthub.live.converter.delete", Label: "删除转换器"},
			}},
		}},
		{ID: "flight", Label: "飞行", Children: []featureNode{
			{ID: "flight.execute", Label: "飞行任务执行"},
			{ID: "device.control", Label: "设备控制", Description: "包括控制会话、返航、暂停与恢复。"},
		}},
		{ID: "resources", Label: "地图与模型", Children: []featureNode{
			{ID: "flighthub.actions", Label: "地图标注与模型创建", Description: "同时控制地图标注写入和模型创建。"},
			{ID: "flighthub.geospatial.delete", Label: "删除地图标注"},
			{ID: "flighthub.model.delete", Label: "删除模型"},
			{ID: "flighthub.model-resource.delete", Label: "删除模型资源"},
		}},
		{ID: "device", Label: "设备管理", Children: []featureNode{
			{ID: "flighthub.camera.change", Label: "切换相机"},
			{ID: "flighthub.lens.change", Label: "切换镜头"},
			{ID: "flighthub.rtk.calibrate", Label: "RTK 校准"},
			{ID: "flighthub.relay.pair", Label: "设备对频"},
			{ID: "flighthub.device-migration", Label: "设备迁移"},
			{ID: "flighthub.sn-decrypt", Label: "序列号解密"},
		}},
		{ID: "organization", Label: "组织服务", Children: []featureNode{
			{ID: "flighthub.organization.project-member", Label: "司空项目成员管理接口", Description: "仅决定是否开放司空接口，不在这里给任何成员分配权限。"},
		}},
	}},
}

func featureLeafIDs(nodes []featureNode) map[string]bool {
	out := map[string]bool{}
	var visit func([]featureNode)
	visit = func(nodes []featureNode) {
		for _, n := range nodes {
			if len(n.Children) == 0 {
				out[n.ID] = true
			} else {
				visit(n.Children)
			}
		}
	}
	visit(nodes)
	return out
}
func featureValues(raw []byte) (map[string]bool, error) {
	var stored map[string]json.RawMessage
	if err := json.Unmarshal(raw, &stored); err != nil {
		return nil, err
	}
	out := map[string]bool{}
	for key := range featureLeafIDs(projectFeatureTree) {
		var enabled bool
		// Only a literal boolean true grants enablement; unknown keys never escape.
		_ = json.Unmarshal(stored[key], &enabled)
		out[key] = enabled
	}
	return out, nil
}
func (s *Server) projectFeatureRoutes() {
	g := s.router.Group("/api/projects/:id/feature-settings", s.requireUser, s.timeout)
	g.GET("", s.readProjectFeatures)
	g.PATCH("", s.saveProjectFeatures)
}
func (s *Server) readProjectFeatures(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	pid, e := projectID(c)
	if e != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	a, e := s.projectAccess(c.Request.Context(), s.queries, currentUser(c).ID, pid, "project:view")
	if e != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	if a.Role != "owner" && a.Role != "admin" {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	raw, e := s.queries.ReadProjectFeatures(c.Request.Context(), pid)
	if e != nil {
		s.failure(c, 500, "PROJECT_FEATURES_FAILED")
		return
	}
	values, e := featureValues(raw)
	if e != nil {
		s.failure(c, 500, "PROJECT_FEATURES_FAILED")
		return
	}
	c.JSON(200, gin.H{"projectId": pid, "tree": projectFeatureTree, "values": values})
}

type featureChange struct {
	Enabled  *bool `json:"enabled"`
	Expected *bool `json:"expected"`
}

func (s *Server) saveProjectFeatures(c *gin.Context) {
	c.Header("Cache-Control", "private, no-store")
	pid, e := projectID(c)
	if e != nil {
		s.failure(c, 404, "PROJECT_NOT_FOUND")
		return
	}
	var input struct {
		Changes map[string]featureChange `json:"changes"`
	}
	allowed := featureLeafIDs(projectFeatureTree)
	if strictJSON(c, &input) != nil || len(input.Changes) == 0 || len(input.Changes) > len(allowed) {
		s.failure(c, 400, "PROJECT_FEATURES_INPUT_INVALID")
		return
	}
	for key, v := range input.Changes {
		if !allowed[key] || v.Enabled == nil || v.Expected == nil {
			s.failure(c, 400, "PROJECT_FEATURES_INPUT_INVALID")
			return
		}
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	a, e := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if e != nil || (a.Role != "owner" && a.Role != "admin") {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "project.features.update", ResourceType: "project", ResourceID: strconv.Itoa(int(pid)), Input: input, PolicyResult: map[string]any{"roles": []string{"owner", "admin"}, "scope": "project-features"}}
	result, e := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "project:view", true), func(w *database.WriteTx) (map[string]bool, error) {
		if e := w.Queries.EnsureProjectFeatures(ctx, pid); e != nil {
			return nil, e
		}
		if _, e := w.Queries.LockProjectFeatures(ctx, pid); e != nil {
			return nil, e
		}
		raw, e := w.Queries.ReadProjectFeatures(ctx, pid)
		if e != nil {
			return nil, e
		}
		values, e := featureValues(raw)
		if e != nil {
			return nil, e
		}
		changes := map[string]bool{}
		for key, v := range input.Changes {
			if values[key] != *v.Expected {
				return nil, errors.New("PROJECT_FEATURES_CONFLICT")
			}
			values[key] = *v.Enabled
			if key != "operations.overview" && key != "devices.commands" && key != "storage.objects" && key != "algorithms.external" {
				changes[key] = *v.Enabled
			}
		}
		actionChanges, _ := json.Marshal(changes)
		e = w.Queries.SaveProjectFeatures(ctx, sqlcgen.SaveProjectFeaturesParams{ProjectID: pid, OperationsOverviewEnabled: values["operations.overview"], DeviceCommandsEnabled: values["devices.commands"], ObjectStorageEnabled: values["storage.objects"], ExternalAlgorithmsEnabled: values["algorithms.external"], ActionChanges: actionChanges, UpdatedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
		return values, e
	})
	if e != nil {
		if e.Error() == "PROJECT_FEATURES_CONFLICT" {
			s.failure(c, 409, e.Error())
			return
		}
		if e.Error() == "PROJECT_ACCESS_DENIED" {
			s.failure(c, 403, e.Error())
			return
		}
		s.failure(c, 500, "PROJECT_FEATURES_FAILED")
		return
	}
	c.JSON(200, gin.H{"values": result})
}
