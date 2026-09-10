package agent

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestEvidenceVersionHashIsOrderIndependent(t *testing.T) {
	first := evidenceVersionHash([]evidenceRef{{Type: "asset", ID: "7", Version: "2"}, {Type: "detection", ID: "3", Version: "canonical-v1"}})
	second := evidenceVersionHash([]evidenceRef{{Type: "detection", ID: "3", Version: "canonical-v1"}, {Type: "asset", ID: "7", Version: "2"}})
	if first == "" || first != second {
		t.Fatal("evidence provenance hash must be deterministic")
	}
}

func TestCopilotCompletionUsesConfiguredProviderWithoutLeakingCredential(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Header.Get("authorization") != "Bearer test-key" {
			t.Fatal("configured provider credential was not used")
		}
		response.Header().Set("content-type", "application/json")
		_, _ = response.Write([]byte(`{"choices":[{"message":{"content":"  需要人工复核原始影像。  "}}]}`))
	}))
	defer server.Close()
	processor := JobProcessor{HTTPClient: server.Client()}
	result, err := processor.complete(context.Background(), providerConfig{BaseURL: server.URL, ModelID: "fixture", APIKey: "test-key"}, "evidence only")
	if err != nil || result != "需要人工复核原始影像。" {
		t.Fatalf("unexpected model completion: %q, %v", result, err)
	}
}

func TestCopilotFailureCodesExposeNoProviderResponse(t *testing.T) {
	if failureCodeFor(assertedError("MODEL_REQUEST_FAILED")) != "MODEL_REQUEST_FAILED" {
		t.Fatal("known failure code was not preserved")
	}
	if failureCodeFor(assertedError("provider said secret-key=abc")) != "COPILOT_EXECUTION_FAILED" {
		t.Fatal("untrusted provider error escaped into the persisted failure code")
	}
}

type assertedError string

func (value assertedError) Error() string { return string(value) }
