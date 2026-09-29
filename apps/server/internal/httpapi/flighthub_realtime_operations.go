package httpapi

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"slices"
	"strconv"
	"time"

	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type fhRealtimeOperation struct {
	Key              string  `json:"key"`
	Label            string  `json:"label"`
	Group            string  `json:"group"`
	Kind             string  `json:"kind"`
	Capability       string  `json:"capability"`
	Flag             string  `json:"-"`
	Permission       string  `json:"-"`
	DeviceCapability string  `json:"deviceCapability"`
	Fields           []gin.H `json:"fields"`
}

func fhRealtimeCatalog() []fhRealtimeOperation {
	field := func(key, label, kind string) gin.H {
		return gin.H{"key": key, "label": label, "type": kind, "required": true}
	}
	camera := field("cameraIndex", "相机索引", "text")
	return []fhRealtimeOperation{
		{"return_home", "返航", "飞行控制", "command", "device.control", "device.control", "mission:operate", "flight.return_home", []gin.H{}},
		{"return_home_cancel", "取消返航", "飞行控制", "command", "device.control", "device.control", "mission:operate", "flight.return_home", []gin.H{}},
		{"flighttask_pause", "暂停飞行任务", "飞行控制", "command", "device.control", "device.control", "mission:operate", "mission.execute", []gin.H{}},
		{"flighttask_recovery", "恢复飞行任务", "飞行控制", "command", "device.control", "device.control", "mission:operate", "mission.execute", []gin.H{}},
		{"camera.change", "切换机场相机", "相机与画质", "command", "device.camera.change", "flighthub.camera.change", "mission:operate", "camera.change", []gin.H{camera, {"key": "cameraPosition", "label": "相机位置", "type": "select", "required": true, "options": []gin.H{{"value": "indoor", "label": "舱内"}, {"value": "outdoor", "label": "舱外"}}}}},
		{"camera.change_lens", "切换飞行器镜头", "相机与画质", "command", "device.lens.change", "flighthub.lens.change", "mission:operate", "camera.lens.change", []gin.H{camera, {"key": "lensType", "label": "镜头", "type": "text", "required": true, "hint": "填写设备相机目录实际返回的 lens_type"}}},
		{"control.acquire", "获取负载控制权", "控制权", "control", "device.control", "device.control", "mission:operate", "camera.payload.control", []gin.H{camera}},
		{"live-quality-set", "调整图传画质", "相机与画质", "live", "live.quality.set", "flighthub.live.quality", "mission:operate", "stream.video.quality", []gin.H{camera, {"key": "qualityType", "label": "画质", "type": "select", "required": true, "options": []gin.H{{"value": "adaptive", "label": "自动"}, {"value": "smooth", "label": "流畅"}, {"value": "ultra_high_definition", "label": "超清"}}}}},
		{"rtk-calibrate", "网络 RTK 标定", "设备维护", "admin", "device.rtk.calibrate", "flighthub.rtk.calibrate", "device:configure", "rtk.calibrate", []gin.H{field("host", "NTRIP 服务地址", "text"), field("port", "端口", "number"), field("account", "账号", "text"), field("password", "密码", "password"), field("mountPoint", "挂载点", "text")}},
		{"active-project-update", "修改司空项目绑定", "设备维护", "admin", "device.active-project.update", "flighthub.device-migration", "device:configure", "device.active-project.update", []gin.H{field("activeProjectUuid", "目标司空项目 UUID", "text")}},
	}
}

type fhRealtimeFlightState struct {
	Mode       string
	TaskStatus string
	Fresh      bool
}

func (state fhRealtimeFlightState) allows(key string) bool {
	switch key {
	case "return_home", "return_home_cancel", "flighttask_pause", "flighttask_recovery":
		if !state.Fresh {
			return false
		}
	default:
		return true
	}
	switch key {
	case "return_home_cancel":
		return state.Mode == "auto_returning_to_home"
	case "return_home":
		return slices.Contains([]string{"manual_flight", "wayline_flight", "live_flight_controls", "panoramic_photography", "intelligent_tracking", "adsb_avoidance", "apas", "airborne_rtk_fixing_mode"}, state.Mode)
	case "flighttask_pause":
		return state.TaskStatus == "executing" && state.Mode == "wayline_flight"
	case "flighttask_recovery":
		return state.TaskStatus == "paused" && state.Mode != "auto_returning_to_home" && slices.Contains([]string{"manual_flight", "wayline_flight", "live_flight_controls"}, state.Mode)
	}
	return false
}

func (s *Server) fhRealtimeFlightState(ctx context.Context, pid, did int32, cid int64) (fhRealtimeFlightState, error) {
	var state fhRealtimeFlightState
	// Dock commands affect its paired aircraft. Scope the topology, telemetry,
	// and task to this connector; unrelated aircraft must not enable controls.
	err := s.db.QueryRowContext(ctx, `select coalesce(flight.payload_json->>'mode',''),
	 coalesce(task.input_snapshot_json->>'remoteStatus',''),
	 coalesce(flight.captured_at>now()-interval '30 seconds' and flight.captured_at<=now()+interval '1 second',false)
	 from devices device
	 left join device_external_identities gateway on gateway.device_id=device.id and gateway.project_id=device.project_id and gateway.adapter_id=$3
	 left join lateral (select child.device_id from device_external_identities child
	   where child.project_id=$1 and child.adapter_id=$3 and child.identity_json->>'parentExternalId'=gateway.external_device_id
	   and child.discovery_status='managed' and child.device_id is not null order by child.id limit 1) aircraft on true
	 left join device_latest_telemetry flight on flight.project_id=$1 and flight.adapter_id=$3
	   and flight.device_id=coalesce(aircraft.device_id,device.id) and flight.telemetry_type='dji.flighthub.state'
	 left join lateral (select run.input_snapshot_json from task_runs run
	   where run.project_id=$1 and run.selected_device_id in(device.id,aircraft.device_id)
	   and run.trigger_source='dji-flighthub' and run.status in('running','paused')
	   and run.input_snapshot_json->>'observedAt' is not null
	   and (run.input_snapshot_json->>'observedAt')::timestamptz>now()-interval '30 seconds'
	   and exists(select 1 from connector_remote_resources resource where resource.id=(run.input_snapshot_json->>'remoteResourceId')::bigint
	     and resource.project_id=$1 and resource.connector_instance_id=$3 and resource.resource_kind='flight-task' and resource.status='active')
	   order by (run.input_snapshot_json->>'observedAt')::timestamptz desc,run.id desc limit 1) task on true
	 where device.project_id=$1 and device.id=$2 limit 1`, pid, did, cid).Scan(&state.Mode, &state.TaskStatus, &state.Fresh)
	return state, err
}

// Operation metadata depends on a device capability, not another model list.
// Transport applicability remains owned by the adapter's execution policies.
func fhRealtimeFind(key, model string, capabilities map[string]string) (fhRealtimeOperation, bool) {
	for _, operation := range fhRealtimeCatalog() {
		if operation.Key != key {
			continue
		}
		if _, declared := capabilities[operation.DeviceCapability]; !declared {
			return fhRealtimeOperation{}, false
		}
		if operation.Kind == "command" {
			policy, ok := fhDiscretePolicies[operation.Key]
			if !ok || !slices.Contains(policy.types, model) {
				return fhRealtimeOperation{}, false
			}
		}
		return operation, true
	}
	return fhRealtimeOperation{}, false
}

func (s *Server) fhRealtimeCapabilities(ctx context.Context, pid, did int32) (map[string]string, error) {
	// The type profile declares capabilities; instance observations determine
	// availability. A stale instance row cannot add a capability to its type.
	rows, err := s.db.QueryContext(ctx, `select entry.key,coalesce(cap.availability,'unavailable')
      from devices device join device_types type on type.id=device.device_type_id
      join driver_definitions driver on driver.id=type.driver_definition_id
      cross join lateral jsonb_each(type.capability_profile_json) entry
      left join device_capabilities cap on cap.device_id=device.id and cap.project_id=device.project_id and cap.capability_code=entry.key
      where device.id=$2 and device.project_id=$1 and entry.value->>'enabled'='true'
      and exists(select 1 from jsonb_array_elements(driver.manifest_json->'capabilities') definition where definition->>'code'=entry.key)`, pid, did)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]string{}
	for rows.Next() {
		var code, state string
		if err = rows.Scan(&code, &state); err != nil {
			return nil, err
		}
		result[code] = state
	}
	return result, rows.Err()
}

func (s *Server) fhRealtimeRoute(ctx context.Context, pid, did int32) (gin.H, string, error) {
	var model string
	if err := s.db.QueryRowContext(ctx, `select type.type_key from devices device join device_types type on type.id=device.device_type_id where device.project_id=$1 and device.id=$2`, pid, did).Scan(&model); err != nil {
		return nil, "", err
	}
	raw, err := s.queries.FHCommandRoutes(ctx, sqlcgen.FHCommandRoutesParams{P1: pid, P2: did})
	rows, err := decodeFHRows(raw, err)
	if err != nil {
		return nil, "", err
	}
	if len(rows) == 0 || rows[0]["connectorKey"] != "dji.flighthub2" || len(rows) > 1 && rows[0]["priority"] == rows[1]["priority"] {
		return nil, "", errors.New("DEVICE_COMMAND_ROUTE_UNAVAILABLE")
	}
	return rows[0], model, nil
}

func fhRealtimeBody(operation fhRealtimeOperation, params gin.H, cid int64, did int32, approval string) (gin.H, error) {
	body := gin.H{"connectorInstanceId": float64(cid), "deviceId": float64(did), "approvalRequestId": approval, "idempotencyKey": "realtime-operation:" + approval}
	switch operation.Kind {
	case "command":
		p := fhDiscretePolicies[operation.Key]
		if !validFHCommandParameters(operation.Key, map[string]any(params)) {
			return nil, errors.New("FLIGHTHUB_COMMAND_PARAMETERS_INVALID")
		}
		body = gin.H{"capabilityCode": p.capability, "commandKey": operation.Key, "parameters": params, "approvalRequestId": approval, "idempotencyKey": "realtime-operation:" + approval, "reason": operation.Label, "confirmation": fmt.Sprintf("CONFIRM %d %s", did, p.capability)}
	case "control":
		if len(params) != 1 || !fhPayloadIndex.MatchString(fhString(params["cameraIndex"])) {
			return nil, errFHInput
		}
		body["controls"] = gin.H{"flight": false, "payloadIndex": []any{params["cameraIndex"]}}
	case "admin", "live":
		body["action"], body["request"] = operation.Key, params
		schema := fhDeviceAdminSchema
		if operation.Kind == "live" {
			schema = fhLiveSchema
			delete(body, "approvalRequestId")
		}
		if operation.Key == "rtk-calibrate" {
			body["confirmation"] = "CALIBRATE RTK"
		}
		if operation.Key == "active-project-update" {
			body["confirmation"] = "MOVE DEVICE"
		}
		raw, _ := json.Marshal(body)
		parsed, err := parseFHInput(raw, schema)
		if err != nil {
			return nil, err
		}
		body = gin.H(parsed)
	default:
		return nil, errFHInput
	}
	return body, nil
}

func (s *Server) fhRealtimeOperations(c *gin.Context) {
	pid, err := projectID(c)
	did64, e := strconv.ParseInt(c.Param("deviceId"), 10, 32)
	if err != nil || e != nil || did64 <= 0 {
		s.failure(c, 400, "DEVICE_OPERATION_INPUT_INVALID")
		return
	}
	did, uid, ctx := int32(did64), currentUser(c).ID, c.Request.Context()
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil {
		s.failure(c, 404, "DEVICE_OPERATION_NOT_FOUND")
		return
	}
	route, model, err := s.fhRealtimeRoute(ctx, pid, did)
	if err != nil {
		s.failure(c, 404, "DEVICE_OPERATION_NOT_FOUND")
		return
	}
	cid, _ := strconv.ParseInt(fhString(route["connectorInstanceId"]), 10, 64)
	capabilities, err := s.fhRealtimeCapabilities(ctx, pid, did)
	if err != nil {
		s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
		return
	}
	flightState, err := s.fhRealtimeFlightState(ctx, pid, did, cid)
	if err != nil {
		s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
		return
	}
	if c.Request.Method == "GET" {
		items := []gin.H{}
		var online bool
		if err = s.db.QueryRowContext(ctx, "select status='online' from devices where project_id=$1 and id=$2", pid, did).Scan(&online); err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		cameraOptions := []gin.H{}
		cameraRows, err := s.db.QueryContext(ctx, `select channel_key,display_name from device_stream_channels where project_id=$1 and device_id=$2 and protocol='dji-flighthub-openapi' and availability='available' and source_json->>'connectorInstanceId'=$3 order by display_name`, pid, did, strconv.FormatInt(cid, 10))
		if err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		for cameraRows.Next() {
			var key, label string
			if err = cameraRows.Scan(&key, &label); err != nil {
				break
			}
			cameraOptions = append(cameraOptions, gin.H{"value": key, "label": label})
		}
		cameraErr := cameraRows.Err()
		cameraRows.Close()
		if err != nil || cameraErr != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		for _, operation := range fhRealtimeCatalog() {
			if !flightState.allows(operation.Key) {
				continue
			}
			if _, ok := fhRealtimeFind(operation.Key, model, capabilities); !ok {
				continue
			}
			raw, err := s.queries.FHCommandGovernance(ctx, sqlcgen.FHCommandGovernanceParams{P1: pid, P2: did, P3: cid, P5: operation.Flag, P6: operation.Capability})
			governance, err := fhFirstRow(raw, err, "DEVICE_OPERATION_NOT_FOUND")
			if err != nil {
				s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
				return
			}
			blockers := []string{}
			if capabilities[operation.DeviceCapability] != "available" {
				blockers = append(blockers, "设备当前未开放此能力")
			}
			_, permissionErr := s.projectAccess(ctx, s.queries, uid, pid, operation.Permission)
			permitted := permissionErr == nil && (operation.Kind != "admin" || access.Role == "owner" || access.Role == "admin")
			if !permitted {
				blockers = append(blockers, "当前账号没有操作权限")
			}
			if route["connectorStatus"] != "connected" {
				blockers = append(blockers, "司空连接器未连接")
			}
			if governance["featureEnabled"] != true {
				blockers = append(blockers, "项目尚未启用此操作")
			}
			if !online {
				blockers = append(blockers, "设备不在线")
			}
			captured, err := time.Parse(time.RFC3339Nano, fhString(governance["stateCapturedAt"]))
			if err != nil || time.Since(captured) > 30*time.Second || captured.After(time.Now().Add(time.Second)) {
				blockers = append(blockers, "设备状态已过期，等待司空同步最新状态")
			}
			for _, field := range operation.Fields {
				if field["key"] == "cameraIndex" && len(cameraOptions) > 0 {
					field["type"] = "select"
					field["options"] = cameraOptions
				}
			}
			items = append(items, gin.H{"key": operation.Key, "label": operation.Label, "group": operation.Group, "deviceCapability": operation.DeviceCapability, "fields": operation.Fields, "canRequest": permitted, "available": len(blockers) == 0, "blockers": blockers})
		}
		approvals := []gin.H{}
		rows, err := s.db.QueryContext(ctx, `select request.id::text,request.status,request.context_json->>'operation',request.context_json->>'label',request.requested_by_user_id,usr.name,request.expires_at,request.require_separation,request.required_approvals,coalesce(request.context_json->'summary','{}'::jsonb),coalesce(
 (select jsonb_build_object('id',job.id,'status',job.status,'error',job.result_json->>'errorCode') from device_commands job where job.project_id=request.project_id and job.device_id=$3 and job.idempotency_key='realtime-operation:'||request.id::text),
 (select jsonb_build_object('id',job.id,'status',job.status,'error',job.last_error_code) from connector_device_admin_jobs job where job.project_id=request.project_id and job.device_id=$3 and job.idempotency_key='realtime-operation:'||request.id::text limit 1),
 (select jsonb_build_object('id',job.id,'status',job.status,'error',job.last_error_code) from connector_live_action_jobs job where job.project_id=request.project_id and job.device_id=$3 and job.idempotency_key='realtime-operation:'||request.id::text limit 1),
 (select jsonb_build_object('id',job.id,'status',job.status,'error',job.failure_code) from connector_control_sessions job where job.project_id=request.project_id and job.device_id=$3 and job.idempotency_key='realtime-operation:'||request.id::text), 'null'::jsonb)
 from approval_requests request join users usr on usr.id=request.requested_by_user_id where request.project_id=$1 and request.resource_type='device' and request.resource_id=$2 and request.context_json->>'source'='manual-device-operation' order by request.created_at desc limit 30`, pid, strconv.Itoa(int(did)), did)
		if err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		for rows.Next() {
			var id, status, key, label, name string
			var requester, required int32
			var expires time.Time
			var separation bool
			var summary json.RawMessage
			var receipt json.RawMessage
			if err = rows.Scan(&id, &status, &key, &label, &requester, &name, &expires, &separation, &required, &summary, &receipt); err != nil {
				break
			}
			approvals = append(approvals, gin.H{"id": id, "key": key, "label": label, "requestedBy": name, "summary": summary, "receipt": receipt})
		}
		rowErr := rows.Err()
		rows.Close()
		if err != nil || rowErr != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		sessions := []gin.H{}
		sessionRows, err := s.db.QueryContext(ctx, `select id::text,status,holder_user_id,lease_expires_at from connector_control_sessions where project_id=$1 and device_id=$2 and status in('requested','acquiring','active','releasing') and absolute_expires_at>now() and lease_expires_at>now() order by created_at desc`, pid, did)
		if err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		for sessionRows.Next() {
			var id, status string
			var holder int32
			var lease time.Time
			if err = sessionRows.Scan(&id, &status, &holder, &lease); err != nil {
				break
			}
			sessions = append(sessions, gin.H{"id": id, "status": status, "owned": holder == uid, "leaseExpiresAt": timestamp(lease)})
		}
		sessionErr := sessionRows.Err()
		sessionRows.Close()
		if err != nil || sessionErr != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		c.JSON(200, gin.H{"operations": items, "executions": approvals, "controlSessions": sessions})
		return
	}
	var input struct {
		Action         string `json:"action"`
		Operation      string `json:"operation"`
		Parameters     gin.H  `json:"parameters"`
		IdempotencyKey string `json:"idempotencyKey"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	if decoder.Decode(&input) != nil {
		s.failure(c, 400, "DEVICE_OPERATION_INPUT_INVALID")
		return
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		s.failure(c, 400, "DEVICE_OPERATION_INPUT_INVALID")
		return
	}
	if input.Action == "execute" && len(input.IdempotencyKey) >= 8 && len(input.IdempotencyKey) <= 200 {
		operation, ok := fhRealtimeFind(input.Operation, model, capabilities)
		if !ok || input.Parameters == nil {
			s.failure(c, 400, "DEVICE_OPERATION_INPUT_INVALID")
			return
		}
		if capabilities[operation.DeviceCapability] != "available" {
			s.failure(c, 409, "DEVICE_CAPABILITY_UNAVAILABLE")
			return
		}
		if !flightState.allows(operation.Key) {
			s.failure(c, 409, "DEVICE_OPERATION_STATE_CHANGED")
			return
		}
		if _, err = s.projectAccess(ctx, s.queries, uid, pid, operation.Permission); err != nil {
			s.failure(c, 403, "PROJECT_ACCESS_DENIED")
			return
		}
		if operation.Kind == "admin" && access.Role != "owner" && access.Role != "admin" {
			s.failure(c, 403, "PROJECT_ACCESS_DENIED")
			return
		}
		id := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("manual-device-operation:%d:%d:%d:%s", pid, did, uid, input.IdempotencyKey)))
		body, err := fhRealtimeBody(operation, input.Parameters, cid, did, id.String())
		if err != nil {
			s.failure(c, 400, "DEVICE_OPERATION_PARAMETERS_INVALID")
			return
		}
		envelope, err := credentials.EncryptJSON(body, s.credentialSecret, credentials.AAD("realtime-device-operation", id.String(), pid))
		if err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_EXECUTION_FAILED")
			return
		}
		summary := gin.H{}
		for key, value := range input.Parameters {
			if key == "password" {
				summary[key] = "已填写（隐藏）"
			} else {
				summary[key] = value
			}
		}
		digestRaw, _ := json.Marshal(gin.H{"operation": operation.Key, "parameters": input.Parameters, "connectorId": cid})
		digest := fmt.Sprintf("%x", sha256.Sum256(digestRaw))
		contextRaw, _ := json.Marshal(gin.H{"source": "manual-device-operation", "authorizationMode": "rbac", "digest": digest, "operation": operation.Key, "label": operation.Label, "connectorId": cid, "body": envelope, "summary": summary})
		approvalAction := "flighthub.device." + operation.Key
		if operation.Kind == "control" {
			approvalAction = "flighthub.control.acquire"
		}
		if operation.Kind == "admin" {
			approvalAction = "flighthub.admin." + operation.Key
		}
		if operation.Kind == "live" {
			approvalAction = "flighthub.live." + operation.Key
		}
		audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, Action: "device.operation.execute", ResourceType: "device", ResourceID: strconv.Itoa(int(did)), Input: gin.H{"operation": operation.Key}, PolicyResult: map[string]any{"authorizationMode": "rbac", "humanInitiated": true}}
		_, err = database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, operation.Permission, operation.Kind == "admin"), func(w *database.WriteTx) (gin.H, error) {
			_, err := w.Tx.ExecContext(ctx, `insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,require_separation,expires_at,context_json) values($1,$2,$3,'device',$4,$5,$6,'approved',false,now()+interval '15 minutes',$7) on conflict(id) do nothing`, id, pid, access.TeamID, strconv.Itoa(int(did)), approvalAction, uid, contextRaw)
			if err != nil {
				return nil, err
			}
			// Durable RBAC authorization for the existing job FK, without a pending request or second-person approval.
			var savedDigest string
			var saved json.RawMessage
			err = w.Tx.QueryRowContext(ctx, `select context_json->>'digest',context_json from approval_requests where id=$1 and project_id=$2 and requested_by_user_id=$3 and context_json->>'source'='manual-device-operation'`, id, pid, uid).Scan(&savedDigest, &saved)
			if err != nil {
				return nil, err
			}
			if savedDigest != digest {
				return nil, fmt.Errorf("DEVICE_OPERATION_IDEMPOTENCY_CONFLICT")
			}
			var stored struct {
				Body credentials.Envelope `json:"body"`
			}
			if err = json.Unmarshal(saved, &stored); err != nil {
				return nil, err
			}
			if err = credentials.DecryptJSON(stored.Body, s.credentialSecret, credentials.AAD("realtime-device-operation", id.String(), pid), &body); err != nil {
				return nil, err
			}
			return gin.H{}, nil
		})
		if err != nil {
			s.failure(c, 403, "DEVICE_OPERATION_EXECUTION_FAILED")
			return
		}
		frozen, _ := json.Marshal(body)
		c.Request.Body = io.NopCloser(bytes.NewReader(frozen))
		c.Params = append(c.Params, gin.Param{Key: "connectorId", Value: strconv.FormatInt(cid, 10)})
		switch operation.Kind {
		case "command":
			s.submitDeviceCommand(c)
		case "admin":
			s.fhGovernedAction("device-admin")(c)
		case "live":
			s.fhResourceAction("live")(c)
		case "control":
			s.fhControlSession(c)
		}
		return
	}
	s.failure(c, 400, "DEVICE_OPERATION_INPUT_INVALID")
}
