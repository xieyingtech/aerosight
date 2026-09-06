package dji

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strconv"
	"sync"
	"time"
)

var (
	ErrInvalidSignature = errors.New("DJI_CALLBACK_SIGNATURE_INVALID")
	ErrExpiredCallback  = errors.New("DJI_CALLBACK_TIMESTAMP_INVALID")
	ErrReplayedCallback = errors.New("DJI_CALLBACK_REPLAYED")
)

type NonceStore interface {
	Use(nonce string, now time.Time, expiresAt time.Time) bool
}

type MemoryNonceStore struct {
	mu     sync.Mutex
	nonces map[string]time.Time
}

func NewMemoryNonceStore() *MemoryNonceStore {
	return &MemoryNonceStore{nonces: map[string]time.Time{}}
}

func (store *MemoryNonceStore) Use(nonce string, now time.Time, expiresAt time.Time) bool {
	store.mu.Lock()
	defer store.mu.Unlock()
	for key, expiry := range store.nonces {
		if !expiry.After(now) {
			delete(store.nonces, key)
		}
	}
	if _, exists := store.nonces[nonce]; exists {
		return false
	}
	store.nonces[nonce] = expiresAt
	return true
}

func SignCallback(secret []byte, timestamp, nonce string, body []byte) string {
	mac := hmac.New(sha256.New, secret)
	mac.Write([]byte(timestamp))
	mac.Write([]byte("\n"))
	mac.Write([]byte(nonce))
	mac.Write([]byte("\n"))
	mac.Write(body)
	return hex.EncodeToString(mac.Sum(nil))
}

func VerifyCallback(secret []byte, timestamp, nonce, signature string, body []byte, now time.Time, store NonceStore) error {
	seconds, err := strconv.ParseInt(timestamp, 10, 64)
	if err != nil || nonce == "" || now.Sub(time.Unix(seconds, 0)).Abs() > 5*time.Minute {
		return ErrExpiredCallback
	}
	provided, err := hex.DecodeString(signature)
	if err != nil {
		return ErrInvalidSignature
	}
	expected, _ := hex.DecodeString(SignCallback(secret, timestamp, nonce, body))
	if !hmac.Equal(provided, expected) {
		return ErrInvalidSignature
	}
	if store == nil || !store.Use(nonce, now, now.Add(10*time.Minute)) {
		return ErrReplayedCallback
	}
	return nil
}
