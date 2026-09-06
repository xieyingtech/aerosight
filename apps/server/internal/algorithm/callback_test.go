package algorithm

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"testing"
	"time"
)

func callbackFixture() (CallbackAuthenticator, CallbackMetadata, CallbackRunAuth, []byte) {
	now := time.Unix(1_800_000_000, 0)
	token := "random-callback-token-with-at-least-32-characters"
	body := []byte(`{"providerId":7,"externalJobId":"job-1","status":"completed","result":{"results":[]}}`)
	hash := sha256.Sum256([]byte(token))
	metadata := CallbackMetadata{ProviderID: 7, CallbackID: "callback-1", Timestamp: now, Token: token}
	metadata.Signature = SignCallback(token, metadata.CallbackID, now, body)
	return CallbackAuthenticator{Now: func() time.Time { return now }, MaxSkew: 5 * time.Minute}, metadata,
		CallbackRunAuth{ProviderID: 7, CallbackTokenHash: hex.EncodeToString(hash[:])}, body
}

func TestSignedCallbackAuthenticatesRunProviderTokenAndBody(t *testing.T) {
	authenticator, metadata, run, body := callbackFixture()
	if err := authenticator.Verify(metadata, run, body); err != nil {
		t.Fatal(err)
	}
	for name, mutate := range map[string]func(*CallbackMetadata, *CallbackRunAuth, *[]byte){
		"provider": func(metadata *CallbackMetadata, _ *CallbackRunAuth, _ *[]byte) { metadata.ProviderID++ },
		"token": func(metadata *CallbackMetadata, _ *CallbackRunAuth, _ *[]byte) {
			metadata.Token = "forged-callback-token-with-at-least-32-characters"
		},
		"signature": func(metadata *CallbackMetadata, _ *CallbackRunAuth, _ *[]byte) {
			metadata.Signature = "sha256=" + string(make([]byte, 64))
		},
		"body": func(_ *CallbackMetadata, _ *CallbackRunAuth, body *[]byte) { *body = append(*body, ' ') },
		"timestamp": func(metadata *CallbackMetadata, _ *CallbackRunAuth, _ *[]byte) {
			metadata.Timestamp = metadata.Timestamp.Add(-10 * time.Minute)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changedMetadata, changedRun, changedBody := metadata, run, append([]byte(nil), body...)
			mutate(&changedMetadata, &changedRun, &changedBody)
			if !errors.Is(authenticator.Verify(changedMetadata, changedRun, changedBody), ErrCallbackAuthentication) {
				t.Fatal("forged callback was accepted")
			}
		})
	}
}

func TestCallbackRunStateMachineIsIdempotentAndRejectsOutOfOrderTerminal(t *testing.T) {
	next, changed, err := NextCallbackRunStatus("waiting_callback", "completed")
	if err != nil || next != "succeeded" || !changed {
		t.Fatalf("completion failed: %s %v %v", next, changed, err)
	}
	next, changed, err = NextCallbackRunStatus("succeeded", "completed")
	if err != nil || next != "succeeded" || changed {
		t.Fatalf("duplicate completion was not idempotent: %s %v %v", next, changed, err)
	}
	if _, _, err = NextCallbackRunStatus("succeeded", "failed"); !errors.Is(err, ErrCallbackOrder) {
		t.Fatalf("out-of-order failure accepted: %v", err)
	}
	if _, _, err = NextCallbackRunStatus("timed_out", "completed"); !errors.Is(err, ErrCallbackOrder) {
		t.Fatalf("late completion accepted: %v", err)
	}
}

func TestCallbackReplayOnlyAcceptsTheExactPreviouslyAppliedPayload(t *testing.T) {
	duplicate, err := classifyCallbackReplay("run-1", strings.Repeat("a", 64), "run-1", strings.Repeat("a", 64))
	if err != nil || !duplicate {
		t.Fatalf("exact callback retry was not idempotent: duplicate=%v err=%v", duplicate, err)
	}
	for _, replay := range []struct{ runID, hash string }{{"run-2", strings.Repeat("a", 64)}, {"run-1", strings.Repeat("b", 64)}} {
		if _, err := classifyCallbackReplay("run-1", strings.Repeat("a", 64), replay.runID, replay.hash); !errors.Is(err, ErrCallbackReplay) {
			t.Fatalf("callback id reuse was accepted: %+v err=%v", replay, err)
		}
	}
}

func TestCallbackCredentialsAreIssuedTransientlyAndOnlyTheirHashIsPersistable(t *testing.T) {
	input := validRequest("https://provider.example.test").Input
	input.Definition.ExecutionMode = "callback"
	prepared, tokenHash, err := issueCallbackCredentials(input, "https://aerosight.example.test/")
	if err != nil {
		t.Fatal(err)
	}
	if input.Callback != nil {
		t.Fatal("callback secret mutated the persisted input snapshot")
	}
	if prepared.Callback["url"] != "https://aerosight.example.test/callbacks/algorithms/"+input.RunID || len(prepared.Callback["token"]) < 32 {
		t.Fatalf("invalid transient callback credentials: %+v", prepared.Callback)
	}
	digest := sha256.Sum256([]byte(prepared.Callback["token"]))
	if tokenHash != hex.EncodeToString(digest[:]) || strings.Contains(tokenHash, prepared.Callback["token"]) {
		t.Fatal("callback token hash does not protect the token")
	}
}
