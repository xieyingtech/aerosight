package algorithm

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"aerosight/server/internal/httptransport"
	"github.com/google/uuid"
)

func (s *AssetURLSigner) frameSignature(project int, run string, index int, checksum string, expires int64) string {
	mac := hmac.New(sha256.New, s.secret)
	fmt.Fprintf(mac, "frame\n%d\n%s\n%d\n%s\n%d", project, run, index, checksum, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

func (s *AssetURLSigner) IssueFrameURL(project int, run string, index int, checksum string, expires time.Time) (string, error) {
	if len(s.secret) < 32 || !strings.HasPrefix(s.baseURL, "https://") || !validFrame(project, run, index, checksum) {
		return "", errors.New("algorithm frame signing is unavailable")
	}
	query := url.Values{"projectId": {strconv.Itoa(project)}, "checksum": {checksum}, "expires": {strconv.FormatInt(expires.Unix(), 10)}}
	query.Set("signature", s.frameSignature(project, run, index, checksum, expires.Unix()))
	return fmt.Sprintf("%s/algorithm-assets/frames/%s/%d?%s", s.baseURL, run, index, query.Encode()), nil
}

func validFrame(project int, run string, index int, checksum string) bool {
	id, err := uuid.Parse(run)
	digest, hashErr := hex.DecodeString(checksum)
	return project > 0 && project <= 2147483647 && err == nil && id.String() == run && index >= 0 && index < MaxVideoFrames && hashErr == nil && len(digest) == 32
}

func (h *AssetAccessHandler) serveFrame(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	parts := strings.Split(strings.TrimPrefix(r.URL.Path, "/algorithm-assets/frames/"), "/")
	if len(parts) != 2 || h.signer == nil {
		http.Error(w, "frame access denied", http.StatusForbidden)
		return
	}
	project, _ := strconv.Atoi(r.URL.Query().Get("projectId"))
	index, indexErr := strconv.Atoi(parts[1])
	expires, _ := strconv.ParseInt(r.URL.Query().Get("expires"), 10, 64)
	checksum := r.URL.Query().Get("checksum")
	now := h.signer.now()
	if indexErr != nil || !validFrame(project, parts[0], index, checksum) || expires <= now.Unix() || expires > now.Add(10*time.Minute).Unix() || !hmac.Equal([]byte(r.URL.Query().Get("signature")), []byte(h.signer.frameSignature(project, parts[0], index, checksum, expires))) {
		http.Error(w, "frame access denied", http.StatusForbidden)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	var exists bool
	err := h.db.QueryRowContext(ctx, `select exists(select 1 from algorithm_runs run join assets asset on asset.id=run.input_asset_id and asset.project_id=run.project_id where run.id=$1 and run.project_id=$2 and asset.status='available' and run.input_snapshot_json#>'{context,videoAnalysis}' is not null)`, parts[0], project).Scan(&exists)
	if err != nil || !exists || h.store == nil {
		http.Error(w, "frame unavailable", http.StatusNotFound)
		return
	}
	key := fmt.Sprintf("projects/%d/algorithm-runs/%s/frames/%06d.jpg", project, parts[0], index)
	object, err := h.store.ReadAlgorithmAsset(ctx, key)
	digest := sha256.Sum256(object.Body)
	if err != nil || hex.EncodeToString(digest[:]) != checksum {
		http.Error(w, "frame unavailable", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "image/jpeg")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	httptransport.ServeContent(w, r, bytes.NewReader(object.Body))
}
