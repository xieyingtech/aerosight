package flighthub

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"strconv"
	"strings"

	"aerosight/server/internal/inspection"
)

// InspectionMediaStatus compares the full file list with the provider's full
// file counters. Callers must not pass only the selected image count as listed.
func InspectionMediaStatus(task FlightTask, listed, accessible int, truncated bool) inspection.MediaStatus {
	status := inspection.MediaStatus{FlightSucceeded: task.Status == "success", CountsComparable: task.FolderInfo.CountsKnown, Listed: listed, Accessible: accessible, Truncated: truncated}
	if task.FolderInfo.CountsKnown {
		expected, uploaded := task.FolderInfo.ExpectedFileCount, task.FolderInfo.UploadedFileCount
		status.Expected = &expected
		status.Uploaded = &uploaded
	}
	return status
}

// HashInspectionMedia reads a freshly authorized link without forwarding API
// credentials. The client shares the FlightHub allowlist and redirect policy.
// Hashing is streaming and bounded so a large media response cannot exhaust RAM.
func (client *Client) HashInspectionMedia(ctx context.Context, download TemporaryDownload, maxBytes int64) (string, error) {
	hash := sha256.New()
	if err := client.copyInspectionMedia(ctx, download, maxBytes, hash); err != nil {
		return "", err
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

// ReadInspectionMedia returns bounded bytes for the algorithm gateway. It uses
// the exact same link validation and credential isolation as observation hashing.
func (client *Client) ReadInspectionMedia(ctx context.Context, download TemporaryDownload, maxBytes int64) ([]byte, error) {
	var buffer bytes.Buffer
	if err := client.copyInspectionMedia(ctx, download, maxBytes, &buffer); err != nil {
		return nil, err
	}
	return buffer.Bytes(), nil
}

func (client *Client) copyInspectionMedia(ctx context.Context, download TemporaryDownload, maxBytes int64, destination io.Writer) error {
	if maxBytes < 1 || maxBytes > 64<<20 {
		return &APIError{SafeCode: "request_invalid"}
	}
	link, err := client.ValidateTemporaryLink(LinkDownload, download.URL, download.ExpiresAt)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, link.String(), nil)
	if err != nil {
		return &APIError{SafeCode: "temporary_link_invalid"}
	}
	response, err := client.httpClient.Do(req)
	if err != nil {
		return &APIError{SafeCode: "media_read_failed", Retryable: true}
	}
	defer response.Body.Close()
	if response.StatusCode == http.StatusUnauthorized || response.StatusCode == http.StatusForbidden {
		return &APIError{SafeCode: "temporary_link_expired", Retryable: true}
	}
	if response.StatusCode != http.StatusOK {
		return &APIError{SafeCode: "media_read_failed", Retryable: true}
	}
	if response.ContentLength > maxBytes {
		return &APIError{SafeCode: "media_size_limit_exceeded"}
	}
	n, err := io.Copy(destination, io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return &APIError{SafeCode: "media_read_failed", Retryable: true}
	}
	if n > maxBytes {
		return &APIError{SafeCode: "media_size_limit_exceeded"}
	}
	if n == 0 {
		return &APIError{SafeCode: "media_empty"}
	}
	return nil
}

func inspectionMediaVersion(item FlightTaskMedia) string {
	return secureRemoteKey(strings.Join([]string{item.UpdatedAt, strconv.FormatInt(item.SizeBytes, 10), item.FileType, item.Suffix}, ":"))
}

// The version is compared with the freshly listed file, not just the local
// catalogue. A URL refresh must not silently switch an observation's content.
func (client *Client) RefreshInspectionMediaURL(ctx context.Context, token, projectUUID, taskUUID, mediaUUID, version string) (TemporaryDownload, error) {
	if version == "" {
		return TemporaryDownload{}, &APIError{SafeCode: "request_invalid"}
	}
	items, err := client.ListFlightTaskMedia(ctx, token, projectUUID, taskUUID)
	if err != nil {
		return TemporaryDownload{}, err
	}
	for _, item := range items {
		if item.UUID != mediaUUID {
			continue
		}
		if inspectionMediaVersion(item) != version {
			return TemporaryDownload{}, &APIError{SafeCode: "media_version_changed"}
		}
		return client.validateDownload(item.OriginalURL, 0)
	}
	return TemporaryDownload{}, &APIError{SafeCode: "scope_not_found"}
}
