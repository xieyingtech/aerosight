package algorithm

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestFrameAccessBindsProjectRunIndexAndChecksum(t *testing.T) {
	s := NewAssetURLSigner(strings.Repeat("s", 32), "https://aerosight.example")
	now := time.Unix(1900000000, 0)
	s.now = func() time.Time { return now }
	run := "00000000-0000-4000-8000-000000001001"
	checksum := strings.Repeat("a", 64)
	raw, err := s.IssueFrameURL(1, run, 3, checksum, now.Add(5*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	for _, change := range []func(*url.URL){
		func(u *url.URL) { q := u.Query(); q.Set("projectId", "2"); u.RawQuery = q.Encode() },
		func(u *url.URL) { u.Path = strings.Replace(u.Path, "/3", "/4", 1) },
		func(u *url.URL) { u.Path = strings.Replace(u.Path, run, "00000000-0000-4000-8000-000000001002", 1) },
		func(u *url.URL) { q := u.Query(); q.Set("checksum", strings.Repeat("b", 64)); u.RawQuery = q.Encode() },
		func(u *url.URL) { q := u.Query(); q.Set("expires", "1900000000"); u.RawQuery = q.Encode() },
	} {
		u, _ := url.Parse(raw)
		change(u)
		w := httptest.NewRecorder()
		NewAssetAccessHandler(nil, nil, s).ServeHTTP(w, httptest.NewRequest(http.MethodGet, u.String(), nil))
		if w.Code != http.StatusForbidden {
			t.Fatalf("tampered frame URL accepted: %s", u)
		}
	}
	for _, index := range []int{-1, MaxVideoFrames} {
		if _, err := s.IssueFrameURL(1, run, index, checksum, now.Add(time.Minute)); err == nil {
			t.Fatal("invalid frame index signed")
		}
	}
}
