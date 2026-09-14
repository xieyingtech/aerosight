package algorithm

import (
	"net/url"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestAssetURLIsShortLivedTamperEvidentAndVersionScoped(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	signer := NewAssetURLSigner(strings.Repeat("s", 32), "https://worker.example.test/")
	signer.now = func() time.Time { return now }
	issued, err := signer.IssueAssetURL(2, 41, 7, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	parsed, _ := url.Parse(issued)
	expires, _ := strconv.ParseInt(parsed.Query().Get("expires"), 10, 64)
	if !signer.Verify(2, 41, 7, expires, parsed.Query().Get("signature")) {
		t.Fatal("valid asset URL rejected")
	}
	if signer.Verify(2, 41, 8, expires, parsed.Query().Get("signature")) {
		t.Fatal("asset version tampering accepted")
	}
	signer.now = func() time.Time { return now.Add(6 * time.Minute) }
	if signer.Verify(2, 41, 7, expires, parsed.Query().Get("signature")) {
		t.Fatal("expired asset URL accepted")
	}
}

func TestPinnedAssetURLCannotDowngradeOrChangeDigest(t *testing.T) {
	now := time.Unix(1_800_000_000, 0)
	signer := NewAssetURLSigner(strings.Repeat("s", 32), "https://worker.example")
	signer.now = func() time.Time { return now }
	checksum := strings.Repeat("a", 64)
	raw, err := signer.IssuePinnedAssetURL(2, 41, 7, checksum, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	link, _ := url.Parse(raw)
	values := link.Query()
	expires, _ := strconv.ParseInt(values.Get("expires"), 10, 64)
	signature := values.Get("signature")
	if !signer.VerifyPinned(2, 41, 7, expires, checksum, signature) {
		t.Fatal("valid pinned link rejected")
	}
	if signer.Verify(2, 41, 7, expires, signature) {
		t.Fatal("removing checksum downgraded pinned token")
	}
	if signer.VerifyPinned(2, 41, 7, expires, strings.Repeat("b", 64), signature) {
		t.Fatal("checksum tampering accepted")
	}
	if signer.VerifyPinned(3, 41, 7, expires, checksum, signature) {
		t.Fatal("project tampering accepted")
	}
	legacy, err := signer.IssueAssetURL(2, 41, 7, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	legacyLink, _ := url.Parse(legacy)
	if signer.VerifyPinned(2, 41, 7, expires, checksum, legacyLink.Query().Get("signature")) {
		t.Fatal("legacy token promoted to pinned")
	}
	if _, err := signer.IssuePinnedAssetURL(2, 41, 7, "invalid", now.Add(time.Minute)); err == nil {
		t.Fatal("invalid digest issued")
	}
}
