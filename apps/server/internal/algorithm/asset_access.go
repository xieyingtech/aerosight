package algorithm

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type AlgorithmAsset struct {
	Body        []byte
	ContentType string
}

type AlgorithmAssetStore interface {
	ReadAlgorithmAsset(context.Context, string) (AlgorithmAsset, error)
}

type AssetURLSigner struct {
	secret  []byte
	baseURL string
	now     func() time.Time
}

func NewAssetURLSigner(secret, baseURL string) *AssetURLSigner {
	return &AssetURLSigner{secret: []byte(secret), baseURL: strings.TrimRight(baseURL, "/"), now: time.Now}
}

func (signer *AssetURLSigner) IssueAssetURL(projectID, assetID, version int, expiresAt time.Time) (string, error) {
	if len(signer.secret) < 32 || !strings.HasPrefix(signer.baseURL, "https://") {
		return "", errors.New("algorithm asset URL signing is unavailable")
	}
	values := url.Values{
		"projectId": {strconv.Itoa(projectID)}, "version": {strconv.Itoa(version)},
		"expires": {strconv.FormatInt(expiresAt.Unix(), 10)},
	}
	values.Set("signature", signer.signature(projectID, assetID, version, expiresAt.Unix()))
	return fmt.Sprintf("%s/algorithm-assets/%d?%s", signer.baseURL, assetID, values.Encode()), nil
}

func (signer *AssetURLSigner) Verify(projectID, assetID, version int, expires int64, signature string) bool {
	if expires <= signer.now().Unix() || expires > signer.now().Add(10*time.Minute).Unix() {
		return false
	}
	expected, err := hex.DecodeString(signer.signature(projectID, assetID, version, expires))
	if err != nil {
		return false
	}
	provided, err := hex.DecodeString(signature)
	return err == nil && hmac.Equal(expected, provided)
}

func (signer *AssetURLSigner) signature(projectID, assetID, version int, expires int64) string {
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = fmt.Fprintf(mac, "%d.%d.%d.%d", projectID, assetID, version, expires)
	return hex.EncodeToString(mac.Sum(nil))
}

type AssetAccessHandler struct {
	db     *sql.DB
	store  AlgorithmAssetStore
	signer *AssetURLSigner
}

func NewAssetAccessHandler(db *sql.DB, store AlgorithmAssetStore, signer *AssetURLSigner) *AssetAccessHandler {
	return &AssetAccessHandler{db: db, store: store, signer: signer}
}

func (handler *AssetAccessHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet {
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	assetID, err := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/algorithm-assets/"))
	projectID, projectErr := strconv.Atoi(request.URL.Query().Get("projectId"))
	version, versionErr := strconv.Atoi(request.URL.Query().Get("version"))
	expires, expiresErr := strconv.ParseInt(request.URL.Query().Get("expires"), 10, 64)
	if err != nil || projectErr != nil || versionErr != nil || expiresErr != nil || handler.signer == nil ||
		!handler.signer.Verify(projectID, assetID, version, expires, request.URL.Query().Get("signature")) {
		http.Error(writer, "asset access denied", http.StatusForbidden)
		return
	}
	if handler.store == nil {
		http.Error(writer, "asset unavailable", http.StatusServiceUnavailable)
		return
	}
	var storageKey, contentType string
	err = handler.db.QueryRowContext(request.Context(), `
		select storage_key, mime_type from assets
		where id=$1 and project_id=$2 and version=$3 and status='available'`, assetID, projectID, version).Scan(&storageKey, &contentType)
	if err != nil {
		http.Error(writer, "asset unavailable", http.StatusNotFound)
		return
	}
	asset, err := handler.store.ReadAlgorithmAsset(request.Context(), storageKey)
	if err != nil {
		http.Error(writer, "asset unavailable", http.StatusNotFound)
		return
	}
	if asset.ContentType != "" {
		contentType = asset.ContentType
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.WriteHeader(http.StatusOK)
	_, _ = writer.Write(asset.Body)
}
