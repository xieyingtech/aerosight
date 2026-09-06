package dji

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type MediaPathStatus struct {
	Ready  bool
	Tracks []string
}

type MediaPathInspector interface {
	Inspect(context.Context, string) (MediaPathStatus, error)
}

type MediaMTXInspector struct {
	baseURL  string
	username string
	password string
	client   *http.Client
}

func NewMediaMTXInspector(baseURL, username, password string, client *http.Client) (*MediaMTXInspector, error) {
	parsed, err := url.Parse(strings.TrimSpace(baseURL))
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil {
		return nil, errors.New("MEDIAMTX_API_URL_INVALID")
	}
	if username == "" || password == "" {
		return nil, errors.New("MEDIAMTX_API_AUTH_REQUIRED")
	}
	if client == nil {
		client = &http.Client{Timeout: 3 * time.Second}
	}
	return &MediaMTXInspector{baseURL: strings.TrimRight(parsed.String(), "/"), username: username, password: password, client: client}, nil
}

func (inspector *MediaMTXInspector) Inspect(ctx context.Context, path string) (MediaPathStatus, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, inspector.baseURL+"/v3/paths/list", nil)
	if err != nil {
		return MediaPathStatus{}, err
	}
	request.SetBasicAuth(inspector.username, inspector.password)
	response, err := inspector.client.Do(request)
	if err != nil {
		return MediaPathStatus{}, err
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return MediaPathStatus{}, fmt.Errorf("MEDIAMTX_API_STATUS_%d", response.StatusCode)
	}
	var payload struct {
		Items []struct {
			Name   string   `json:"name"`
			Ready  bool     `json:"ready"`
			Tracks []string `json:"tracks"`
		} `json:"items"`
	}
	if err := json.NewDecoder(response.Body).Decode(&payload); err != nil {
		return MediaPathStatus{}, errors.New("MEDIAMTX_API_RESPONSE_INVALID")
	}
	for _, item := range payload.Items {
		if item.Name == path {
			return MediaPathStatus{Ready: item.Ready && len(item.Tracks) > 0, Tracks: item.Tracks}, nil
		}
	}
	return MediaPathStatus{}, nil
}

type LiveStreamHealthCoordinator struct {
	inspector MediaPathInspector
	now       func() time.Time
	owner     string
}

func NewLiveStreamHealthCoordinator(inspector MediaPathInspector, owner string, now func() time.Time) (*LiveStreamHealthCoordinator, error) {
	if inspector == nil {
		return nil, errors.New("MEDIA_PATH_INSPECTOR_REQUIRED")
	}
	if strings.TrimSpace(owner) == "" {
		return nil, errors.New("LIVE_STREAM_LEASE_OWNER_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	return &LiveStreamHealthCoordinator{inspector: inspector, now: now, owner: owner}, nil
}

func (coordinator *LiveStreamHealthCoordinator) ReconcileOnce(ctx context.Context, database *sql.DB) (int, error) {
	rows, err := database.QueryContext(ctx, `select id,project_id,status,ingest_ref
		from live_streams where source_type='dji' and status in ('starting','live','degraded')
		order by started_at limit 100`)
	if err != nil {
		return 0, err
	}
	type candidate struct {
		id, projectID int
		status, path  string
	}
	var candidates []candidate
	for rows.Next() {
		var item candidate
		if err := rows.Scan(&item.id, &item.projectID, &item.status, &item.path); err != nil {
			rows.Close()
			return 0, err
		}
		candidates = append(candidates, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	updated := 0
	for _, item := range candidates {
		media, inspectErr := coordinator.inspector.Inspect(ctx, item.path)
		if inspectErr != nil {
			continue
		}
		next, reason := item.status, ""
		if media.Ready {
			next = "live"
		} else if item.status == "live" {
			next, reason = "degraded", "MEDIAMTX_INPUT_MISSING"
		}
		if next == item.status {
			continue
		}
		result, err := database.ExecContext(ctx, `update live_streams set status=$3,status_reason=nullif($4,''),
			playback_ref=case when $3='live' then ingest_ref else playback_ref end,
			last_active_at=case when $3='live' then $5 else last_active_at end,
			lease_owner=$7,lease_expires_at=$5+interval '45 seconds',updated_at=now()
			where id=$1 and project_id=$2 and status=$6`, item.id, item.projectID, next, reason,
			coordinator.now().UTC(), item.status, coordinator.owner)
		if err != nil {
			return updated, err
		}
		if affected, _ := result.RowsAffected(); affected == 1 {
			updated++
		}
	}
	return updated, nil
}

func (coordinator *LiveStreamHealthCoordinator) CleanupOnce(ctx context.Context, database *sql.DB) (int, error) {
	tx, err := database.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	rows, err := tx.QueryContext(ctx, `select stream.id,stream.project_id,stream.team_id,stream.device_id,
		stream.status,coalesce(stream.vendor_stream_ref,'')
		from live_streams stream join devices device on device.id=stream.device_id and device.project_id=stream.project_id
		where stream.source_type='dji' and stream.status in ('requested','starting','live','degraded','stopping')
		  and (device.status not in ('online','degraded') or stream.lease_expires_at is null or stream.lease_expires_at<now())
		order by stream.started_at limit 100 for update of stream skip locked`)
	if err != nil {
		return 0, err
	}
	type orphan struct {
		id, projectID, teamID, deviceID int
		status, vendorStreamRef         string
	}
	var orphans []orphan
	for rows.Next() {
		var item orphan
		if err := rows.Scan(&item.id, &item.projectID, &item.teamID, &item.deviceID, &item.status, &item.vendorStreamRef); err != nil {
			rows.Close()
			return 0, err
		}
		orphans = append(orphans, item)
	}
	if err := rows.Close(); err != nil {
		return 0, err
	}
	for _, item := range orphans {
		if item.status == "stopping" || item.vendorStreamRef == "" {
			if _, err := tx.ExecContext(ctx, `update live_streams set status=case when $3='stopping' then 'stopped' else 'failed' end,
				status_reason='LIVE_STREAM_ORPHAN_RECONCILED',ended_at=$4,playback_ref=null,
				lease_owner=null,lease_expires_at=null,updated_at=now() where id=$1 and project_id=$2`,
				item.id, item.projectID, item.status, coordinator.now().UTC()); err != nil {
				return 0, err
			}
			continue
		}
		var commandID string
		err := tx.QueryRowContext(ctx, `insert into device_commands(
			id,project_id,team_id,device_id,live_stream_id,command_key,idempotency_key,capability_code,
			parameters_json,safety_context_json,status,priority,deadline_at)
			values(gen_random_uuid(),$1,$2,$3,$4,'stop',$5,'stream.video.control',$6,$7,'dispatchable',40,$8::timestamptz+interval '30 seconds')
			on conflict(live_stream_id,command_key) where live_stream_id is not null do update
			set idempotency_key=device_commands.idempotency_key returning id::text`,
			item.projectID, item.teamID, item.deviceID, item.id, fmt.Sprintf("live-stream:%d:stop", item.id),
			jsonObject(map[string]any{"video_id": item.vendorStreamRef}),
			jsonObject(map[string]any{"liveStreamId": item.id, "cleanup": true}), coordinator.now().UTC()).Scan(&commandID)
		if err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `insert into outbox_events(project_id,team_id,event_id,event_type,payload_json)
			values($1,$2,$3,'device.command.dispatch',$4) on conflict(event_id) do nothing`,
			item.projectID, item.teamID, "device.command.dispatch:"+commandID,
			jsonObject(map[string]any{"commandId": commandID})); err != nil {
			return 0, err
		}
		if _, err := tx.ExecContext(ctx, `update live_streams set status='stopping',status_reason='LIVE_STREAM_CLEANUP_REQUESTED',
			lease_owner=$3,lease_expires_at=$4::timestamptz+interval '45 seconds',updated_at=now()
			where id=$1 and project_id=$2`, item.id, item.projectID, coordinator.owner, coordinator.now().UTC()); err != nil {
			return 0, err
		}
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return len(orphans), nil
}

func (coordinator *LiveStreamHealthCoordinator) Run(ctx context.Context, database *sql.DB, interval time.Duration) error {
	if interval <= 0 {
		interval = 2 * time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		if _, err := coordinator.ReconcileOnce(ctx, database); err != nil && ctx.Err() == nil {
			return err
		}
		if _, err := coordinator.CleanupOnce(ctx, database); err != nil && ctx.Err() == nil {
			return err
		}
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
