package httpapi

import (
	"bytes"
	"embed"
	"encoding/json"
	"errors"
	"github.com/gin-gonic/gin"
	"io"
)

//go:embed skills/*.md
var inspectionSkills embed.FS

const objectSkillName = "inspection-object-query"
const objectSkillVersion = "1.0.0"

func loadAgentSkill(raw json.RawMessage) (gin.H, error) {
	var args struct {
		SkillName string `json:"skillName"`
	}
	if len(raw) > 1024 {
		return nil, errors.New("AGENT_TOOL_INPUT_INVALID")
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if decoder.Decode(&args) != nil || decoder.Decode(new(any)) != io.EOF || args.SkillName != objectSkillName {
		return nil, errors.New("AGENT_TOOL_SKILL_NOT_FOUND")
	}
	body, err := inspectionSkills.ReadFile("skills/inspection-object-query.md")
	if err != nil {
		return nil, err
	}
	return gin.H{"quality": "trusted-platform-skill", "summary": "已加载巡检目标查询与复核 Skill@v" + objectSkillVersion, "items": []gin.H{{"id": objectSkillName, "version": objectSkillVersion, "instructions": string(body), "reference": gin.H{"type": "skill", "id": objectSkillName}}}}, nil
}
