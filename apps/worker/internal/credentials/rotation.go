package credentials

import (
	"encoding/json"
	"errors"
)

func ValidateEnvelope(envelope Envelope, authSecret, aad string) error {
	plaintext, err := Decrypt(envelope, authSecret, aad)
	if plaintext != nil {
		clear(plaintext)
	}
	return err
}

func ReencryptEnvelope(envelope Envelope, oldAuthSecret, newAuthSecret, aad string) (Envelope, error) {
	plaintext, err := Decrypt(envelope, oldAuthSecret, aad)
	if err != nil {
		return Envelope{}, err
	}
	defer clear(plaintext)
	rotated, err := Encrypt(plaintext, newAuthSecret, aad)
	if err != nil {
		return Envelope{}, err
	}
	verified, err := Decrypt(rotated, newAuthSecret, aad)
	if verified != nil {
		clear(verified)
	}
	if err != nil {
		return Envelope{}, errors.New("CREDENTIAL_ROTATION_VERIFICATION_FAILED")
	}
	return rotated, nil
}

func ParseEnvelope(raw []byte) (Envelope, error) {
	var envelope Envelope
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Envelope{}, errors.New("CREDENTIAL_ENVELOPE_INVALID")
	}
	if envelope.Version != Version || envelope.Algorithm != Algorithm {
		return Envelope{}, errors.New("CREDENTIAL_ENVELOPE_VERSION_UNSUPPORTED")
	}
	return envelope, nil
}
