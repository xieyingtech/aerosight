package algorithm

import (
	"aerosight/server/internal/database/sqlcgen"
	"aerosight/server/internal/httptransport"
	"bytes"
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

type RemoteAlgorithmAssetReader func(context.Context, int, int, int) (AlgorithmAsset, bool, error)

type AssetAccessHandler struct {
	remote RemoteAlgorithmAssetReader
	db     *sql.DB
	store  AlgorithmAssetStore
	signer *AssetURLSigner
}

func NewAssetAccessHandler(db *sql.DB, store AlgorithmAssetStore, signer *AssetURLSigner) *AssetAccessHandler {
	return &AssetAccessHandler{db: db, store: store, signer: signer}
}

func (handler *AssetAccessHandler) WithRemoteReader(reader RemoteAlgorithmAssetReader) *AssetAccessHandler {
	handler.remote = reader
	return handler
}

func (handler *AssetAccessHandler) ServeHTTP(writer http.ResponseWriter, request *http.Request) {
	if request.Method != http.MethodGet && request.Method != http.MethodHead {
		writer.Header().Set("Allow", "GET, HEAD")
		http.Error(writer, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	assetID, err := strconv.Atoi(strings.TrimPrefix(request.URL.Path, "/algorithm-assets/"))
	projectID, projectErr := strconv.Atoi(request.URL.Query().Get("projectId"))
	version, versionErr := strconv.Atoi(request.URL.Query().Get("version"))
	expires, expiresErr := strconv.ParseInt(request.URL.Query().Get("expires"), 10, 64)
	checksum := request.URL.Query().Get("checksum")
	verified := false
	if handler.signer != nil {
		if checksum != "" {
			verified = handler.signer.VerifyPinned(projectID, assetID, version, expires, checksum, request.URL.Query().Get("signature"))
		} else {
			verified = handler.signer.Verify(projectID, assetID, version, expires, request.URL.Query().Get("signature"))
		}
	}
	if err != nil || projectErr != nil || versionErr != nil || expiresErr != nil || assetID <= 0 || assetID > 2147483647 || projectID <= 0 || projectID > 2147483647 || version <= 0 || version > 2147483647 || !verified {
		http.Error(writer, "asset access denied", http.StatusForbidden)
		return
	}
	if handler.store == nil && handler.remote == nil {
		http.Error(writer, "asset unavailable", http.StatusServiceUnavailable)
		return
	}
	lookup, cancel := httptransport.OperationContext(request.Context())
	defer cancel()
	row, err := sqlcgen.New(handler.db).ReadAlgorithmAccessAsset(lookup, sqlcgen.ReadAlgorithmAccessAssetParams{ID: int32(assetID), ProjectID: int32(projectID), Version: int32(version)})
	if err != nil {
		if lookup.Err() == context.DeadlineExceeded {
			http.Error(writer, "asset lookup timed out", http.StatusGatewayTimeout)
			return
		}
		http.Error(writer, "asset unavailable", http.StatusNotFound)
		return
	}
	var asset AlgorithmAsset
	handled := false
	if handler.remote != nil {
		asset, handled, err = handler.remote(lookup, projectID, assetID, version)
	}
	if err == nil && !handled {
		if handler.store == nil {
			err = errors.New("asset storage unavailable")
		} else {
			asset, err = handler.store.ReadAlgorithmAsset(lookup, row.StorageKey)
		}
	}
	if err != nil {
		if lookup.Err() == context.DeadlineExceeded {
			http.Error(writer, "asset lookup timed out", http.StatusGatewayTimeout)
			return
		}
		http.Error(writer, "asset unavailable", http.StatusNotFound)
		return
	}
	if checksum != "" {
		digest := sha256.Sum256(asset.Body)
		if hex.EncodeToString(digest[:]) != checksum {
			http.Error(writer, "asset version content changed", http.StatusConflict)
			return
		}
	}
	cancel()
	contentType := row.MimeType.String
	if asset.ContentType != "" {
		contentType = asset.ContentType
	}
	writer.Header().Set("Content-Type", contentType)
	writer.Header().Set("Cache-Control", "private, no-store")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	httptransport.ServeContent(writer, request, bytes.NewReader(asset.Body))
}
