package credentials

import (
	"encoding/hex"
	"reflect"
	"testing"
)

const testAuthSecret = "0123456789abcdef0123456789abcdef"

func TestCredentialEncryptionRoundTripAndScopeBinding(t *testing.T) {
	aad := AAD("device-adapter", 9, 3)
	envelope, err := EncryptJSON(map[string]string{"username": "pilot", "password": "secret"}, testAuthSecret, aad)
	if err != nil {
		t.Fatal(err)
	}
	var decoded map[string]string
	if err := DecryptJSON(envelope, testAuthSecret, aad, &decoded); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(decoded, map[string]string{"username": "pilot", "password": "secret"}) {
		t.Fatalf("unexpected credentials: %#v", decoded)
	}
	if err := DecryptJSON(envelope, testAuthSecret, AAD("device-adapter", 9, 4), &decoded); err == nil {
		t.Fatal("expected AAD mismatch to fail")
	}
}

func TestCredentialEncryptionMatchesWebVector(t *testing.T) {
	nonce, _ := hex.DecodeString("000102030405060708090a0b")
	aad := AAD("algorithm-provider", 17, 3)
	envelope, err := EncryptWithNonce([]byte(`{"password":"secret","username":"pilot"}`), testAuthSecret, aad, nonce)
	if err != nil {
		t.Fatal(err)
	}
	if envelope.Nonce != "AAECAwQFBgcICQoL" ||
		envelope.Ciphertext != "HXWy7eF_4k6LAqvbh-dL7wVmb05NgxRR52mnDf_6eMToeCL6qRQbcQ" ||
		envelope.AuthenticationTag != "WUHcFG7rJk8ARwdWzHUffQ" {
		t.Fatalf("shared vector changed: %#v", envelope)
	}
}
