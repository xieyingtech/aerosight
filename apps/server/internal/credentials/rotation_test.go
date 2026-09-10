package credentials

import (
	"testing"
)

func TestReencryptEnvelopeUsesNewKeyAndPreservesPayload(t *testing.T) {
	oldSecret := "0123456789abcdef0123456789abcdef"
	newSecret := "fedcba9876543210fedcba9876543210"
	aad := AAD("ai-provider", 4, nil)
	original, err := EncryptJSON(map[string]string{"apiKey": "secret"}, oldSecret, aad)
	if err != nil {
		t.Fatal(err)
	}
	rotated, err := ReencryptEnvelope(original, oldSecret, newSecret, aad)
	if err != nil {
		t.Fatal(err)
	}
	if rotated.KeyFingerprint == original.KeyFingerprint || rotated.Nonce == original.Nonce {
		t.Fatal("rotation must use the new key and nonce")
	}
	if err := ValidateEnvelope(rotated, oldSecret, aad); err == nil {
		t.Fatal("old secret unexpectedly decrypted rotated credentials")
	}
	var decoded map[string]string
	if err := DecryptJSON(rotated, newSecret, aad, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded["apiKey"] != "secret" {
		t.Fatalf("unexpected payload: %#v", decoded)
	}
}

func TestReencryptEnvelopeFailsClosedForDamagedInput(t *testing.T) {
	secret := "0123456789abcdef0123456789abcdef"
	aad := AAD("device-adapter", 9, 3)
	envelope, err := EncryptJSON(map[string]string{"password": "secret"}, secret, aad)
	if err != nil {
		t.Fatal(err)
	}
	envelope.Ciphertext = "damaged"
	if _, err := ReencryptEnvelope(envelope, secret, "fedcba9876543210fedcba9876543210", aad); err == nil {
		t.Fatal("damaged envelope unexpectedly rotated")
	}
}
