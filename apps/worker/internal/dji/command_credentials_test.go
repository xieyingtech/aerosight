package dji

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"aerosight/worker/internal/credentials"
)

func TestLivePublishCredentialsAreInjectedOnlyAtDispatchAndRedactedForPersistence(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	envelope, err := credentials.EncryptJSON(map[string]string{
		"mediaPublishUser": "publisher", "mediaPublishPassword": "publish-secret",
	}, secret, credentials.AAD("device-adapter", 7, 3))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	parameters, err := injectLivePublishCredentials([]byte(`{"url_type":1,"url":"rtmp://media.example/demo/aerosight/opaque","video_id":"camera","video_quality":3}`),
		raw, secret, 7, 3, "rtmp://media.example:1935", "demo/aerosight/opaque")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(parameters), "user=publisher") || !strings.Contains(string(parameters), "pass=publish-secret") {
		t.Fatalf("credentials not injected: %s", parameters)
	}
	service, err := BuildServiceCommand("DOCK", "command", "business", "dock2", "stream.video.control", "start", parameters, testTime())
	if err != nil {
		t.Fatal(err)
	}
	redacted := redactLiveServicePayload(service.Payload)
	if strings.Contains(string(redacted), "publisher") || strings.Contains(string(redacted), "publish-secret") {
		t.Fatalf("persistent payload leaked credentials: %s", redacted)
	}
}

func testTime() time.Time { return time.Unix(1_700_000_000, 0).UTC() }
