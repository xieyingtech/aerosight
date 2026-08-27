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
