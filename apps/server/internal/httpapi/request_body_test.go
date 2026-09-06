package httpapi

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

func TestRequestBodyLimitsAndSlowRead(t *testing.T) {
	s := boundaryServer(t, io.Discard)
	s.cfg.RequestTimeout = 100 * time.Millisecond
	s.router.POST("/api/body-limit-test", func(c *gin.Context) {
		_, err := io.ReadAll(c.Request.Body)
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			s.failure(c, 413, "BODY_TOO_LARGE")
			return
		}
		if err != nil {
			s.failure(c, 400, "INVALID_BODY")
			return
		}
		c.Status(204)
	})
	ts := httptest.NewServer(s.Handler())
	defer ts.Close()
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 3 * time.Second}
	res, err := client.Get(ts.URL + "/api/auth/csrf")
	if err != nil {
		t.Fatal(err)
	}
	var payload struct {
		Token string `json:"csrfToken"`
	}
	if err = json.NewDecoder(res.Body).Decode(&payload); err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	for _, length := range []int{2 << 20, (2 << 20) + 1} {
		r, _ := http.NewRequest("POST", ts.URL+"/api/body-limit-test", strings.NewReader(strings.Repeat("a", length)))
		r.Header.Set("X-CSRF-Token", payload.Token)
		r.Header.Set("Origin", s.cfg.PublicOrigin)
		res, err = client.Do(r)
		if err != nil {
			t.Fatal(err)
		}
		res.Body.Close()
		want := 204
		if length > 2<<20 {
			want = 413
		}
		if res.StatusCode != want {
			t.Fatalf("length %d: %d want %d", length, res.StatusCode, want)
		}
	}
	u, _ := url.Parse(ts.URL)
	var cookies []string
	for _, cookie := range jar.Cookies(u) {
		cookies = append(cookies, cookie.Name+"="+cookie.Value)
	}
	// A missing header token makes gorilla/csrf inspect the form before Gin.
	form := &countedBody{Reader: strings.NewReader("field=" + strings.Repeat("a", (2<<20)+4096))}
	formRequest := httptest.NewRequest("POST", "/api/body-limit-test", form)
	formRequest.Header.Set("Cookie", strings.Join(cookies, "; "))
	formRequest.Header.Set("Origin", s.cfg.PublicOrigin)
	formRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	formResponse := httptest.NewRecorder()
	s.Handler().ServeHTTP(formResponse, formRequest)
	if formResponse.Code != 403 || form.read > (2<<20)+1 {
		t.Fatalf("CSRF parsed unbounded form: status=%d bytes=%d", formResponse.Code, form.read)
	}
	conn, err := net.Dial("tcp", u.Host)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(2 * time.Second))
	start := time.Now()
	fmt.Fprintf(conn, "POST /api/body-limit-test HTTP/1.1\r\nHost: %s\r\nOrigin: %s\r\nCookie: %s\r\nX-CSRF-Token: %s\r\nContent-Length: 10\r\n\r\na", u.Host, s.cfg.PublicOrigin, strings.Join(cookies, "; "), payload.Token)
	res, err = http.ReadResponse(bufio.NewReader(conn), nil)
	if err != nil {
		t.Fatal(err)
	}
	res.Body.Close()
	if res.StatusCode != 400 || time.Since(start) > time.Second {
		t.Fatalf("slow body not rejected promptly: %d", res.StatusCode)
	}
}

type countedBody struct {
	io.Reader
	read int
}

func (b *countedBody) Read(p []byte) (int, error) {
	n, err := b.Reader.Read(p)
	b.read += n
	return n, err
}
