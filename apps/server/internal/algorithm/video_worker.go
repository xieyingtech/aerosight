package algorithm

import (
	"aerosight/server/internal/media"
	"aerosight/server/internal/outbox"
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/google/uuid"
	"image/jpeg"
	"io"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

const MaxVideoFrames = 10000

func ValidVideoFPS(fps float64) bool {
	return !math.IsNaN(fps) && !math.IsInf(fps, 0) && fps >= 0.2 && fps <= 5
}

type VideoFrame struct {
	Index  int             `json:"index"`
	TimeMs float64         `json:"timeMs"`
	Width  int             `json:"width"`
	Height int             `json:"height"`
	Result CanonicalResult `json:"result"`
}
type VideoSummary struct {
	SchemaVersion       string  `json:"schemaVersion"`
	FPS                 float64 `json:"fps"`
	DurationSeconds     float64 `json:"durationSeconds"`
	ProcessedFrames     int     `json:"processedFrames"`
	TotalFrames         int     `json:"totalFrames"`
	Complete            bool    `json:"complete"`
	AnnotationObjectKey string  `json:"annotationObjectKey,omitempty"`
}

func (p *Processor) WithVideoWorker(db *sql.DB, storage media.ObjectStorage) *Processor {
	p.inspectionDB = db
	p.videoStorage = storage
	return p
}
func (p *Processor) RunVideo(ctx context.Context, interval time.Duration) error {
	return runInspectionLoop(ctx, interval, p.ProcessVideoNext)
}

// Session advisory locks prevent two replicas from processing the same video.
// PostgreSQL releases the lock after a crash; durable frame artifacts allow resume.
func (p *Processor) ProcessVideoNext(ctx context.Context) (bool, error) {
	rows, err := p.inspectionDB.QueryContext(ctx, `select id::text from algorithm_runs where status in ('queued','running') and input_snapshot_json#>'{context,videoAnalysis}' is not null order by created_at limit 8`)
	if err != nil {
		return false, err
	}
	var ids []string
	for rows.Next() {
		var id string
		if err = rows.Scan(&id); err != nil {
			rows.Close()
			return false, err
		}
		ids = append(ids, id)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return false, err
	}
	for _, id := range ids {
		conn, e := p.inspectionDB.Conn(ctx)
		if e != nil {
			return false, e
		}
		var locked bool
		e = conn.QueryRowContext(ctx, `select pg_try_advisory_lock(hashtextextended($1,0))`, "video-analysis:"+id).Scan(&locked)
		if e != nil || !locked {
			conn.Close()
			if e != nil {
				return false, e
			}
			continue
		}
		e = p.processVideo(ctx, id)
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		_, unlockErr := conn.ExecContext(cleanup, `select pg_advisory_unlock(hashtextextended($1,0))`, "video-analysis:"+id)
		cancel()
		conn.Close()
		if e != nil && ctx.Err() == nil {
			_, saveErr := p.inspectionDB.ExecContext(ctx, `update algorithm_runs set status='failed',error_code='video_analysis_failed',error_message=$2,finished_at=now() where id=$1 and status in ('queued','running')`, id, e.Error())
			if saveErr != nil {
				return true, saveErr
			}
		}
		if unlockErr != nil {
			return true, unlockErr
		}
		return true, nil
	}
	return false, nil
}
func (p *Processor) processVideo(ctx context.Context, id string) error {
	if p.videoStorage == nil {
		return errors.New("video object storage is unavailable")
	}
	var event outbox.Event
	var state string
	tx, err := p.inspectionDB.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err = tx.QueryRowContext(ctx, `select project_id,team_id,status from algorithm_runs where id=$1 for update`, id).Scan(&event.ProjectID, &event.TeamID, &state); err != nil {
		return err
	}
	if state != "queued" && state != "running" {
		return nil
	}
	if _, err = tx.ExecContext(ctx, `update algorithm_runs set status='queued' where id=$1`, id); err != nil {
		return err
	}
	event.Payload, _ = json.Marshal(map[string]string{"runId": id})
	prepared, err := p.prepare(ctx, tx, event)
	if err != nil {
		return err
	}
	if prepared == nil {
		return tx.Commit()
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	input := prepared.request.Input
	options, _ := input.Context["videoAnalysis"].(map[string]any)
	fps, _ := options["fps"].(float64)
	if !ValidVideoFPS(fps) || input.Definition.ExecutionMode != "synchronous" {
		return errors.New("unsupported video analysis settings")
	}
	var key, checksum string
	var metadata []byte
	err = p.inspectionDB.QueryRowContext(ctx, `select storage_key,coalesce(checksum_sha256,checksum,''),metadata_json from assets where id=$1 and project_id=$2 and version=$3 and status='available'`, input.InputAsset.AssetID, input.ProjectID, input.InputAsset.Version).Scan(&key, &checksum, &metadata)
	if err != nil || checksum != input.InputAsset.ChecksumSHA256 {
		return errors.New("source video version is unavailable")
	}
	var meta struct {
		Duration float64 `json:"durationSeconds"`
	}
	json.Unmarshal(metadata, &meta)
	if meta.Duration <= 0 || math.Ceil(meta.Duration*fps) > MaxVideoFrames {
		return errors.New("video exceeds the analysis frame limit")
	}
	dir, err := os.MkdirTemp("", "aerosight-video-")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	filename := filepath.Join(dir, "source.mp4")
	var source io.ReadCloser
	if opener, ok := p.videoStorage.(interface {
		OpenObject(context.Context, string) (io.ReadSeekCloser, error)
	}); ok {
		source, err = opener.OpenObject(ctx, key)
	} else {
		var object media.Object
		object, err = p.videoStorage.GetObject(ctx, key)
		source = io.NopCloser(bytes.NewReader(object.Body))
	}
	if err != nil {
		return errors.New("source video download failed")
	}
	file, err := os.Create(filename)
	if err != nil {
		source.Close()
		return err
	}
	hash := sha256.New()
	size, copyErr := io.Copy(io.MultiWriter(file, hash), io.LimitReader(source, 512*1024*1024+1))
	source.Close()
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil || size > 512*1024*1024 {
		return errors.New("source video could not be copied")
	}
	if hex.EncodeToString(hash.Sum(nil)) != checksum {
		return errors.New("source video checksum changed")
	}
	summary := VideoSummary{SchemaVersion: "aerosight.video-annotations/v1", FPS: fps, DurationSeconds: meta.Duration, TotalFrames: int(math.Ceil(meta.Duration * fps))}
	saveProgress := func() error {
		canonical, _ := json.Marshal(map[string]any{"kind": "video", "result": summary})
		_, e := p.inspectionDB.ExecContext(ctx, `update algorithm_runs set canonical_result_json=$2 where id=$1 and status='running'`, id, canonical)
		return e
	}
	if err = saveProgress(); err != nil {
		return err
	}
	filter := fmt.Sprintf("setpts=PTS-STARTPTS,fps=fps=%g:start_time=0:round=up:eof_action=pass,scale=w='min(1280,iw)':h='min(1280,ih)':force_original_aspect_ratio=decrease", fps)
	cmd := exec.CommandContext(ctx, "ffmpeg", "-v", "error", "-i", filename, "-map", "0:v:0", "-an", "-vf", filter, "-frames:v", fmt.Sprint(MaxVideoFrames+1), "-f", "image2pipe", "-c:v", "mjpeg", "-q:v", "5", "pipe:1")
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	cmd.Stderr = io.Discard
	if err = cmd.Start(); err != nil {
		return errors.New("video decoder could not start")
	}
	waited := false
	defer func() {
		if !waited {
			cmd.Process.Kill()
			cmd.Wait()
		}
	}()
	reader := bufio.NewReader(stdout)
	var annotations bytes.Buffer
	lastProgress := time.Now()
	for index := 0; ; index++ {
		frameBytes, e := readMJPEGFrame(reader)
		if errors.Is(e, io.EOF) {
			break
		} else if e != nil {
			return errors.New("video frame stream failed")
		}
		img, e := jpeg.Decode(bytes.NewReader(frameBytes))
		if errors.Is(e, io.EOF) {
			break
		}
		if e != nil {
			return errors.New("video frame decoding failed")
		}
		if index >= MaxVideoFrames {
			return errors.New("video exceeds the analysis frame limit")
		}
		frame := VideoFrame{Index: index, TimeMs: float64(index) * 1000 / fps, Width: img.Bounds().Dx(), Height: img.Bounds().Dy()}
		frameKey := fmt.Sprintf("projects/%d/algorithm-runs/%s/frames/%06d", input.ProjectID, id, index)
		cached, e := p.videoStorage.GetObject(ctx, frameKey+".json")
		if e == nil {
			if json.Unmarshal(cached.Body, &frame) != nil || frame.Index != index {
				return errors.New("stored video annotation is invalid")
			}
		} else {
			var pixels bytes.Buffer
			pixels.Write(frameBytes)
			object, e := p.videoStorage.PutObject(ctx, frameKey+".jpg", bytes.NewReader(pixels.Bytes()), "image/jpeg")
			if e != nil {
				return errors.New("frame storage failed")
			}
			request := prepared.request
			request.Input = input
			request.Input.Context = make(map[string]any)
			for k, v := range input.Context {
				request.Input.Context[k] = v
			}
			delete(request.Input.Context, "videoAnalysis")
			request.Input.Context["videoAssetId"] = input.InputAsset.AssetID
			request.Input.Context["mediaTimeSeconds"] = frame.TimeMs / 1000
			request.Input.Context["frameIndex"] = index
			request.Input.RunID = uuid.NewSHA1(uuid.MustParse(id), []byte(fmt.Sprint(index))).String()
			request.Input.Context["videoRunId"] = id
			expires := time.Now().Add(5 * time.Minute).UTC()
			var frameURL string
			if presigner, ok := p.videoStorage.(interface {
				PresignRead(context.Context, string, time.Duration) (*media.Access, error)
			}); ok {
				access, signErr := presigner.PresignRead(ctx, object.Key, 5*time.Minute)
				if signErr != nil {
					return errors.New("frame access could not be issued")
				}
				if access != nil {
					frameURL = access.URL
				}
			}
			if frameURL == "" {
				signer, ok := p.assetIssuer.(interface {
					IssueFrameURL(int, string, int, string, time.Time) (string, error)
				})
				if !ok {
					return errors.New("frame access signing is unavailable")
				}
				frameURL, e = signer.IssueFrameURL(input.ProjectID, id, index, object.ChecksumSHA256, expires)
				if e != nil {
					return errors.New("frame access could not be issued")
				}
			}
			request.Input.InputAsset = AssetReference{AssetID: input.InputAsset.AssetID, Version: input.InputAsset.Version, ChecksumSHA256: object.ChecksumSHA256, MIMEType: "image/jpeg", AccessURL: frameURL, AccessExpiresAt: expires}
			attempts := &bufferedAttempts{}
			outcome, e := NewHTTPJSONAdapter(p.client, attempts, p.breaker).Execute(ctx, request)
			diagnostics, marshalErr := json.Marshal(attempts.values)
			if marshalErr != nil {
				return marshalErr
			}
			if _, storeErr := p.videoStorage.PutObject(ctx, frameKey+".attempts.json", bytes.NewReader(diagnostics), "application/json"); storeErr != nil {
				return errors.New("frame diagnostics storage failed")
			}
			if e != nil {
				return errors.New("frame algorithm execution failed")
			}
			if outcome.Kind == "accepted" || outcome.Kind == "waiting_callback" {
				return errors.New("video analysis requires a synchronous frame algorithm")
			}
			frame.Result = outcome.Result
			if frame.Result.Kind == "" {
				frame.Result = CanonicalResult{Kind: ResultDetection, Detections: outcome.Detections}
			}
			if _, e = p.videoStorage.PutObject(ctx, frameKey+".raw.json", bytes.NewReader(outcome.Raw), "application/json"); e != nil {
				return errors.New("frame raw result storage failed")
			}
			encoded, _ := json.Marshal(frame)
			if _, e = p.videoStorage.PutObject(ctx, frameKey+".json", bytes.NewReader(encoded), "application/json"); e != nil {
				return errors.New("frame annotation storage failed")
			}
		}
		encoded, _ := json.Marshal(frame)
		annotations.Write(encoded)
		annotations.WriteByte('\n')
		if annotations.Len() > 64*1024*1024 {
			return errors.New("video annotations exceed 64 MB")
		}
		summary.ProcessedFrames = index + 1
		if time.Since(lastProgress) >= time.Second {
			if err = saveProgress(); err != nil {
				return err
			}
			lastProgress = time.Now()
		}
	}
	err = cmd.Wait()
	waited = true
	if err != nil {
		return errors.New("video decoding did not complete")
	}
	if summary.ProcessedFrames == 0 {
		return errors.New("video has no decodable frames")
	}
	object, err := p.videoStorage.PutObject(ctx, fmt.Sprintf("projects/%d/algorithm-runs/%s/annotations.jsonl", input.ProjectID, id), bytes.NewReader(annotations.Bytes()), "application/x-ndjson")
	if err != nil {
		return errors.New("video annotations storage failed")
	}
	summary.Complete = true
	summary.TotalFrames = summary.ProcessedFrames
	summary.AnnotationObjectKey = object.Key
	canonical, _ := json.Marshal(map[string]any{"kind": "video", "result": summary})
	_, err = p.inspectionDB.ExecContext(ctx, `update algorithm_runs set status='succeeded',canonical_result_json=$2,raw_result_object_key=$3,raw_result_checksum_sha256=$4,finished_at=now() where id=$1 and status='running'`, id, canonical, object.Key, object.ChecksumSHA256)
	return err
}

// FFmpeg's MJPEG encoder emits JPEGs without embedded thumbnails. Split at
// EOI before decoding so the JPEG decoder cannot read ahead into the next frame.
func readMJPEGFrame(reader *bufio.Reader) ([]byte, error) {
	var frame []byte
	for {
		part, err := reader.ReadBytes(0xd9)
		frame = append(frame, part...)
		if len(frame) > 4*1024*1024 {
			return nil, errors.New("encoded frame exceeds limit")
		}
		if err != nil {
			if err == io.EOF && len(frame) == 0 {
				return nil, io.EOF
			}
			return nil, io.ErrUnexpectedEOF
		}
		if len(frame) >= 2 && frame[len(frame)-2] == 0xff {
			return frame, nil
		}
	}
}
