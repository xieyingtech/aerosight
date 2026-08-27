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
}

func NewLiveStreamHealthCoordinator(inspector MediaPathInspector, now func() time.Time) (*LiveStreamHealthCoordinator, error) {
	if inspector == nil {
		return nil, errors.New("MEDIA_PATH_INSPECTOR_REQUIRED")
	}
	if now == nil {
		now = time.Now
	}
	return &LiveStreamHealthCoordinator{inspector: inspector, now: now}, nil
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
			lease_expires_at=$5+interval '45 seconds',updated_at=now()
			where id=$1 and project_id=$2 and status=$6`, item.id, item.projectID, next, reason,
			coordinator.now().UTC(), item.status)
		if err != nil {
			return updated, err
		}
		if affected, _ := result.RowsAffected(); affected == 1 {
			updated++
		}
	}
	return updated, nil
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
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
