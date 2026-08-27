package dji

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestMediaMTXInspectorRequiresActualReadyTrack(t *testing.T) {
	ready := false
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		username, password, authenticated := request.BasicAuth()
		if !authenticated || username != "admin" || password != "secret" {
			response.WriteHeader(http.StatusUnauthorized)
			return
		}
		response.Header().Set("content-type", "application/json")
		if ready {
			_, _ = response.Write([]byte(`{"items":[{"name":"demo/aerosight/session","ready":true,"tracks":["H264"]}]}`))
		} else {
			_, _ = response.Write([]byte(`{"items":[{"name":"demo/aerosight/session","ready":false,"tracks":[]}]}`))
		}
	}))
	defer server.Close()
	inspector, err := NewMediaMTXInspector(server.URL, "admin", "secret", server.Client())
	if err != nil {
		t.Fatal(err)
	}
	status, err := inspector.Inspect(context.Background(), "demo/aerosight/session")
	if err != nil || status.Ready {
		t.Fatalf("session without received tracks became live: status=%+v err=%v", status, err)
	}
	ready = true
	status, err = inspector.Inspect(context.Background(), "demo/aerosight/session")
	if err != nil || !status.Ready || len(status.Tracks) != 1 {
		t.Fatalf("ready H264 input was not detected: status=%+v err=%v", status, err)
	}
}

func TestMediaMTXInspectorIntegration(t *testing.T) {
	baseURL := os.Getenv("AEROSIGHT_TEST_MEDIAMTX_API_URL")
	username := os.Getenv("AEROSIGHT_TEST_MEDIAMTX_API_USER")
	password := os.Getenv("AEROSIGHT_TEST_MEDIAMTX_API_PASSWORD")
	path := os.Getenv("AEROSIGHT_TEST_MEDIAMTX_READY_PATH")
	if baseURL == "" || username == "" || password == "" || path == "" {
		t.Skip("MediaMTX integration environment is not configured")
	}
	inspector, err := NewMediaMTXInspector(baseURL, username, password, nil)
	if err != nil {
		t.Fatal(err)
	}
	status, err := inspector.Inspect(context.Background(), path)
	if err != nil || !status.Ready || len(status.Tracks) == 0 {
		t.Fatalf("MediaMTX did not confirm a received media track: status=%+v err=%v", status, err)
	}
}
