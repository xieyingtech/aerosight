package httpapi

import (
	"aerosight/server/internal/algorithm"
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/media"
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"image"
	"image/draw"
	"image/jpeg"
	"io"
	"math"
	"net/url"
	"strings"
	"time"
)

type objectQueryInput struct {
	AlgorithmRunID        string    `json:"algorithmRunId"`
	Labels                []string  `json:"labels"`
	MinConfidence         float64   `json:"minConfidence"`
	SelectedDetectionKeys *[]string `json:"selectedDetectionKeys"`
	SelectionReason       string    `json:"selectionReason"`
	IncludeImage          *bool     `json:"includeImage"`
}
type queryBox struct {
	Type   string  `json:"type"`
	X      float64 `json:"x"`
	Y      float64 `json:"y"`
	Width  float64 `json:"width"`
	Height float64 `json:"height"`
}
type queryDetection struct {
	DetectionKey  string    `json:"detectionKey"`
	Label         string    `json:"label"`
	Confidence    float64   `json:"confidence"`
	PixelGeometry *queryBox `json:"pixelGeometry,omitempty"`
}
type objectQueryRun struct {
	ID            string `json:"id"`
	Status        string `json:"status"`
	InputAssetID  int32  `json:"inputAssetId"`
	InputSnapshot struct {
		InputAsset struct {
			AssetID        int32  `json:"assetId"`
			Version        int    `json:"version"`
			ChecksumSHA256 string `json:"checksumSha256"`
		} `json:"inputAsset"`
	} `json:"inputSnapshot"`
	CanonicalResult struct {
		Kind   string `json:"kind"`
		Result struct {
			Detections []queryDetection `json:"detections"`
		} `json:"result"`
	} `json:"canonicalResult"`
}

func objectQuerySchema() map[string]any {
	return agentObject(map[string]any{
		"algorithmRunId":        agentText(),
		"labels":                map[string]any{"type": "array", "maxItems": 30, "items": agentText()},
		"minConfidence":         map[string]any{"type": "number", "minimum": 0, "maximum": 1},
		"selectedDetectionKeys": map[string]any{"type": "array", "maxItems": 100, "uniqueItems": true, "items": agentText()},
		"selectionReason":       map[string]any{"type": "string", "maxLength": 1000},
		"includeImage":          map[string]any{"type": "boolean"},
	})
}
func parseObjectQuery(raw json.RawMessage) (objectQueryInput, error) {
	var input objectQueryInput
	bad := errors.New("AGENT_TOOL_INPUT_INVALID")
	if len(raw) > 32*1024 {
		return input, bad
	}
	schema, _ := json.Marshal(objectQuerySchema())
	args, err := parseFHInput(raw, schema)
	if err != nil || chatScopeArgument(args) != nil {
		return input, bad
	}
	if json.Unmarshal(raw, &input) != nil {
		return input, bad
	}
	if input.AlgorithmRunID != "" {
		if _, err := uuid.Parse(input.AlgorithmRunID); err != nil {
			return input, bad
		}
	}
	if input.AlgorithmRunID == "" && (len(input.Labels) > 0 || input.SelectedDetectionKeys != nil || input.MinConfidence != 0 || input.SelectionReason != "") {
		return input, bad
	}
	if input.SelectedDetectionKeys != nil && strings.TrimSpace(input.SelectionReason) == "" {
		return input, bad
	}
	return input, nil
}
func filterQueryDetections(all []queryDetection, input objectQueryInput) ([]queryDetection, error) {
	labels := map[string]bool{}
	for _, label := range input.Labels {
		labels[strings.ToLower(strings.TrimSpace(label))] = true
	}
	candidates := []queryDetection{}
	known := map[string]bool{}
	for _, d := range all {
		if d.DetectionKey == "" || known[d.DetectionKey] || len(d.DetectionKey) > 200 || len(d.Label) > 200 || d.Label == "" || math.IsNaN(d.Confidence) || math.IsInf(d.Confidence, 0) || d.Confidence < 0 || d.Confidence > 1 {
			return nil, errors.New("AGENT_TOOL_DETECTION_RESULT_INVALID")
		}
		known[d.DetectionKey] = true
		if d.Confidence < input.MinConfidence || (len(labels) > 0 && !labels[strings.ToLower(d.Label)]) {
			continue
		}
		candidates = append(candidates, d)
	}
	if input.SelectedDetectionKeys == nil {
		return candidates, nil
	}
	chosen := map[string]bool{}
	for _, key := range *input.SelectedDetectionKeys {
		chosen[key] = true
	}
	filtered := []queryDetection{}
	for _, d := range candidates {
		if chosen[d.DetectionKey] {
			filtered = append(filtered, d)
			delete(chosen, d.DetectionKey)
		}
	}
	if len(chosen) > 0 {
		return nil, errors.New("AGENT_TOOL_DETECTION_KEY_INVALID")
	}
	return filtered, nil
}

func objectResultHref(pid int32, id string, input objectQueryInput, detections []queryDetection) string {
	query := url.Values{"projectId": {fmt.Sprint(pid)}, "runId": {id}}
	if input.SelectedDetectionKeys != nil || len(input.Labels) > 0 || input.MinConfidence > 0 {
		query.Set("filtered", "1")
		for _, d := range detections {
			query.Add("object", d.DetectionKey)
		}
		if input.SelectionReason != "" {
			query.Set("reason", input.SelectionReason)
		} else {
			query.Set("reason", fmt.Sprintf("类别 %s，置信度 ≥ %.2f", strings.Join(input.Labels, ","), input.MinConfidence))
		}
	}
	return projectPageURL(pid, "/projects/algorithms/runs/detail/", query)
}
func (s *Server) executeObjectQuery(ctx context.Context, uid, pid int32, raw json.RawMessage) (gin.H, error) {
	input, err := parseObjectQuery(raw)
	if err != nil {
		return nil, err
	}
	if _, err = s.projectAccess(ctx, s.queries, uid, pid, "project:view"); err != nil {
		return nil, errors.New("PROJECT_ACCESS_DENIED")
	}
	result := gin.H{"projectId": pid, "observedAt": timestamp(time.Now()), "quality": "recorded-detection-query", "truncated": false, "items": []gin.H{}}
	if input.AlgorithmRunID == "" {
		rows, err := s.queries.ListAlgorithmRuns(ctx, pid)
		if err != nil {
			return nil, err
		}
		items := []gin.H{}
		for _, raw := range rows {
			var row map[string]any
			if json.Unmarshal(raw, &row) != nil {
				continue
			}
			canonical := fhObject(row["canonicalResult"])
			// Pending runs may not have a kind yet; expose their real status for polling.
			if canonical["kind"] != "detection" && row["status"] == "succeeded" {
				continue
			}
			id := fmt.Sprint(row["id"])
			items = append(items, gin.H{"id": id, "status": row["status"], "inputAssetId": row["inputAssetId"], "definitionName": row["definitionName"], "providerName": row["providerName"], "createdAt": row["createdAt"], "reference": gin.H{"type": "algorithm-run", "id": id, "href": objectResultHref(pid, id, objectQueryInput{}, nil)}})
		}
		result["items"] = items
		result["truncated"] = len(rows) == 100
		result["summary"] = fmt.Sprintf("返回 %d 次算法运行；成功的检测运行可查询目标，未完成运行请核对状态。", len(items))
		return result, nil
	}
	id, _ := uuid.Parse(input.AlgorithmRunID)
	encoded, err := s.queries.ReadAlgorithmRunDetail(ctx, sqlcgen.ReadAlgorithmRunDetailParams{ProjectID: pid, ID: id})
	if errors.Is(err, sql.ErrNoRows) {
		return nil, errors.New("AGENT_TOOL_ALGORITHM_RUN_NOT_FOUND")
	}
	if err != nil {
		return nil, err
	}
	var run objectQueryRun
	if json.Unmarshal(encoded, &run) != nil {
		return nil, errors.New("AGENT_TOOL_RESPONSE_INVALID")
	}
	ref := gin.H{"type": "algorithm-run", "id": run.ID, "href": objectResultHref(pid, run.ID, objectQueryInput{}, nil)}
	if run.Status != "succeeded" {
		result["items"] = []gin.H{{"id": run.ID, "status": run.Status, "reference": ref}}
		result["summary"] = "算法运行状态为 " + run.Status + "，尚无可用成功检测结果。"
		return result, nil
	}
	if run.CanonicalResult.Kind != "detection" {
		return nil, errors.New("AGENT_TOOL_NOT_DETECTION_RESULT")
	}
	selected, err := filterQueryDetections(run.CanonicalResult.Result.Detections, input)
	if err != nil {
		return nil, err
	}
	count := len(selected)
	if count > 100 {
		selected = selected[:100]
		result["truncated"] = true
	}
	href := objectResultHref(pid, run.ID, input, selected)
	ref["href"] = href
	result["items"] = []gin.H{{"id": run.ID, "version": run.InputSnapshot.InputAsset.Version, "assetId": run.InputAssetID, "status": run.Status, "totalDetections": len(run.CanonicalResult.Result.Detections), "matchedCount": count, "detections": selected, "selectionReason": input.SelectionReason, "reference": ref}}
	result["summary"] = fmt.Sprintf("在 %d 个检测候选中匹配 %d 个目标，返回 %d 个；仅代表本次候选查询，预测需复核。", len(run.CanonicalResult.Result.Detections), count, len(selected))
	if input.SelectionReason != "" {
		result["summary"] = fmt.Sprint(result["summary"]) + " 筛选依据：" + input.SelectionReason
	}
	if input.IncludeImage != nil && !*input.IncludeImage {
		result["visualStatus"] = "not_requested"
		return result, nil
	}
	body, err := s.readObjectRunImage(ctx, uid, pid, run)
	if err != nil {
		result["visualStatus"] = "unavailable"
		result["visualNotice"] = "原图权限、版本、校验和或读取不通过；不得判断颜色或场景属性。"
		result["summary"] = fmt.Sprint(result["summary"]) + " 原图不可用于视觉复核。"
		return result, nil
	}
	photos, keys, err := objectQueryPhotos(body, selected)
	if err != nil {
		result["visualStatus"] = "unavailable"
		result["visualNotice"] = "原图无法解码；不得进行视觉属性判断。"
		return result, nil
	}
	result["visualStatus"] = "available"
	result["previewedDetectionKeys"] = keys
	result["unpreviewedCount"] = count - len(keys)
	result["visualNotice"] = "整图和候选裁剪发送至当前 AI Provider；bbox 为原图像素坐标，未裁剪候选不算完成属性复核。"
	result["_images"] = photos
	return result, nil
}

// Shares the browser and agent image authority: the recorded run's input version
// and digest, current media permission, and the same optional remote reader.
func (s *Server) readObjectRunImage(ctx context.Context, uid, pid int32, run objectQueryRun) ([]byte, error) {
	input := run.InputSnapshot.InputAsset
	if input.AssetID != run.InputAssetID || input.Version < 1 || len(input.ChecksumSHA256) != 64 {
		return nil, errors.New("AGENT_TOOL_IMAGE_VERSION_UNVERIFIABLE")
	}
	asset, _, err := s.readMediaAsset(ctx, uid, pid, input.AssetID, "preview")
	if err != nil {
		return nil, err
	}
	var valid bool
	err = s.db.QueryRowContext(ctx, `select exists(select 1 from assets where project_id=$1 and id=$2 and version=$3 and status='available' and deleted_at is null and kind='image' and coalesce(checksum_sha256,checksum,'')=$4)`, pid, input.AssetID, input.Version, input.ChecksumSHA256).Scan(&valid)
	if err != nil || !valid {
		return nil, errors.New("AGENT_TOOL_IMAGE_VERSION_CHANGED")
	}
	var body []byte
	handled := false
	if s.inspectionMedia != nil {
		var remote algorithm.AlgorithmAsset
		remote, handled, err = s.inspectionMedia(ctx, int(pid), int(input.AssetID), input.Version)
		body = remote.Body
	}
	if err != nil {
		return nil, err
	}
	if !handled {
		var isRemote bool
		if err = s.db.QueryRowContext(ctx, `select exists(select 1 from connector_asset_access_refs where project_id=$1 and id=$2)`, pid, input.AssetID).Scan(&isRemote); err != nil || isRemote {
			return nil, errors.New("AGENT_TOOL_IMAGE_READER_UNAVAILABLE")
		}
		file, err := media.OpenStoredProjectObject(ctx, s.mediaObjectStorage, s.mediaStorageRoot, pid, asset.StorageKey)
		if err != nil {
			return nil, err
		}
		defer file.Close()
		body, err = io.ReadAll(io.LimitReader(file, (20<<20)+1))
		if err != nil {
			return nil, err
		}
	}
	if len(body) > 20<<20 {
		return nil, errors.New("AGENT_TOOL_IMAGE_TOO_LARGE")
	}
	digest := sha256.Sum256(body)
	if hex.EncodeToString(digest[:]) != input.ChecksumSHA256 {
		return nil, errors.New("AGENT_TOOL_IMAGE_VERSION_CHANGED")
	}
	return body, nil
}
func objectQueryPhotos(body []byte, detections []queryDetection) ([]agentPhoto, []string, error) {
	body, err := normalizeObjectImage(body)
	if err != nil {
		return nil, nil, err
	}
	whole, err := agentPhotoPreview(body)
	if err != nil {
		return nil, nil, err
	}
	config, _, err := image.DecodeConfig(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	whole.caption = fmt.Sprintf("整图预览；原图尺寸 %d×%d；检测框坐标均属于原图。", config.Width, config.Height)
	source, _, err := image.Decode(bytes.NewReader(body))
	if err != nil {
		return nil, nil, err
	}
	photos := []agentPhoto{whole}
	keys := []string{}
	for _, d := range detections {
		if len(keys) >= 8 {
			break
		}
		b := d.PixelGeometry
		if b == nil || b.Type != "bbox" || !finiteQueryBox(*b) {
			continue
		}
		rect := image.Rect(int(math.Floor(b.X)), int(math.Floor(b.Y)), int(math.Ceil(b.X+b.Width)), int(math.Ceil(b.Y+b.Height))).Intersect(source.Bounds())
		if rect.Empty() {
			continue
		}
		cropped := image.NewRGBA(image.Rect(0, 0, rect.Dx(), rect.Dy()))
		draw.Draw(cropped, cropped.Bounds(), source, rect.Min, draw.Src)
		var buf bytes.Buffer
		if jpeg.Encode(&buf, cropped, &jpeg.Options{Quality: 85}) != nil {
			continue
		}
		photo, err := agentPhotoPreview(buf.Bytes())
		if err != nil {
			continue
		}
		photo.caption = fmt.Sprintf("候选目标 detectionKey=%s，检测标签=%s；下面图片只对应这个 ID。", d.DetectionKey, d.Label)
		photos = append(photos, photo)
		keys = append(keys, d.DetectionKey)
	}
	return photos, keys, nil
}
func finiteQueryBox(b queryBox) bool {
	for _, n := range []float64{b.X, b.Y, b.Width, b.Height} {
		if math.IsNaN(n) || math.IsInf(n, 0) || math.Abs(n) > 1e8 {
			return false
		}
	}
	return b.Width > 0 && b.Height > 0
}
func (s *Server) readObjectQueryImage(c *gin.Context) {
	pid, err := projectID(c)
	id, e := uuid.Parse(c.Param("runId"))
	if err != nil || e != nil {
		s.failure(c, 404, "ALGORITHM_RUN_NOT_FOUND")
		return
	}
	uid := currentUser(c).ID
	if _, err = s.projectAccess(c.Request.Context(), s.queries, uid, pid, "project:view"); err != nil {
		s.failure(c, 403, "PROJECT_ACCESS_DENIED")
		return
	}
	raw, err := s.queries.ReadAlgorithmRunDetail(c.Request.Context(), sqlcgen.ReadAlgorithmRunDetailParams{ProjectID: pid, ID: id})
	var run objectQueryRun
	if err != nil || json.Unmarshal(raw, &run) != nil || run.Status != "succeeded" {
		s.failure(c, 404, "ALGORITHM_RUN_NOT_FOUND")
		return
	}
	body, err := s.readObjectRunImage(c.Request.Context(), uid, pid, run)
	if err != nil {
		s.failure(c, 409, "ALGORITHM_INPUT_IMAGE_UNAVAILABLE")
		return
	}
	body, err = normalizeObjectImage(body)
	if err != nil {
		s.failure(c, 415, "ALGORITHM_INPUT_IMAGE_UNSUPPORTED")
		return
	}
	if _, err = agentPhotoPreview(body); err != nil {
		s.failure(c, 415, "ALGORITHM_INPUT_IMAGE_UNSUPPORTED")
		return
	}
	c.Header("Cache-Control", "private, no-store")
	c.Header("X-Content-Type-Options", "nosniff")
	c.Data(200, "image/"+imageFormat(body), body)
}
func imageFormat(body []byte) string {
	_, format, _ := image.DecodeConfig(bytes.NewReader(body))
	return format
}
