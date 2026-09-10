package httpapi

import (
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"regexp"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
)

type algorithmDefinitionInput struct {
	ProviderID                int64
	Name, Capability          string
	Description               sql.NullString
	Definition, Configuration map[string]any
}

func parseAlgorithmDefinition(body map[string]any) (algorithmDefinitionInput, error) {
	out := algorithmDefinitionInput{}
	bad := errors.New("ALGORITHM_DEFINITION_INPUT_INVALID")
	definition, ok := body["definition"].(map[string]any)
	if !ok {
		return out, bad
	}
	configurationValue := body["configuration"]
	if configurationValue == nil {
		configurationValue = body["version"]
	}
	config, ok := configurationValue.(map[string]any)
	if !ok {
		return out, bad
	}
	for k := range definition {
		if k != "providerId" && k != "name" && k != "capabilityCode" && k != "description" {
			return out, bad
		}
	}
	// Provider IDs use the same numeric coercion and safe integer range as run
	// configuration IDs; normalize the result before computing the audit hash.
	coerced, err := parseAlgorithmRunInput(map[string]any{"configurationSnapshotId": definition["providerId"], "assetId": float64(1)})
	if err != nil {
		return out, bad
	}
	out.ProviderID = coerced.SnapshotID
	name, ok := definition["name"].(string)
	name = strings.TrimSpace(name)
	if !ok || utf16Length(name) < 1 || utf16Length(name) > 160 {
		return out, bad
	}
	out.Name = name
	capability, ok := definition["capabilityCode"].(string)
	if !ok || !regexp.MustCompile(`^[a-z][a-z0-9]*(?:[._-][a-z0-9]+)*$`).MatchString(capability) {
		return out, bad
	}
	out.Capability = capability
	out.Definition = map[string]any{"providerId": out.ProviderID, "name": name, "capabilityCode": capability}
	if value, present := definition["description"]; present {
		out.Definition["description"] = nil
		if value != nil {
			description, yes := value.(string)
			description = strings.TrimSpace(description)
			if !yes || utf16Length(description) > 2000 {
				return out, bad
			}
			out.Description = sql.NullString{String: description, Valid: true}
			out.Definition["description"] = description
		}
	}
	allowed := map[string]bool{"executionMode": true, "modelOrProcess": true, "inputSchema": true, "parametersSchema": true, "outputSchema": true, "protocolConfig": true, "outputMapping": true, "labelMapping": true, "displayMetadata": true, "publishThreshold": true}
	for k := range config {
		if !allowed[k] {
			return out, bad
		}
	}
	mode, ok := config["executionMode"].(string)
	if !ok || (mode != "synchronous" && mode != "asynchronous" && mode != "callback") {
		return out, bad
	}
	model, ok := config["modelOrProcess"].(string)
	model = strings.TrimSpace(model)
	if !ok || utf16Length(model) < 1 || utf16Length(model) > 240 {
		return out, bad
	}
	out.Configuration = map[string]any{"executionMode": mode, "modelOrProcess": model}
	for _, key := range []string{"inputSchema", "parametersSchema", "outputSchema", "protocolConfig", "outputMapping", "labelMapping", "displayMetadata"} {
		value, present := config[key]
		if !present && (key == "labelMapping" || key == "displayMetadata") {
			value = map[string]any{}
		}
		object, yes := value.(map[string]any)
		if !yes || object == nil {
			return out, bad
		}
		if key == "inputSchema" || key == "parametersSchema" || key == "outputSchema" {
			if typ, present := object["type"]; present {
				switch typ.(type) {
				case string, []any:
				default:
					return out, bad
				}
			}
		}
		out.Configuration[key] = object
	}
	threshold := float64(0)
	if value, present := config["publishThreshold"]; present {
		threshold, ok = value.(float64)
		if !ok || math.IsNaN(threshold) || threshold < 0 || threshold > 1 {
			return out, bad
		}
	}
	out.Configuration["publishThreshold"] = threshold
	return out, nil
}

func (s *Server) algorithmDefinitionRoutes() {
	group := s.router.Group("/api/projects/:id/algorithm-definitions", s.requireUser, s.timeout)
	group.GET("", func(c *gin.Context) {
		s.scopedRead(c, func(q *sqlcgen.Queries, a sqlcgen.GetProjectAccessRow) (any, error) {
			raw, err := q.ListAlgorithmCatalog(c.Request.Context(), a.ProjectID)
			if err != nil {
				return nil, err
			}
			rows, err := decodeSnapshotRows(raw)
			return gin.H{"definitions": rows}, err
		})
	})
	group.POST("", s.saveAlgorithmDefinition)
	group.PUT("/:definitionId", s.saveAlgorithmDefinition)
}

func (s *Server) algorithmDefinitionFailure(c *gin.Context, err error) {
	code, status := "ALGORITHM_DEFINITION_FAILED", 400
	switch err.Error() {
	case "PROJECT_ACCESS_DENIED":
		code, status = err.Error(), 403
	case "ALGORITHM_PROVIDER_NOT_FOUND", "ALGORITHM_DEFINITION_NOT_FOUND", "ALGORITHM_DEFINITION_INPUT_INVALID":
		code = err.Error()
	}
	s.failure(c, status, code)
}

func (s *Server) saveAlgorithmDefinition(c *gin.Context) {
	fail := func() { s.failure(c, 400, "ALGORITHM_DEFINITION_INPUT_INVALID") }
	pid, err := projectID(c)
	if err != nil {
		fail()
		return
	}
	var id int64
	creating := c.Request.Method == "POST"
	if !creating {
		id, err = strconv.ParseInt(c.Param("definitionId"), 10, 64)
		if err != nil || id <= 0 {
			fail()
			return
		}
	}
	var raw map[string]any
	if strictJSON(c, &raw) != nil {
		fail()
		return
	}
	input, err := parseAlgorithmDefinition(raw)
	if err != nil {
		fail()
		return
	}
	ctx := c.Request.Context()
	uid := currentUser(c).ID
	a, err := s.projectAccess(ctx, s.queries, uid, pid, "algorithm:manage")
	if err != nil {
		s.algorithmDefinitionFailure(c, errors.New("PROJECT_ACCESS_DENIED"))
		return
	}
	resource := ""
	if !creating {
		resource = strconv.FormatInt(id, 10)
	}
	audit := database.AuditContext{ProjectID: pid, TeamID: a.TeamID, ActorUserID: uid, RequestID: c.GetHeader("X-Request-ID"), Action: "algorithm_definition.save", ResourceType: "algorithm_definition", ResourceID: resource, Input: gin.H{"definition": input.Definition, "configuration": input.Configuration}, PolicyResult: map[string]any{"permission": "algorithm:manage", "internalConfigurationSnapshots": true}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, a.TeamID, "algorithm:manage", false), func(w *database.WriteTx) (gin.H, error) {
		team, e := w.Queries.LockAlgorithmDefinitionProvider(ctx, sqlcgen.LockAlgorithmDefinitionProviderParams{ProjectID: pid, ID: input.ProviderID})
		if errors.Is(e, sql.ErrNoRows) {
			return nil, errors.New("ALGORITHM_PROVIDER_NOT_FOUND")
		}
		if e != nil {
			return nil, e
		}
		number := int32(1)
		if creating {
			id, e = w.Queries.CreateAlgorithmDefinition(ctx, sqlcgen.CreateAlgorithmDefinitionParams{ProjectID: pid, TeamID: team, ProviderID: input.ProviderID, Name: input.Name, CapabilityCode: input.Capability, Description: input.Description, CreatedByUserID: sql.NullInt32{Int32: uid, Valid: true}})
			if e != nil {
				return nil, e
			}
		} else {
			if _, e = w.Queries.LockAlgorithmDefinition(ctx, sqlcgen.LockAlgorithmDefinitionParams{ProjectID: pid, ID: id}); errors.Is(e, sql.ErrNoRows) {
				return nil, errors.New("ALGORITHM_DEFINITION_NOT_FOUND")
			}
			if e != nil {
				return nil, e
			}
			number, e = w.Queries.NextAlgorithmConfigurationVersion(ctx, sqlcgen.NextAlgorithmConfigurationVersionParams{ProjectID: pid, AlgorithmDefinitionID: id})
			if e != nil {
				return nil, e
			}
			e = w.Queries.UpdateAlgorithmDefinition(ctx, sqlcgen.UpdateAlgorithmDefinitionParams{ProjectID: pid, ID: id, ProviderID: input.ProviderID, Name: input.Name, CapabilityCode: input.Capability, Description: input.Description})
			if e != nil {
				return nil, e
			}
			e = w.Queries.RetireAlgorithmConfigurations(ctx, sqlcgen.RetireAlgorithmConfigurationsParams{ProjectID: pid, AlgorithmDefinitionID: id})
			if e != nil {
				return nil, e
			}
		}
		conf := input.Configuration
		encoded := map[string]json.RawMessage{}
		for _, key := range []string{"inputSchema", "parametersSchema", "outputSchema", "protocolConfig", "outputMapping", "labelMapping", "displayMetadata"} {
			value, e := json.Marshal(conf[key])
			if e != nil {
				return nil, e
			}
			encoded[key] = value
		}
		snapshot, e := w.Queries.InsertAlgorithmConfiguration(ctx, sqlcgen.InsertAlgorithmConfigurationParams{ProjectID: pid, TeamID: team, AlgorithmDefinitionID: id, Version: number, ExecutionMode: conf["executionMode"].(string), ModelOrProcess: conf["modelOrProcess"].(string), InputRequirementsJson: encoded["inputSchema"], ParametersSchemaJson: encoded["parametersSchema"], OutputSchemaJson: encoded["outputSchema"], ProtocolConfigJson: encoded["protocolConfig"], OutputMappingJson: encoded["outputMapping"], LabelMappingJson: encoded["labelMapping"], DisplayMetadataJson: encoded["displayMetadata"], PublishThreshold: conf["publishThreshold"].(float64), ActorUserID: sql.NullInt32{Int32: uid, Valid: true}})
		if e != nil {
			return nil, e
		}
		e = w.Queries.SetAlgorithmCurrentConfiguration(ctx, sqlcgen.SetAlgorithmCurrentConfigurationParams{ProjectID: pid, ID: id, CurrentPublishedVersionID: sql.NullInt64{Int64: snapshot, Valid: true}})
		if e != nil {
			return nil, e
		}
		// pg returns bigint INSERT ids as strings; the old update response returns
		// its numeric route argument. Preserve this endpoint-specific distinction.
		var definitionID any = id
		if creating {
			definitionID = strconv.FormatInt(id, 10)
		}
		return gin.H{"definitionId": definitionID, "configurationSnapshotId": strconv.FormatInt(snapshot, 10)}, nil
	})
	if err != nil {
		s.algorithmDefinitionFailure(c, err)
		return
	}
	status := 200
	if creating {
		status = 201
	}
	c.JSON(status, result)
}
