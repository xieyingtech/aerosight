package algorithm

import (
	"encoding/json"
	"testing"

	"aerosight/worker/internal/credentials"
)

func TestDecryptProviderHeadersUsesBoundEncryptedCredential(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	envelope, err := credentials.EncryptJSON(map[string]string{"token": "provider-token"}, secret,
		credentials.AAD("algorithm-provider", 7, 3))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	headers, err := decryptProviderHeaders(7, 3, "bearer", raw, secret)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "Bearer provider-token" {
		t.Fatalf("unexpected headers: %#v", headers)
	}
	if _, err := decryptProviderHeaders(7, 4, "bearer", raw, secret); err == nil {
		t.Fatal("credential decrypted in the wrong project scope")
	}
}

func TestDecryptProviderHeadersSupportsBasicAndNeverNeedsCredentialsForNone(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	envelope, err := credentials.EncryptJSON(map[string]string{"username": "service", "password": "secret"}, secret,
		credentials.AAD("algorithm-provider", 8, 3))
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	headers, err := decryptProviderHeaders(8, 3, "basic", raw, secret)
	if err != nil {
		t.Fatal(err)
	}
	if headers["Authorization"] != "Basic c2VydmljZTpzZWNyZXQ=" {
		t.Fatalf("unexpected headers: %#v", headers)
	}
	if headers, err := decryptProviderHeaders(8, 3, "none", nil, ""); err != nil || headers != nil {
		t.Fatalf("none auth unexpectedly required credentials: %#v %v", headers, err)
	}
}
