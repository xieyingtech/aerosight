package httpapi

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"image"
	"image/png"
	"io"
	"mime/multipart"
	"net/http"
	"testing"
)

func TestImageImportFreezesBytesAndChecksScope(t *testing.T) {
	f := newAPIFixture(t)
	team, pid := f.project(t)
	f.server.AttachMediaStorage(t.TempDir())
	var img bytes.Buffer
	if err := png.Encode(&img, image.NewRGBA(image.Rect(0, 0, 3, 2))); err != nil {
		t.Fatal(err)
	}
	// A valid image with trailing bytes exercises the upload-specific >2 MiB limit.
	data := append(img.Bytes(), make([]byte, 3<<20)...)
	upload := func(project int, body []byte, want int) map[string]any {
		t.Helper()
		var buf bytes.Buffer
		form := multipart.NewWriter(&buf)
		part, _ := form.CreateFormFile("file", "sample.png")
		part.Write(body)
		form.Close()
		r, _ := http.NewRequest("POST", fmt.Sprintf("%s/api/projects/%d/assets/import", f.host.URL, project), &buf)
		r.Header.Set("Content-Type", form.FormDataContentType())
		r.Header.Set("Origin", "http://frontend.test")
		r.Header.Set("X-CSRF-Token", f.csrf)
		res, err := f.client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		defer res.Body.Close()
		raw, _ := io.ReadAll(res.Body)
		if res.StatusCode != want {
			t.Fatalf("status %d: %s", res.StatusCode, raw)
		}
		var out map[string]any
		json.Unmarshal(raw, &out)
		return out
	}
	result := upload(pid, data, 201)
	sum := sha256.Sum256(data)
	if result["checksumSha256"] != hex.EncodeToString(sum[:]) {
		t.Fatal("checksum differs")
	}
	var run any
	var count int
	if err := f.db.QueryRow("select task_run_id from assets where id=$1", result["assetId"]).Scan(&run); err != nil || run != nil {
		t.Fatalf("unexpected Run %v %v", run, err)
	}
	upload(pid, []byte("not an image"), 422)
	if err := f.db.QueryRow("select count(*) from assets where project_id=$1", pid).Scan(&count); err != nil || count != 1 {
		t.Fatal("invalid upload persisted")
	}
	if _, err := f.db.Exec("update team_members set role='member' where team_id=$1", team); err != nil {
		t.Fatal(err)
	}
	upload(pid, data, 403)
	upload(pid+999, data, 403)
}
