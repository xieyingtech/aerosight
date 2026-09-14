package algorithm

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

type PinnedAssetAccessIssuer interface {
	IssuePinnedAssetURL(projectID, assetID, version int, checksum string, expiresAt time.Time) (string, error)
}

func validAssetDigest(value string) bool {
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == strings.ToLower(value)
}

func issueInputAssetURL(issuer AssetAccessIssuer, projectID, assetID, version int, checksum string, expires time.Time, pinned bool) (string, error) {
	if !pinned {
		return issuer.IssueAssetURL(projectID, assetID, version, expires)
	}
	secure, ok := issuer.(PinnedAssetAccessIssuer)
	if !ok {
		return "", errors.New("INSPECTION_PINNED_ASSET_ACCESS_UNAVAILABLE")
	}
	return secure.IssuePinnedAssetURL(projectID, assetID, version, checksum, expires)
}

func (signer *AssetURLSigner) pinnedSignature(projectID, assetID, version int, expires int64, checksum string) string {
	mac := hmac.New(sha256.New, signer.secret)
	_, _ = fmt.Fprintf(mac, "pinned.%d.%d.%d.%d.%s", projectID, assetID, version, expires, checksum)
	return hex.EncodeToString(mac.Sum(nil))
}

func (signer *AssetURLSigner) IssuePinnedAssetURL(projectID, assetID, version int, checksum string, expires time.Time) (string, error) {
	if !validAssetDigest(checksum) {
		return "", errors.New("INSPECTION_ASSET_CHECKSUM_INVALID")
	}
	raw, err := signer.IssueAssetURL(projectID, assetID, version, expires)
	if err != nil {
		return "", err
	}
	link, err := url.Parse(raw)
	if err != nil {
		return "", err
	}
	values := link.Query()
	values.Set("checksum", checksum)
	values.Set("signature", signer.pinnedSignature(projectID, assetID, version, expires.Unix(), checksum))
	link.RawQuery = values.Encode()
	return link.String(), nil
}

func (signer *AssetURLSigner) VerifyPinned(projectID, assetID, version int, expires int64, checksum, signature string) bool {
	if !validAssetDigest(checksum) || expires <= signer.now().Unix() || expires > signer.now().Add(10*time.Minute).Unix() {
		return false
	}
	expected, _ := hex.DecodeString(signer.pinnedSignature(projectID, assetID, version, expires, checksum))
	provided, err := hex.DecodeString(signature)
	return err == nil && hmac.Equal(expected, provided)
}
