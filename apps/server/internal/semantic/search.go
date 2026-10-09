package semantic

import (
	"context"
	"database/sql"
	"errors"
	"github.com/google/uuid"
	"math"
	"strings"
	"time"
)

type SearchInput struct {
	Query string     `json:"query"`
	Limit int        `json:"limit,omitempty"`
	Start *time.Time `json:"start,omitempty"`
	End   *time.Time `json:"end,omitempty"`
}

func (in *SearchInput) Validate() error {
	in.Query = strings.TrimSpace(in.Query)
	if in.Query == "" || len(in.Query) > 2000 {
		return errors.New("MEDIA_SEARCH_INPUT_INVALID")
	}
	if in.Limit == 0 {
		in.Limit = 10
	}
	if in.Limit < 1 || in.Limit > 20 {
		return errors.New("MEDIA_SEARCH_INPUT_INVALID")
	}
	if (in.Start == nil) != (in.End == nil) || (in.Start != nil && !in.Start.Before(*in.End)) {
		return errors.New("MEDIA_SEARCH_INPUT_INVALID")
	}
	return nil
}

type Match struct {
	ID            string     `json:"segmentId"`
	AssetID       int32      `json:"assetId"`
	Version       int        `json:"version"`
	StartMS       int64      `json:"startMs"`
	EndMS         int64      `json:"endMs"`
	Description   string     `json:"description"`
	TimeQuality   string     `json:"timeQuality"`
	CapturedStart *time.Time `json:"capturedStart"`
	CapturedEnd   *time.Time `json:"capturedEnd"`
	Score         float64    `json:"score"`
}
type AuthorizeAsset func(context.Context, int32) error

// Search treats Qdrant IDs and scores as untrusted candidates. PostgreSQL is authoritative.
func (s *Service) Search(ctx context.Context, pid int32, in SearchInput, authorize AuthorizeAsset) ([]Match, bool, error) {
	if err := in.Validate(); err != nil {
		return nil, false, err
	}
	vectors, err := s.Client.Embed(ctx, []string{in.Query}, true)
	if err != nil {
		return nil, false, err
	}
	matches := []Match{}
	seen := map[string]bool{}
	for offset := 0; offset < 500; offset += 100 {
		hits, err := s.Client.Query(ctx, pid, vectors[0], offset)
		if err != nil {
			return nil, false, err
		}
		for _, hit := range hits {
			if _, e := uuid.Parse(hit.ID); e != nil {
				continue
			}
			if seen[hit.ID] || math.IsNaN(hit.Score) || math.IsInf(hit.Score, 0) {
				continue
			}
			seen[hit.ID] = true
			var m Match
			var a, b sql.NullTime
			err = s.DB.QueryRowContext(ctx, `SELECT s.id::text,s.asset_id,j.source_version,s.start_ms,s.end_ms,s.description,s.time_quality,s.captured_start,s.captured_end
 FROM media_index_segments s JOIN media_index_jobs j ON j.id=s.job_id JOIN assets a ON a.id=s.asset_id AND a.project_id=s.project_id
 WHERE s.id=$1::uuid AND s.project_id=$2 AND j.project_id=$2 AND s.active AND j.state='indexed' AND j.space=$3
 AND a.status='available' AND a.deleted_at IS NULL AND a.version=j.source_version AND coalesce(a.checksum_sha256,'')=j.source_checksum
 AND ($4::timestamptz IS NULL OR (s.captured_start<$5 AND s.captured_end>$4))`, hit.ID, pid, s.Client.Config.Space(), in.Start, in.End).Scan(&m.ID, &m.AssetID, &m.Version, &m.StartMS, &m.EndMS, &m.Description, &m.TimeQuality, &a, &b)
			if errors.Is(err, sql.ErrNoRows) {
				continue
			}
			if err != nil {
				return nil, false, err
			}
			if err = authorize(ctx, m.AssetID); err != nil {
				continue
			}
			if a.Valid {
				m.CapturedStart = &a.Time
			}
			if b.Valid {
				m.CapturedEnd = &b.Time
			}
			m.Score = hit.Score
			matches = append(matches, m)
			if len(matches) == in.Limit {
				return matches, true, nil
			}
		}
		if len(hits) < 100 {
			return matches, false, nil
		}
	}
	return matches, true, nil
}
