package httpapi

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strconv"

	"aerosight/server/internal/credentials"
	"aerosight/server/internal/database"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/flighthub"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Manual flights use the same durable dispatcher as workflow flights. The
// authenticated click supplies RBAC authorization; agent approvals stay separate.
func (s *Server) fhFlightLaunch(c *gin.Context) {
	pid, err := projectID(c)
	number, e := strconv.ParseInt(c.Param("deviceId"), 10, 32)
	if err != nil || e != nil || number <= 0 {
		s.failure(c, 400, "DEVICE_OPERATION_INPUT_INVALID")
		return
	}
	did, uid, ctx := int32(number), currentUser(c).ID, c.Request.Context()
	access, err := s.projectAccess(ctx, s.queries, uid, pid, "project:view")
	if err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	route, _, err := s.fhRealtimeRoute(ctx, pid, did)
	if err != nil {
		s.failure(c, 404, "DEVICE_OPERATION_NOT_FOUND")
		return
	}
	cid, _ := strconv.ParseInt(fhString(route["connectorInstanceId"]), 10, 64)
	// FlightHub dispatches a dock mission to the gateway SN. Resolve an
	// aircraft selection through its scoped parent rather than sending its SN.
	var docks []int32
	rows, err := s.db.QueryContext(ctx, `select distinct d.id from devices d join device_types type on type.id=d.device_type_id join device_external_identities identity on identity.device_id=d.id and identity.project_id=d.project_id and identity.adapter_id=$3 where d.project_id=$1 and type.category='dock' and (d.id=$2 or exists(select 1 from device_relationships relation where relation.project_id=$1 and relation.from_device_id=d.id and relation.to_device_id=$2 and relation.relation_type in('contains','mounted-on') and relation.valid_until is null and relation.valid_from<=now()))`, pid, did, cid)
	if err != nil {
		s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
		return
	}
	for rows.Next() {
		var dock int32
		if err = rows.Scan(&dock); err != nil {
			break
		}
		docks = append(docks, dock)
	}
	readErr := rows.Err()
	rows.Close()
	if err != nil || readErr != nil {
		s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
		return
	}
	if len(docks) != 1 {
		s.failure(c, 409, "FLIGHTHUB_FLIGHT_DOCK_UNAVAILABLE")
		return
	}
	did = docks[0]
	capabilities, err := s.fhRealtimeCapabilities(ctx, pid, did)
	if err != nil {
		s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
		return
	}
	if _, ok := capabilities["mission.execute"]; !ok {
		s.failure(c, 403, "DEVICE_CAPABILITY_NOT_GRANTED")
		return
	}
	if c.Request.Method == "GET" {
		if key := c.Query("idempotencyKey"); key != "" {
			var raw []byte
			err = s.db.QueryRowContext(ctx, `select jsonb_build_object('id',job.id,'status',job.status,'error',job.last_error_code,'runId',job.task_run_id,'accepted',job.accepted_at is not null,'remoteStatus',resource.summary_json->>'status') from connector_action_jobs job left join connector_remote_resources resource on resource.id=job.remote_result_resource_id and resource.project_id=job.project_id where job.project_id=$1 and job.connector_instance_id=$2 and job.device_id=$3 and job.requested_by_user_id=$4 and job.action_kind='flight-task-create' and job.idempotency_key=$5`, pid, cid, did, uid, "manual-flight:"+key).Scan(&raw)
			if err != nil {
				s.failure(c, 404, "DEVICE_OPERATION_NOT_FOUND")
				return
			}
			var receipt gin.H
			if json.Unmarshal(raw, &receipt) != nil {
				s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
				return
			}
			if receipt["accepted"] == true && s.flightHub != nil {
				if reader, ok := s.flightHub.client.(interface {
					GetFlightTask(context.Context, string, string, string) (flighthub.FlightTask, error)
				}); ok {
					job, loadErr := flighthub.NewSQLFlightActionStore(s.db).Load(ctx, int(pid), fhString(receipt["id"]))
					if loadErr == nil && job.RemoteResultID != "" {
						var scope struct {
							ProjectUUID string `json:"projectUuid"`
						}
						token, tokenErr := (flighthub.EncryptedTokenResolver{AuthSecret: s.credentialSecret}).ResolveToken(ctx, job.Instance)
						if tokenErr == nil && json.Unmarshal(job.Instance.DiscoveryScope, &scope) == nil {
							flight, readErr := reader.GetFlightTask(ctx, token, scope.ProjectUUID, job.RemoteResultID)
							if readErr == nil {
								receipt["remoteStatus"] = flight.Status
								local := "dispatching"
								switch flight.Status {
								case "executing":
									local = "running"
								case "paused", "suspended":
									local = "paused"
								case "success", "partially_done":
									local = "succeeded"
								case "starting_failure", "terminated", "timeout":
									local = "failed"
								}
								_, updateErr := s.db.ExecContext(ctx, `update task_runs set status=$3,state_version=state_version+case when status<>$3 then 1 else 0 end,started_at=case when $3='running' then coalesce(started_at,now()) else started_at end,finished_at=case when $3 in('succeeded','failed') then coalesce(finished_at,now()) else finished_at end,input_snapshot_json=input_snapshot_json||jsonb_build_object('remoteStatus',$4::text,'observedAt',now()) where project_id=$1 and id=$2 and input_snapshot_json->>'source'='manual-device-flight' and status not in('succeeded','failed','canceled')`, pid, job.TaskRunID, local, flight.Status)
								if updateErr != nil {
									s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
									return
								}
							} else {
								receipt["refreshError"] = "无法读取最新司空状态"
							}
						}
					}
				}
			}
			c.JSON(200, receipt)
			return
		}
		_, permissionErr := s.projectAccess(ctx, s.queries, uid, pid, "mission:operate")
		var enabled bool
		err = s.db.QueryRowContext(ctx, `select coalesce((select flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb from project_feature_flags where project_id=$1),false)`, pid).Scan(&enabled)
		if err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		rows, err := s.db.QueryContext(ctx, `select id,name from (select id,coalesce(summary_json->>'name',remote_id) as name from connector_remote_resources where project_id=$1 and team_id=$2 and connector_instance_id=$3 and resource_kind='wayline' and status='active') routes order by name,id limit 200`, pid, access.TeamID, cid)
		if err != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		defer rows.Close()
		lines := []gin.H{}
		for rows.Next() {
			var id int64
			var name string
			if err = rows.Scan(&id, &name); err != nil {
				s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
				return
			}
			lines = append(lines, gin.H{"id": id, "name": name})
		}
		if rows.Err() != nil {
			s.failure(c, 500, "DEVICE_OPERATION_READ_FAILED")
			return
		}
		c.JSON(200, gin.H{"waylines": lines, "executionDeviceId": did, "canExecute": permissionErr == nil && enabled, "defaults": gin.H{"waylinePrecisionType": "gps", "rthAltitude": 50}})
		return
	}
	var input struct {
		WaylineResourceID int64  `json:"waylineResourceId"`
		Name              string `json:"name"`
		Precision         string `json:"waylinePrecisionType"`
		RTHAltitude       int    `json:"rthAltitude"`
		Key               string `json:"idempotencyKey"`
	}
	decoder := json.NewDecoder(c.Request.Body)
	decoder.DisallowUnknownFields()
	var extra any
	if decoder.Decode(&input) != nil || decoder.Decode(&extra) != io.EOF || input.WaylineResourceID <= 0 || len(input.Key) < 8 || len(input.Key) > 180 || input.RTHAltitude < 20 || input.RTHAltitude > 500 || (input.Precision != "gps" && input.Precision != "rtk") {
		s.failure(c, 400, "DEVICE_OPERATION_PARAMETERS_INVALID")
		return
	}
	request := gin.H{"name": input.Name, "timeZone": "Asia/Shanghai", "taskType": "immediate", "rthAltitude": input.RTHAltitude, "rthMode": "preset", "waylinePrecisionType": input.Precision, "outOfControlActionInFlight": "return_home", "resumableStatus": "manual", "repeatType": "nonrepeating"}
	key := "manual-flight:" + input.Key
	approval := uuid.NewSHA1(uuid.NameSpaceOID, []byte(fmt.Sprintf("%d:%d:%d:%s", pid, did, uid, key)))
	validation, _ := json.Marshal(gin.H{"connectorInstanceId": cid, "taskRunId": 1, "action": "flight-task-create", "waylineResourceId": input.WaylineResourceID, "approvalRequestId": approval.String(), "idempotencyKey": key, "request": request})
	if _, err = parseFHInput(validation, fhFlightSchema); err != nil {
		s.failure(c, 400, "DEVICE_OPERATION_PARAMETERS_INVALID")
		return
	}
	digest, err := database.AuditHash(gin.H{"deviceId": did, "waylineResourceId": input.WaylineResourceID, "request": request})
	if err != nil {
		s.failure(c, 500, "DEVICE_OPERATION_EXECUTION_FAILED")
		return
	}
	request["manualDeviceFlight"] = true
	job := uuid.New()
	envelope, err := credentials.EncryptJSON(request, s.credentialSecret, credentials.AAD("flighthub-flight-action", job.String(), pid))
	if err != nil {
		s.failure(c, 500, "DEVICE_OPERATION_EXECUTION_FAILED")
		return
	}
	encrypted, _ := json.Marshal(envelope)
	audit := database.AuditContext{ProjectID: pid, TeamID: access.TeamID, ActorUserID: uid, IdempotencyKey: key, Action: "device.flight.execute", ResourceType: "device", ResourceID: strconv.Itoa(int(did)), Input: gin.H{"waylineResourceId": input.WaylineResourceID, "requestDigest": digest}, PolicyResult: map[string]any{"authorizationMode": "rbac", "humanInitiated": true}}
	result, err := database.AuditedWrite(ctx, s.db, audit, s.authorizeWrite(uid, pid, access.TeamID, "mission:operate", false), func(w *database.WriteTx) (gin.H, error) {
		if _, err := w.Tx.ExecContext(ctx, `select pg_advisory_xact_lock(hashtextextended($1,0))`, fmt.Sprintf("manual-flight:%d:%d:%s", pid, cid, key)); err != nil {
			return nil, err
		}
		var savedDigest, id, status string
		var run, dock int32
		var actor int32
		err := w.Tx.QueryRowContext(ctx, `select id::text,status,task_run_id,request_digest,device_id,requested_by_user_id from connector_action_jobs where project_id=$1 and connector_instance_id=$2 and action_kind='flight-task-create' and idempotency_key=$3`, pid, cid, key).Scan(&id, &status, &run, &savedDigest, &dock, &actor)
		if err == nil {
			if digest != savedDigest || dock != did || actor != uid {
				return nil, errors.New("IDEMPOTENCY_KEY_REUSED_WITH_DIFFERENT_REQUEST")
			}
			return gin.H{"id": id, "status": status, "runId": run, "reused": true}, nil
		}
		if !errors.Is(err, sql.ErrNoRows) {
			return nil, err
		}
		var allowed bool
		err = w.Tx.QueryRowContext(ctx, `select exists(select 1 from device_external_identities i join devices d on d.id=i.device_id and d.project_id=i.project_id join device_types type on type.id=d.device_type_id join device_adapters a on a.id=i.adapter_id and a.project_id=i.project_id and a.team_id=i.team_id join connector_remote_resources route on route.connector_instance_id=a.id and route.project_id=a.project_id and route.team_id=a.team_id join project_feature_flags flag on flag.project_id=a.project_id where i.project_id=$1 and i.team_id=$2 and i.adapter_id=$3 and i.device_id=$4 and i.discovery_status='managed' and type.capability_profile_json#>>'{mission.execute,enabled}'='true' and a.status in('connecting','connected','degraded') and route.id=$5 and route.resource_kind='wayline' and route.status='active' and flag.flighthub_action_flags_json @> '{"flight.execute":true}'::jsonb)`, pid, access.TeamID, cid, did, input.WaylineResourceID).Scan(&allowed)
		if err != nil {
			return nil, err
		}
		if !allowed {
			return nil, errors.New("FLIGHTHUB_ACTION_SCOPE_MISMATCH")
		}
		// The task is an execution record, not a scheduler template.
		var task int32
		err = w.Tx.QueryRowContext(ctx, `insert into tasks(project_id,team_id,name,trigger_type,script,required_capability_code,created_by_user_id,status) values($1,$2,$3,'manual','flighthub-manual-flight','mission.execute',$4,'archived') returning id`, pid, access.TeamID, fmt.Sprintf("%s · %s", input.Name, job.String()[:8]), uid).Scan(&task)
		if err != nil {
			return nil, err
		}
		snapshot, _ := json.Marshal(gin.H{"source": "manual-device-flight", "connectorId": cid, "waylineResourceId": input.WaylineResourceID, "flightRequest": request})
		err = w.Tx.QueryRowContext(ctx, `insert into task_runs(project_id,team_id,task_id,trigger_source,status,selected_device_id,created_by_user_id,input_snapshot_json) values($1,$2,$3,'manual','dispatching',$4,$5,$6) returning id`, pid, access.TeamID, task, did, uid, snapshot).Scan(&run)
		if err != nil {
			return nil, err
		}
		contextRaw, _ := json.Marshal(gin.H{"source": "manual-device-flight", "authorizationMode": "rbac", "digest": digest})
		_, err = w.Tx.ExecContext(ctx, `insert into approval_requests(id,project_id,team_id,resource_type,resource_id,action,requested_by_user_id,status,require_separation,expires_at,context_json) values($1,$2,$3,'task_run',$4,'flighthub.flight-task.create',$5,'approved',false,now()+interval '15 minutes',$6)`, approval, pid, access.TeamID, strconv.Itoa(int(run)), uid, contextRaw)
		if err != nil {
			return nil, err
		}
		inserted, err := w.Queries.FHFlightInsert(ctx, sqlcgen.FHFlightInsertParams{P1: job, P2: pid, P3: access.TeamID, P4: cid, P5: sql.NullInt32{Int32: run, Valid: true}, P6: sql.NullInt32{Int32: did, Valid: true}, P7: sql.NullInt64{Int64: input.WaylineResourceID, Valid: true}, P9: uuid.NullUUID{UUID: approval, Valid: true}, P10: uid, P11: "flight-task-create", P12: key, P13: digest, P14: encrypted})
		if err != nil {
			return nil, err
		}
		payload, _ := json.Marshal(gin.H{"jobId": inserted.ID})
		err = w.Queries.FHFlightEnqueue(ctx, sqlcgen.FHFlightEnqueueParams{P1: pid, P2: access.TeamID, P3: "flighthub-flight-action:" + inserted.ID, P4: sql.NullString{String: inserted.ID, Valid: true}, P5: payload})
		return gin.H{"id": inserted.ID, "status": inserted.Status, "runId": run, "reused": false}, err
	})
	if err != nil {
		fhActionFailure(c, err)
		return
	}
	c.JSON(202, result)
}
