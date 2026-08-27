package observability

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHealthAndReadinessDistinguishOptionalAndCriticalFailures(t *testing.T) {
	checks := []DependencyCheck{
		{Name: "database", Critical: true, Check: func(_ context.Context) error { return nil }},
		{Name: "object_storage", Critical: false, Check: func(_ context.Context) error { return errors.New("disabled") }},
	}
	handler := NewHealthHandler(checks, time.Second)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"status":"degraded"`) || !strings.Contains(recorder.Body.String(), "OBJECT_STORAGE_UNAVAILABLE") {
		t.Fatalf("optional failure should be ready but degraded: %d %s", recorder.Code, recorder.Body.String())
	}

	handler = NewHealthHandler([]DependencyCheck{{Name: "database", Critical: true, Check: func(_ context.Context) error { return errors.New("offline") }}}, time.Second)
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/readyz", nil))
	if recorder.Code != http.StatusServiceUnavailable || !strings.Contains(recorder.Body.String(), `"ready":false`) {
		t.Fatalf("critical failure should fail readiness: %d %s", recorder.Code, recorder.Body.String())
	}
	recorder = httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/healthz", nil))
	if recorder.Code != http.StatusOK || !strings.Contains(recorder.Body.String(), `"live":true`) {
		t.Fatalf("dependency failure should not pretend the process is dead: %d %s", recorder.Code, recorder.Body.String())
	}
}
