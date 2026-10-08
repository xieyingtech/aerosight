package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"
)

func TestVideoImportAndOfflineFrameProvenance(t *testing.T) {
	f := newAPIFixture(t)
	_, pid := f.project(t)
	f.server.AttachDeviceCredentials("0123456789abcdef0123456789abcdef")
	f.server.AttachMediaStorage(t.TempDir())
	videoPath := filepath.Join(t.TempDir(), "video.mp4")
	if _, err := exec.LookPath("ffmpeg"); err != nil {
		t.Skip("FFmpeg required for actual video import test")
	}
	if output, err := exec.Command("ffmpeg", "-hide_banner", "-loglevel", "error", "-f", "lavfi", "-i", "testsrc2=size=160x90:rate=5", "-t", "3", "-an", "-c:v", "libx264", "-pix_fmt", "yuv420p", "-movflags", "+faststart", videoPath).CombinedOutput(); err != nil {
		t.Fatalf("generate video: %s %v", output, err)
	}
	video, err := os.ReadFile(videoPath)
	if err != nil {
		t.Fatal(err)
	}
	upload := func(pid int, name string, body []byte, fields map[string]string, want int) map[string]any {
		t.Helper()
		var buffer bytes.Buffer
		form := multipart.NewWriter(&buffer)
		part, _ := form.CreateFormFile("file", name)
		part.Write(body)
		for k, v := range fields {
			form.WriteField(k, v)
		}
		form.Close()
		request, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/projects/%d/assets/import", f.host.URL, pid), &buffer)
		request.Header.Set("Content-Type", form.FormDataContentType())
		request.Header.Set("Origin", "http://frontend.test")
		request.Header.Set("X-CSRF-Token", f.csrf)
		response, err := f.client.Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		var result map[string]any
		json.NewDecoder(response.Body).Decode(&result)
		if response.StatusCode != want {
			t.Fatalf("upload %d %+v", response.StatusCode, result)
		}
		return result
	}
	result := upload(pid, "video.mp4", video, map[string]string{"capturedAt": "2026-08-18T07:15:00Z", "sourceDescription": "manual video demo"}, 201)
	if result["kind"] != "video" || result["width"] != float64(160) || result["durationSeconds"] != float64(3) {
		t.Fatalf("video metadata %+v", result)
	}
	aid := int(result["assetId"].(float64))
	access := decodedResponse(t, f.request(t, "GET", fmt.Sprintf("/api/projects/%d/assets/%d/access?action=play", pid, aid), ""))
	request, _ := http.NewRequest("GET", f.host.URL+access["url"].(string), nil)
	request.Header.Set("Range", "bytes=0-31")
	response, err := f.client.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(response.Body)
	response.Body.Close()
	if response.StatusCode != 206 || len(body) != 32 || !bytes.Equal(body, video[:32]) {
		t.Fatalf("video range %d %d", response.StatusCode, len(body))
	}
	var frame bytes.Buffer
	png.Encode(&frame, image.NewRGBA(image.Rect(0, 0, 16, 9)))
	frozen := upload(pid, "frame.png", frame.Bytes(), map[string]string{"videoAssetId": fmt.Sprint(aid), "mediaTimeSeconds": "1.5"}, 201)
	var captured time.Time
	var metadata []byte
	if err = f.db.QueryRow(`select captured_at,metadata_json from assets where id=$1`, frozen["assetId"]).Scan(&captured, &metadata); err != nil {
		t.Fatal(err)
	}
	var source map[string]any
	json.Unmarshal(metadata, &source)
	expected, _ := time.Parse(time.RFC3339Nano, "2026-08-18T07:15:01.5Z")
	if !captured.Equal(expected) || source["videoAssetId"] != float64(aid) || source["mediaTimeSeconds"] != 1.5 || source["source"] != "video-frame" {
		t.Fatalf("frame source %v %s", captured, metadata)
	}
	_, other := f.project(t)
	upload(other, "frame.png", frame.Bytes(), map[string]string{"videoAssetId": fmt.Sprint(aid), "mediaTimeSeconds": "1.5"}, 422)
	upload(pid, "frame.png", frame.Bytes(), map[string]string{"videoAssetId": fmt.Sprint(aid), "mediaTimeSeconds": "999"}, 422)
	upload(pid, "frame.png", frame.Bytes(), map[string]string{"streamId": "999999"}, 422)
}
