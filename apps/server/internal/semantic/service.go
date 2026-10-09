package semantic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"database/sql/driver"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/media"
)

type Service struct {
	DB           *sql.DB
	Client       Client
	Storage      media.ObjectStorage
	Root, Secret string
	Remote       algorithm.RemoteAlgorithmAssetReader
	Logger       *slog.Logger
}
type job struct {
	ID                    int64
	Source                source
	Key, ArtifactChecksum sql.NullString
	Attempts              int
}

func (s *Service) Reconcile(ctx context.Context) error {
	_, err := s.DB.ExecContext(ctx, `INSERT INTO media_index_jobs(project_id,asset_id,source_version,source_checksum,space)
 SELECT project_id,id,version,coalesce(checksum_sha256,''),$1 FROM assets
 WHERE status='available' AND deleted_at IS NULL AND mime_type IN ('image/jpeg','image/png','video/mp4')
 ON CONFLICT(asset_id,source_version,source_checksum,space) DO NOTHING`, s.Client.Config.Space())
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE media_index_jobs j SET state='obsolete',updated_at=now() FROM assets a
 WHERE j.asset_id=a.id AND j.state<>'obsolete' AND (a.status<>'available' OR a.deleted_at IS NOT NULL OR a.version<>j.source_version OR coalesce(a.checksum_sha256,'')<>j.source_checksum)`)
	if err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE media_index_segments s SET active=false FROM media_index_jobs j WHERE s.job_id=j.id AND j.state='obsolete' AND s.active`)
	if err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT s.id::text,j.space FROM media_index_segments s JOIN media_index_jobs j ON j.id=s.job_id WHERE NOT s.active AND NOT s.index_deleted LIMIT 200`)
	if err != nil {
		return err
	}
	ids := []string{}
	bySpace := map[string][]string{}
	for rows.Next() {
		var id, space string
		if err = rows.Scan(&id, &space); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
		bySpace[space] = append(bySpace[space], id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for space, group := range bySpace {
		if err = s.Client.DeleteSpace(ctx, space, group); err != nil {
			return err
		}
	}
	for _, id := range ids {
		if _, err = s.DB.ExecContext(ctx, `UPDATE media_index_segments SET index_deleted=true WHERE id=$1 AND NOT active`, id); err != nil {
			return err
		}
	}
	rows, err = s.DB.QueryContext(ctx, `SELECT id::text,space FROM media_index_retired_points LIMIT 200`)
	if err != nil {
		return err
	}
	ids = []string{}
	bySpace = map[string][]string{}
	for rows.Next() {
		var id, space string
		if err = rows.Scan(&id, &space); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
		bySpace[space] = append(bySpace[space], id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for space, group := range bySpace {
		if err = s.Client.DeleteSpace(ctx, space, group); err != nil {
			return err
		}
		for _, id := range group {
			if _, err = s.DB.ExecContext(ctx, `DELETE FROM media_index_retired_points WHERE id=$1 AND space=$2`, id, space); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Service) Run(ctx context.Context) error {
	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()
	for {
		if err := s.Tick(ctx); err != nil && ctx.Err() == nil && s.Logger != nil {
			s.Logger.Warn("media semantic indexing degraded", "code", safeCode(err))
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}
func (s *Service) Tick(ctx context.Context) error {
	if err := s.Reconcile(ctx); err != nil {
		return err
	}
	if err := s.Client.Ensure(ctx); err != nil {
		return err
	}
	rows, err := s.DB.QueryContext(ctx, `SELECT id FROM media_index_jobs WHERE space=$1 AND state IN ('queued','running','failed') AND (state='running' OR attempts<10) AND next_attempt_at<=now() ORDER BY next_attempt_at,id LIMIT 20`, s.Client.Config.Space())
	if err != nil {
		return err
	}
	ids := []int64{}
	for rows.Next() {
		var id int64
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return err
	}
	for _, id := range ids {
		processed, e := s.Process(ctx, id)
		if e != nil {
			return e
		}
		if processed {
			return nil
		}
	}
	return nil
}
func (s *Service) Process(ctx context.Context, id int64) (bool, error) {
	conn, err := s.DB.Conn(ctx)
	if err != nil {
		return false, err
	}
	defer conn.Close()
	var locked bool
	if err = conn.QueryRowContext(ctx, `SELECT pg_try_advisory_lock(7391,$1::integer)`, id).Scan(&locked); err != nil {
		return false, err
	}
	if !locked {
		return false, nil
	}
	defer func() {
		release, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, e := conn.ExecContext(release, `SELECT pg_advisory_unlock(7391,$1::integer)`, id); e != nil {
			_ = conn.Raw(func(any) error { return driver.ErrBadConn })
		}
	}()
	var j job
	j.ID = id
	err = conn.QueryRowContext(ctx, `UPDATE media_index_jobs SET state='running',attempts=attempts+CASE WHEN state='running' THEN 0 ELSE 1 END,updated_at=now(),error_code=NULL WHERE id=$1 AND space=$2 AND state IN ('queued','running','failed') AND (state='running' OR attempts<10) AND next_attempt_at<=now() RETURNING attempts,artifact_key,artifact_checksum`, id, s.Client.Config.Space()).Scan(&j.Attempts, &j.Key, &j.ArtifactChecksum)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	work, cancel := context.WithTimeout(ctx, 30*time.Minute)
	defer cancel()
	err = conn.QueryRowContext(work, `SELECT a.id,a.project_id,a.version,coalesce(a.checksum_sha256,''),a.storage_key,a.mime_type,a.captured_at,a.metadata_json,(a.remote_connector_id IS NOT NULL) FROM assets a JOIN media_index_jobs j ON j.asset_id=a.id WHERE j.id=$1 AND a.project_id=j.project_id AND a.version=j.source_version AND coalesce(a.checksum_sha256,'')=j.source_checksum AND a.status='available' AND a.deleted_at IS NULL`, id).Scan(&j.Source.ID, &j.Source.ProjectID, &j.Source.Version, &j.Source.Checksum, &j.Source.Key, &j.Source.Mime, &j.Source.Captured, &j.Source.Metadata, &j.Source.Remote)
	if errors.Is(err, sql.ErrNoRows) {
		_, err = conn.ExecContext(ctx, `UPDATE media_index_jobs SET state='obsolete',updated_at=now() WHERE id=$1`, id)
		return true, err
	}
	if err == nil {
		err = s.processArtifact(work, j)
	}
	if err != nil {
		update, c := context.WithTimeout(context.Background(), 5*time.Second)
		defer c()
		_, e := conn.ExecContext(update, `UPDATE media_index_jobs SET state='failed',error_code=$2,next_attempt_at=now()+$3*interval '1 second',updated_at=now() WHERE id=$1 AND state='running'`, id, safeCode(err), min(3600, 15*(1<<min(j.Attempts, 8))))
		if e != nil {
			return true, e
		}
		return true, err
	}
	return true, nil
}
func (s *Service) processArtifact(ctx context.Context, j job) error {
	var artifact Artifact
	if j.Key.Valid {
		obj, err := s.Storage.GetObject(ctx, j.Key.String)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(obj.Body)
		if hex.EncodeToString(sum[:]) != j.ArtifactChecksum.String || json.Unmarshal(obj.Body, &artifact) != nil {
			return errors.New("ARTIFACT_INVALID")
		}
		if artifact.Space != s.Client.Config.Space() || artifact.AssetID != j.Source.ID || artifact.ProjectID != j.Source.ProjectID || artifact.Version != j.Source.Version || artifact.Checksum != j.Source.Checksum {
			return errors.New("ARTIFACT_INVALID")
		}
	}
	checkpoint := func(partial Artifact) error {
		raw, err := json.Marshal(partial)
		if err != nil {
			return err
		}
		sum := sha256.Sum256(raw)
		key := fmt.Sprintf("projects/%d/semantic/%s/jobs/%d/%s.json", j.Source.ProjectID, partial.Space, j.ID, hex.EncodeToString(sum[:]))
		object, err := s.Storage.PutObject(ctx, key, bytes.NewReader(raw), "application/json")
		if err != nil {
			return err
		}
		result, err := s.DB.ExecContext(ctx, `UPDATE media_index_jobs SET artifact_key=$2,artifact_checksum=$3,updated_at=now() WHERE id=$1 AND state='running'`, j.ID, object.Key, hex.EncodeToString(sum[:]))
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return errors.New("SOURCE_UNAVAILABLE")
		}
		return nil
	}
	if !artifact.Complete {
		var err error
		artifact, err = s.analyze(ctx, j.Source, artifact, checkpoint)
		if err != nil {
			return err
		}
	}
	if artifact.Space != s.Client.Config.Space() || artifact.AssetID != j.Source.ID || artifact.ProjectID != j.Source.ProjectID || artifact.Version != j.Source.Version || artifact.Checksum != j.Source.Checksum || len(artifact.Segments) == 0 {
		return errors.New("ARTIFACT_INVALID")
	}
	tx, err := s.DB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var valid bool
	err = tx.QueryRowContext(ctx, `SELECT status='available' AND deleted_at IS NULL AND version=$3 AND coalesce(checksum_sha256,'')=$4 FROM assets WHERE id=$1 AND project_id=$2 FOR SHARE`, j.Source.ID, j.Source.ProjectID, j.Source.Version, j.Source.Checksum).Scan(&valid)
	if err != nil {
		return err
	}
	if !valid {
		return errors.New("SOURCE_UNAVAILABLE")
	}
	points := []Point{}
	for _, seg := range artifact.Segments {
		if !validVector(seg.Vector, s.Client.Config.Dimension) || seg.StartMS < 0 || seg.EndMS <= seg.StartMS {
			return errors.New("ARTIFACT_INVALID")
		}
		_, err = tx.ExecContext(ctx, `INSERT INTO media_index_segments(id,job_id,project_id,asset_id,start_ms,end_ms,description,time_quality,captured_start,captured_end) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10) ON CONFLICT(id) DO UPDATE SET active=true,index_deleted=false`, seg.ID, j.ID, artifact.ProjectID, artifact.AssetID, seg.StartMS, seg.EndMS, seg.Description, seg.TimeQuality, seg.CapturedStart, seg.CapturedEnd)
		if err != nil {
			return err
		}
		points = append(points, Point{ID: seg.ID, Vector: seg.Vector, Payload: map[string]any{"project_id": artifact.ProjectID, "asset_id": artifact.AssetID, "version": artifact.Version, "space": artifact.Space}})
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	if err = s.Client.Upsert(ctx, points); err != nil {
		return err
	}
	_, err = s.DB.ExecContext(ctx, `UPDATE media_index_jobs j SET state='indexed',error_code=NULL,updated_at=now() FROM assets a WHERE j.id=$1 AND j.state='running' AND a.id=j.asset_id AND a.project_id=j.project_id AND a.status='available' AND a.deleted_at IS NULL AND a.version=j.source_version AND coalesce(a.checksum_sha256,'')=j.source_checksum`, j.ID)
	return err
}

type Status struct {
	Enabled   bool       `json:"enabled"`
	State     string     `json:"state"`
	Attempts  int        `json:"attempts"`
	ErrorCode *string    `json:"errorCode"`
	Segments  int        `json:"segments"`
	UpdatedAt *time.Time `json:"updatedAt"`
}

func (s *Service) Status(ctx context.Context, pid, aid int32) (Status, error) {
	out := Status{Enabled: true, State: "queued"}
	var code sql.NullString
	var updated time.Time
	err := s.DB.QueryRowContext(ctx, `SELECT j.state,j.attempts,j.error_code,j.updated_at,(SELECT count(*) FROM media_index_segments x WHERE x.job_id=j.id AND active) FROM media_index_jobs j JOIN assets a ON a.id=j.asset_id AND a.project_id=j.project_id WHERE j.project_id=$1 AND j.asset_id=$2 AND j.space=$3 AND j.source_version=a.version AND j.source_checksum=coalesce(a.checksum_sha256,'') ORDER BY j.id DESC LIMIT 1`, pid, aid, s.Client.Config.Space()).Scan(&out.State, &out.Attempts, &code, &updated, &out.Segments)
	if errors.Is(err, sql.ErrNoRows) {
		var mime string
		if e := s.DB.QueryRowContext(ctx, `SELECT coalesce(mime_type,'') FROM assets WHERE id=$1 AND project_id=$2 AND status='available' AND deleted_at IS NULL`, aid, pid).Scan(&mime); e != nil {
			return out, e
		}
		if mime != "image/jpeg" && mime != "image/png" && mime != "video/mp4" {
			out.State = "unsupported"
		}
		return out, nil
	}
	if err != nil {
		return out, err
	}
	out.UpdatedAt = &updated
	if code.Valid {
		out.ErrorCode = &code.String
	}
	return out, nil
}
func (s *Service) Retry(ctx context.Context, pid, aid int32) error {
	return s.RetryTx(ctx, s.DB, pid, aid)
}
func (s *Service) RetryTx(ctx context.Context, executor interface {
	ExecContext(context.Context, string, ...any) (sql.Result, error)
}, pid, aid int32) error {
	result, err := executor.ExecContext(ctx, `INSERT INTO media_index_jobs(project_id,asset_id,source_version,source_checksum,space) SELECT project_id,id,version,coalesce(checksum_sha256,''),$3 FROM assets WHERE project_id=$1 AND id=$2 AND status='available' AND deleted_at IS NULL AND mime_type IN ('image/jpeg','image/png','video/mp4') ON CONFLICT(asset_id,source_version,source_checksum,space) DO UPDATE SET state='queued',attempts=0,next_attempt_at=now(),error_code=NULL,updated_at=now() WHERE media_index_jobs.state<>'running'`, pid, aid, s.Client.Config.Space())
	if err != nil {
		return err
	}
	count, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if count == 0 {
		return errors.New("MEDIA_INDEX_BUSY_OR_UNSUPPORTED")
	}
	return nil
}
