package http

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"aerosight/internal/auth"
	"aerosight/internal/store"
)

func TestProtectedRouteRequiresSession(t *testing.T) {
	router := NewRouter(&store.Store{}, auth.NewManager([]byte("01234567890123456789012345678901")))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/projects", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}

func TestTeamsRouteRequiresSession(t *testing.T) {
	router := NewRouter(&store.Store{}, auth.NewManager([]byte("01234567890123456789012345678901")))
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/api/teams", nil)

	router.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", recorder.Code)
	}
}
