package credentials

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

const (
	Version   = 1
	Algorithm = "AES-256-GCM"
)

var (
	hkdfSalt = []byte("aerosight/credential-encryption/salt/v1")
	hkdfInfo = []byte("aerosight/credential-encryption/v1")
)

type Envelope struct {
	Version           int    `json:"version"`
	Algorithm         string `json:"algorithm"`
	KeyFingerprint    string `json:"keyFingerprint"`
	Nonce             string `json:"nonce"`
	Ciphertext        string `json:"ciphertext"`
	AuthenticationTag string `json:"authenticationTag"`
}

func deriveKey(authSecret string) ([]byte, error) {
	if authSecret == "" {
		return nil, errors.New("AUTH_SECRET_REQUIRED_FOR_CREDENTIALS")
	}
	extract := hmac.New(sha256.New, hkdfSalt)
	_, _ = extract.Write([]byte(authSecret))
	pseudorandomKey := extract.Sum(nil)
	expand := hmac.New(sha256.New, pseudorandomKey)
	_, _ = expand.Write(hkdfInfo)
	_, _ = expand.Write([]byte{1})
	return expand.Sum(nil)[:32], nil
}

func KeyFingerprint(authSecret string) (string, error) {
	key, err := deriveKey(authSecret)
	if err != nil {
		return "", err
	}
	digest := sha256.Sum256(key)
	return hex.EncodeToString(digest[:])[:16], nil
}

func AAD(resourceType string, resourceID any, scopeID any) string {
	scope := "platform"
	if scopeID != nil {
		scope = fmt.Sprint(scopeID)
	}
	return fmt.Sprintf("aerosight:%s:%s:%v", resourceType, scope, resourceID)
}

func Encrypt(plaintext []byte, authSecret, aad string) (Envelope, error) {
	nonce := make([]byte, 12)
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return Envelope{}, err
	}
	return EncryptWithNonce(plaintext, authSecret, aad, nonce)
}

func EncryptWithNonce(plaintext []byte, authSecret, aad string, nonce []byte) (Envelope, error) {
	key, err := deriveKey(authSecret)
	if err != nil {
		return Envelope{}, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return Envelope{}, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return Envelope{}, err
	}
	if len(nonce) != gcm.NonceSize() {
		return Envelope{}, errors.New("CREDENTIAL_NONCE_INVALID")
	}
	sealed := gcm.Seal(nil, nonce, plaintext, []byte(aad))
	tagOffset := len(sealed) - gcm.Overhead()
	fingerprint, _ := KeyFingerprint(authSecret)
	return Envelope{
		Version: Version, Algorithm: Algorithm, KeyFingerprint: fingerprint,
		Nonce:             base64.RawURLEncoding.EncodeToString(nonce),
		Ciphertext:        base64.RawURLEncoding.EncodeToString(sealed[:tagOffset]),
		AuthenticationTag: base64.RawURLEncoding.EncodeToString(sealed[tagOffset:]),
	}, nil
}

func Decrypt(envelope Envelope, authSecret, aad string) ([]byte, error) {
	if envelope.Version != Version || envelope.Algorithm != Algorithm {
		return nil, errors.New("CREDENTIAL_ENVELOPE_VERSION_UNSUPPORTED")
	}
	fingerprint, err := KeyFingerprint(authSecret)
	if err != nil {
		return nil, err
	}
	if envelope.KeyFingerprint != fingerprint {
		return nil, errors.New("CREDENTIAL_KEY_MISMATCH")
	}
	nonce, err := base64.RawURLEncoding.DecodeString(envelope.Nonce)
	if err != nil {
		return nil, errors.New("CREDENTIAL_ENVELOPE_NONCE_INVALID")
	}
	ciphertext, err := base64.RawURLEncoding.DecodeString(envelope.Ciphertext)
	if err != nil {
		return nil, errors.New("CREDENTIAL_ENVELOPE_CIPHERTEXT_INVALID")
	}
	tag, err := base64.RawURLEncoding.DecodeString(envelope.AuthenticationTag)
	if err != nil {
		return nil, errors.New("CREDENTIAL_ENVELOPE_AUTHENTICATION_TAG_INVALID")
	}
	key, _ := deriveKey(authSecret)
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	plaintext, err := gcm.Open(nil, nonce, append(ciphertext, tag...), []byte(aad))
	if err != nil {
		return nil, errors.New("CREDENTIAL_DECRYPTION_FAILED")
	}
	return plaintext, nil
}

func EncryptJSON(value any, authSecret, aad string) (Envelope, error) {
	plaintext, err := json.Marshal(value)
	if err != nil {
		return Envelope{}, err
	}
	return Encrypt(plaintext, authSecret, aad)
}

func DecryptJSON(envelope Envelope, authSecret, aad string, target any) error {
	plaintext, err := Decrypt(envelope, authSecret, aad)
	if err != nil {
		return err
	}
	defer clear(plaintext)
	if err := json.Unmarshal(plaintext, target); err != nil {
		return errors.New("CREDENTIAL_PAYLOAD_INVALID")
	}
	return nil
}
