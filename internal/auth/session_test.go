package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSessionRoundTrip(t *testing.T) {
	manager := NewManager([]byte("01234567890123456789012345678901"))
	recorder := httptest.NewRecorder()

	if err := manager.Set(recorder, User{ID: 7, Name: "Admin", Role: "admin"}); err != nil {
		t.Fatalf("set session: %v", err)
	}

	request := httptest.NewRequest(http.MethodGet, "/", nil)
	for _, cookie := range recorder.Result().Cookies() {
		request.AddCookie(cookie)
	}

	user, err := manager.Read(request)
	if err != nil {
		t.Fatalf("read session: %v", err)
	}
	if user.ID != 7 || user.Role != "admin" {
		t.Fatalf("unexpected user: %#v", user)
	}
}

func TestSessionRejectsTampering(t *testing.T) {
	manager := NewManager([]byte("01234567890123456789012345678901"))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.AddCookie(&http.Cookie{Name: CookieName, Value: "bad.value"})

	if _, err := manager.Read(request); err == nil {
		t.Fatal("expected tampered session to fail")
	}
}
