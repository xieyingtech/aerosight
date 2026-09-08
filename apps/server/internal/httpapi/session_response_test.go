package httpapi

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/alexedwards/scs/v2"
)

type failedSessionCommit struct{ err error }

func (s failedSessionCommit) Find(string) ([]byte, bool, error)      { return nil, false, nil }
func (s failedSessionCommit) Delete(string) error                    { return nil }
func (s failedSessionCommit) Commit(string, []byte, time.Time) error { return s.err }

func TestSessionCommitFailureOwnsResponse(t *testing.T) {
	for _, tc := range []struct {
		name   string
		err    error
		status int
		code   string
	}{
		{"cancelled", context.Canceled, 400, "REQUEST_CANCELLED"},
		{"timeout", context.DeadlineExceeded, 504, "REQUEST_TIMEOUT"},
		{"storage", errors.New("private database detail"), 500, "SESSION_FAILED"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			sessions := scs.New()
			sessions.Store = failedSessionCommit{tc.err}
			sessions.ErrorFunc = func(w http.ResponseWriter, _ *http.Request, err error) { writeSessionFailure(w, err) }
			handler := sessionResponseBoundary(sessions.LoadAndSave(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				sessions.Put(r.Context(), "user", 1)
				w.WriteHeader(201)
				_, _ = w.Write([]byte(`{"content":"must not append"}`))
			})))
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest("POST", "/", nil))
			if recorder.Code != tc.status || strings.TrimSpace(recorder.Body.String()) != `{"error":"`+tc.code+`"}` {
				t.Fatalf("response %d %s", recorder.Code, recorder.Body.String())
			}
			if recorder.Header().Get("Set-Cookie") != "" {
				t.Fatal("failed commit issued a session cookie")
			}
		})
	}
}
