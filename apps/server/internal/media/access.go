package media

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode/utf16"

	"golang.org/x/text/unicode/norm"
)

type Access struct {
	URL       string `json:"url"`
	ExpiresAt string `json:"expiresAt"`
}

func ValidAccessAction(action string) bool {
	return action == "preview" || action == "play" || action == "download"
}
func AccessAllowed(action, role string, permissions map[string]bool, sensitive bool) bool {
	return ValidAccessAction(action) && (action != "download" || !sensitive || role == "owner" || role == "admin" || permissions["issue:handle"] || permissions["event:handle"])
}
func accessSignature(secret string, pid, aid int32, action, expires string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	fmt.Fprintf(mac, "%d\n%d\n%s\n%s", pid, aid, action, expires)
	return hex.EncodeToString(mac.Sum(nil))
}
func IssueAccess(secret string, pid, aid int32, action string, now time.Time, ttl int) (Access, error) {
	if len(utf16.Encode([]rune(secret))) < 16 {
		return Access{}, errors.New("MEDIA_SIGNING_SECRET_TOO_SHORT")
	}
	if !ValidAccessAction(action) || pid <= 0 || aid <= 0 {
		return Access{}, errors.New("INVALID_MEDIA_ACCESS_ACTION")
	}
	if ttl < 1 || ttl > 300 {
		return Access{}, errors.New("INVALID_MEDIA_ACCESS_TTL")
	}
	until := now.Add(time.Duration(ttl) * time.Second)
	expires := strconv.FormatInt(until.UnixMilli(), 10)
	return Access{URL: fmt.Sprintf("/api/projects/%d/assets/%d/content?action=%s&expires=%s&signature=%s", pid, aid, action, expires, accessSignature(secret, pid, aid, action, expires)), ExpiresAt: until.UTC().Format("2006-01-02T15:04:05.000Z")}, nil
}
func VerifyAccess(secret string, pid, aid int32, action, expires, signature string, now time.Time) bool {
	if len(utf16.Encode([]rune(secret))) < 16 || !ValidAccessAction(action) || !regexp.MustCompile(`^\d+$`).MatchString(expires) {
		return false
	}
	expiration, err := strconv.ParseInt(expires, 10, 64)
	if err != nil || expiration <= now.UnixMilli() {
		return false
	}
	return hmac.Equal([]byte(signature), []byte(accessSignature(secret, pid, aid, action, expires)))
}
func SafeDownloadName(value *string, fallback string) string {
	name := fallback
	if value != nil {
		name = *value
	}
	name = regexp.MustCompile(`[^a-zA-Z0-9._-]+`).ReplaceAllString(norm.NFKC.String(name), "-")
	if len(name) > 128 {
		name = name[:128]
	}
	if name == "" {
		name = fallback
	}
	return name
}

// Open within the project root so both lexical traversal and symlink escapes
// are rejected by the OS-backed root API. Do not read the entire media file.
func OpenProjectObject(root string, pid int32, key string) (*os.File, error) {
	prefix := fmt.Sprintf("projects/%d/", pid)
	if root == "" || !strings.HasPrefix(key, prefix) || strings.ContainsAny(key, "\\:") || path.Clean(key) != key {
		return nil, errors.New("INVALID_ASSET_STORAGE_KEY")
	}
	storage, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer storage.Close()
	// Project directory aliases must not redirect one tenant into another.
	for _, directory := range []string{"projects", strings.TrimSuffix(prefix, "/")} {
		info, err := storage.Lstat(directory)
		if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
			return nil, errors.New("INVALID_ASSET_STORAGE_KEY")
		}
	}
	project, err := storage.OpenRoot(strings.TrimSuffix(prefix, "/"))
	if err != nil {
		return nil, err
	}
	defer project.Close()
	file, err := project.Open(strings.TrimPrefix(key, prefix))
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		file.Close()
		return nil, errors.New("INVALID_ASSET_STORAGE_KEY")
	}
	return file, nil
}
