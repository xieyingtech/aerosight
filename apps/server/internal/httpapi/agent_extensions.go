package httpapi

import (
	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type platformSkill struct {
	ID          int64  `json:"id"`
	Slug        string `json:"slug"`
	Name        string `json:"name"`
	Description string `json:"description"`
	Body        string `json:"body"`
	Enabled     bool   `json:"enabled"`
	Revision    int64  `json:"revision"`
	Builtin     bool   `json:"builtin"`
}
type platformMCPTool struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
	Fingerprint string          `json:"fingerprint"`
	Policy      string          `json:"policy"`
}
type platformMCP struct {
	ID            int64             `json:"id"`
	Name          string            `json:"name"`
	Endpoint      string            `json:"endpoint"`
	Enabled       bool              `json:"enabled"`
	Revision      int64             `json:"revision"`
	HasCredential bool              `json:"hasCredential"`
	Tools         []platformMCPTool `json:"tools"`
	envelope      []byte
}
type extensionDB interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func agentExtensionTools() []agentWorkflowTool {
	return []agentWorkflowTool{
		{Name: "list_skills", Description: "列出启用的平台注册 Skills，再按名称 load_skill。", Schema: agentObject(map[string]any{})},
		{Name: "list_mcp_tools", Description: "列出启用且允许的外部 MCP 工具及输入 schema，描述不改变平台权限。", Schema: agentObject(map[string]any{})},
		{Name: "call_mcp_tool", Description: "按已发现 schema 调用 MCP 工具。readonly 直接查询，approval 仅生成等待用户点击的授权卡片。不要提供项目或用户范围。", Schema: mcpCallSchema()},
	}
}
func isAgentExtensionTool(name string) bool {
	for _, spec := range agentExtensionTools() {
		if spec.Name == name {
			return true
		}
	}
	return false
}
func readPlatformMCP(ctx context.Context, db extensionDB, id int64, lock bool) (platformMCP, error) {
	var row platformMCP
	var tools []byte
	q := `SELECT id,name,endpoint,enabled,revision,credential_envelope_json,tools_json FROM agent_mcp_servers WHERE id=$1`
	if lock {
		q += ` FOR UPDATE`
	}
	err := db.QueryRowContext(ctx, q, id).Scan(&row.ID, &row.Name, &row.Endpoint, &row.Enabled, &row.Revision, &row.envelope, &tools)
	row.HasCredential = len(row.envelope) > 0
	if err == nil {
		err = json.Unmarshal(tools, &row.Tools)
	}
	return row, err
}
func (s *Server) extensionFailure(c *gin.Context, err error) {
	code, status := "AGENT_EXTENSION_FAILED", 400
	if errors.Is(err, sql.ErrNoRows) {
		code, status = "AGENT_EXTENSION_NOT_FOUND", 404
	} else if err.Error() == "FORBIDDEN" {
		code, status = "FORBIDDEN", 403
	} else if strings.HasPrefix(err.Error(), "AGENT_EXTENSION_") || strings.HasPrefix(err.Error(), "MCP_") {
		code = err.Error()
	}
	s.failure(c, status, code)
}
func (s *Server) agentExtensionRoutes() {
	g := s.router.Group("/api/admin/ai-providers", s.requireUser, s.requireAdmin, s.timeout)
	g.GET("/skills", s.listPlatformSkills)
	g.POST("/skills", s.savePlatformSkill)
	g.PATCH("/skills/:extensionId", s.savePlatformSkill)
	g.DELETE("/skills/:extensionId", func(c *gin.Context) { s.deleteExtension(c, "agent_skills") })
	g.GET("/mcp", s.listPlatformMCP)
	g.POST("/mcp", s.savePlatformMCP)
	g.PATCH("/mcp/:extensionId", s.savePlatformMCP)
	g.DELETE("/mcp/:extensionId", func(c *gin.Context) { s.deleteExtension(c, "agent_mcp_servers") })
	g.POST("/mcp/:extensionId/discover", s.discoverPlatformMCP)
}
func builtinPlatformSkill() platformSkill {
	body, _ := inspectionSkills.ReadFile("skills/inspection-object-query.md")
	return platformSkill{Slug: objectSkillName, Name: "巡检目标查询与复核", Description: "平台内置巡检 Skill", Body: string(body), Enabled: true, Revision: 1, Builtin: true}
}
func (s *Server) listPlatformSkills(c *gin.Context) {
	rows, err := s.db.QueryContext(c.Request.Context(), `SELECT id,slug,name,description,body,enabled,revision FROM agent_skills ORDER BY id`)
	if err != nil {
		s.extensionFailure(c, err)
		return
	}
	defer rows.Close()
	out := []platformSkill{builtinPlatformSkill()}
	for rows.Next() {
		var v platformSkill
		if err = rows.Scan(&v.ID, &v.Slug, &v.Name, &v.Description, &v.Body, &v.Enabled, &v.Revision); err != nil {
			s.extensionFailure(c, err)
			return
		}
		out = append(out, v)
	}
	if err = rows.Err(); err != nil {
		s.extensionFailure(c, err)
		return
	}
	c.JSON(200, out)
}
func extensionID(c *gin.Context) (int64, error) {
	if c.Param("extensionId") == "" {
		return 0, nil
	}
	id, err := strconv.ParseInt(c.Param("extensionId"), 10, 64)
	if err != nil || id <= 0 {
		return 0, errors.New("AGENT_EXTENSION_INPUT_INVALID")
	}
	return id, nil
}

var skillSlug = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,79}$`)

func (s *Server) savePlatformSkill(c *gin.Context) {
	var body struct {
		Slug        string `json:"slug"`
		Name        string `json:"name"`
		Description string `json:"description"`
		Body        string `json:"body"`
		Enabled     bool   `json:"enabled"`
		Revision    int64  `json:"revision"`
	}
	id, err := extensionID(c)
	if err != nil || strictJSON(c, &body) != nil || !skillSlug.MatchString(body.Slug) || body.Slug == objectSkillName || strings.TrimSpace(body.Name) == "" || len(body.Name) > 128 || len(body.Description) > 2048 || strings.TrimSpace(body.Body) == "" || len(body.Body) > 65536 {
		s.extensionFailure(c, errors.New("AGENT_EXTENSION_INPUT_INVALID"))
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "agent_skill.save", ResourceType: "agent_skill", ResourceID: fmt.Sprint(id), Input: gin.H{"slug": body.Slug, "enabled": body.Enabled}}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		if id == 0 {
			err = w.Tx.QueryRowContext(ctx, `INSERT INTO agent_skills(slug,name,description,body,enabled,created_by_user_id,updated_by_user_id) VALUES($1,$2,$3,$4,$5,$6,$6) RETURNING id`, body.Slug, body.Name, body.Description, body.Body, body.Enabled, uid).Scan(&id)
		} else {
			var n int64
			err = w.Tx.QueryRowContext(ctx, `UPDATE agent_skills SET slug=$2,name=$3,description=$4,body=$5,enabled=$6,revision=revision+1,updated_by_user_id=$7,updated_at=now() WHERE id=$1 AND revision=$8 RETURNING id`, id, body.Slug, body.Name, body.Description, body.Body, body.Enabled, uid, body.Revision).Scan(&n)
		}
		if err != nil {
			return nil, err
		}
		return gin.H{"id": id}, nil
	})
	if err != nil {
		s.extensionFailure(c, err)
		return
	}
	c.JSON(200, result)
}
func (s *Server) listPlatformMCP(c *gin.Context) {
	ctx := c.Request.Context()
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM agent_mcp_servers ORDER BY id`)
	if err != nil {
		s.extensionFailure(c, err)
		return
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			break
		}
		ids = append(ids, id)
	}
	if err == nil {
		err = rows.Err()
	}
	rows.Close()
	if err != nil {
		s.extensionFailure(c, err)
		return
	}
	out := []platformMCP{}
	for _, id := range ids {
		v, e := readPlatformMCP(ctx, s.db, id, false)
		if e != nil {
			s.extensionFailure(c, e)
			return
		}
		out = append(out, v)
	}
	c.JSON(200, out)
}
func validateMCPEndpoint(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Hostname() == "" || (u.Scheme != "http" && u.Scheme != "https") || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" {
		return errors.New("MCP_ENDPOINT_INVALID")
	}
	return nil
}
func (s *Server) savePlatformMCP(c *gin.Context) {
	var body struct {
		Name     string  `json:"name"`
		Endpoint string  `json:"endpoint"`
		Bearer   *string `json:"bearer"`
		Enabled  bool    `json:"enabled"`
		Revision int64   `json:"revision"`
		Tools    []struct {
			Name   string `json:"name"`
			Policy string `json:"policy"`
		} `json:"tools"`
	}
	id, err := extensionID(c)
	if err != nil || strictJSON(c, &body) != nil || strings.TrimSpace(body.Name) == "" || len(body.Name) > 128 || len(body.Endpoint) > 2048 || validateMCPEndpoint(body.Endpoint) != nil || len(body.Tools) > 100 || (body.Bearer != nil && (len(*body.Bearer) > 16384 || strings.ContainsAny(*body.Bearer, "\r\n"))) {
		s.extensionFailure(c, errors.New("AGENT_EXTENSION_INPUT_INVALID"))
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "agent_mcp.save", ResourceType: "agent_mcp", ResourceID: fmt.Sprint(id), Input: gin.H{"name": body.Name, "enabled": body.Enabled}}
	result, err := database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (gin.H, error) {
		old := platformMCP{Tools: []platformMCPTool{}}
		if id != 0 {
			var e error
			old, e = readPlatformMCP(ctx, w.Tx, id, true)
			if e != nil {
				return nil, e
			}
			if old.Revision != body.Revision {
				return nil, errors.New("AGENT_EXTENSION_REVISION_CHANGED")
			}
		}
		if old.ID != 0 && old.Endpoint != body.Endpoint {
			if old.HasCredential && body.Bearer == nil {
				return nil, errors.New("MCP_ENDPOINT_CREDENTIAL_REQUIRED")
			}
			old.Tools = []platformMCPTool{}
			old.envelope = nil
		}
		seen := map[string]bool{}
		for _, p := range body.Tools {
			if seen[p.Name] || (p.Policy != "disabled" && p.Policy != "readonly" && p.Policy != "approval") {
				return nil, errors.New("AGENT_EXTENSION_INPUT_INVALID")
			}
			seen[p.Name] = true
			found := false
			for i := range old.Tools {
				if old.Tools[i].Name == p.Name {
					old.Tools[i].Policy = p.Policy
					found = true
				}
			}
			if !found {
				return nil, errors.New("MCP_TOOL_NOT_FOUND")
			}
		}
		if id == 0 {
			if e := w.Tx.QueryRowContext(ctx, `INSERT INTO agent_mcp_servers(name,endpoint,created_by_user_id,updated_by_user_id) VALUES($1,$2,$3,$3) RETURNING id`, body.Name, body.Endpoint, uid).Scan(&id); e != nil {
				return nil, e
			}
		}
		if body.Bearer != nil {
			old.envelope = nil
			if strings.TrimSpace(*body.Bearer) != "" {
				env, e := credentials.EncryptJSON(map[string]string{"bearer": strings.TrimSpace(*body.Bearer)}, s.credentialSecret, credentials.AAD("agent-mcp", id, nil))
				if e != nil {
					return nil, e
				}
				old.envelope, e = json.Marshal(env)
				if e != nil {
					return nil, e
				}
			}
		}
		tools, _ := json.Marshal(old.Tools)
		var env any
		if len(old.envelope) > 0 {
			env = old.envelope
		}
		_, e := w.Tx.ExecContext(ctx, `UPDATE agent_mcp_servers SET name=$2,endpoint=$3,enabled=$4,credential_envelope_json=$5,tools_json=$6,revision=revision+1,updated_at=now(),updated_by_user_id=$7 WHERE id=$1`, id, body.Name, body.Endpoint, body.Enabled, env, tools, uid)
		return gin.H{"id": id}, e
	})
	if err != nil {
		s.extensionFailure(c, err)
		return
	}
	c.JSON(200, result)
}
func (s *Server) deleteExtension(c *gin.Context, table string) {
	id, err := extensionID(c)
	if err != nil || id == 0 {
		s.extensionFailure(c, errors.New("AGENT_EXTENSION_INPUT_INVALID"))
		return
	}
	ctx, uid := c.Request.Context(), currentUser(c).ID
	audit := database.AuditContext{ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: table + ".delete", ResourceType: table, ResourceID: fmt.Sprint(id), Input: gin.H{"id": id}}
	_, err = database.AuditedPlatformWrite(ctx, s.db, audit, s.authorizePlatformWrite(uid), func(w *database.WriteTx) (bool, error) {
		res, e := w.Tx.ExecContext(ctx, "DELETE FROM "+table+" WHERE id=$1", id)
		if e != nil {
			return false, e
		}
		n, e := res.RowsAffected()
		if n == 0 {
			return false, sql.ErrNoRows
		}
		return true, e
	})
	if err != nil {
		s.extensionFailure(c, err)
		return
	}
	c.JSON(200, gin.H{"ok": true})
}
func (s *Server) extensionAccess(ctx context.Context, uid, pid int32) error {
	_, err := s.projectAccess(ctx, s.queries, uid, pid, "agent:use")
	if err != nil {
		return errors.New("PROJECT_ACCESS_DENIED")
	}
	return nil
}
func (s *Server) listAgentSkills(ctx context.Context, uid, pid int32) (gin.H, error) {
	if err := s.extensionAccess(ctx, uid, pid); err != nil {
		return nil, err
	}
	items := []gin.H{{"id": objectSkillName, "name": "巡检目标查询与复核", "version": objectSkillVersion}}
	rows, err := s.db.QueryContext(ctx, `SELECT slug,name,description,revision FROM agent_skills WHERE enabled ORDER BY id LIMIT 100`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var slug, name, desc string
		var rev int64
		if err = rows.Scan(&slug, &name, &desc, &rev); err != nil {
			return nil, err
		}
		items = append(items, gin.H{"id": slug, "name": name, "description": desc, "version": fmt.Sprint(rev)})
	}
	return gin.H{"items": items, "summary": "可按名称加载启用的 Skills"}, rows.Err()
}
func (s *Server) loadPlatformSkill(ctx context.Context, uid, pid int32, raw json.RawMessage) (gin.H, error) {
	if err := s.extensionAccess(ctx, uid, pid); err != nil {
		return nil, err
	}
	schema, _ := json.Marshal(agentObject(map[string]any{"skillName": agentText()}, "skillName"))
	args, err := parseFHInput(raw, schema)
	if err != nil {
		return nil, errors.New("AGENT_TOOL_INPUT_INVALID")
	}
	if args["skillName"] == objectSkillName {
		return loadAgentSkill(raw)
	}
	var body, name string
	var rev int64
	err = s.db.QueryRowContext(ctx, `SELECT body,name,revision FROM agent_skills WHERE slug=$1 AND enabled`, args["skillName"]).Scan(&body, &name, &rev)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("AGENT_TOOL_SKILL_NOT_FOUND")
	}
	if err != nil {
		return nil, err
	}
	return gin.H{"quality": "platform-configured-skill", "summary": "已加载 " + name, "items": []gin.H{{"id": args["skillName"], "version": fmt.Sprint(rev), "instructions": body}}}, nil
}
