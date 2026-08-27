package algorithm

import (
	"context"
	"errors"
	"net/url"
	"strings"
	"sync"
	"testing"
	"time"
)

type endpointValidatorFixture struct {
	err   error
	calls int
}

func (fixture *endpointValidatorFixture) ValidateEndpoint(context.Context, *url.URL) error {
	fixture.calls++
	return fixture.err
}

type secretResolverFixture struct{ value string }

func (fixture secretResolverFixture) ResolveSecret(context.Context, string) (string, error) {
	return fixture.value, nil
}

type executorFixture struct {
	mu       sync.Mutex
	requests []Request
	active   int
	maximum  int
	release  chan struct{}
}

func (fixture *executorFixture) Execute(ctx context.Context, request Request) (Outcome, error) {
	fixture.mu.Lock()
	fixture.requests = append(fixture.requests, request)
	fixture.active++
	if fixture.active > fixture.maximum {
		fixture.maximum = fixture.active
	}
	fixture.mu.Unlock()
	if fixture.release != nil {
		select {
		case <-fixture.release:
		case <-ctx.Done():
			return Outcome{}, ctx.Err()
		}
	}
	fixture.mu.Lock()
	fixture.active--
	fixture.mu.Unlock()
	return Outcome{Kind: "completed"}, nil
}

func providerConfigFixture() ProviderConfig {
	return ProviderConfig{ID: 3, ProjectID: 17, AdapterType: "http-json", BaseURL: "https://models.example.test/v1/run",
		SecretRef: "secret://projects/17/providers/3", AuthType: "bearer", AllowedHeaders: []string{"X-Trace-ID"},
		Timeout: 30 * time.Second, ConcurrencyLimit: 1, Status: "active"}
}

func providerRequestFixture() Request {
	return Request{Endpoint: "https://attacker.invalid/override", Headers: map[string]string{"X-Trace-ID": "trace-1"}, Input: Input{ProjectID: 17}}
}

func TestProviderConfigRejectsSSRFAndInlineSecrets(t *testing.T) {
	for _, endpoint := range []string{"http://models.example.test", "https://127.0.0.1/run", "https://10.0.0.2/run", "https://metadata.google.internal/latest", "https://user:pass@models.example.test"} {
		config := providerConfigFixture()
		config.BaseURL = endpoint
		if err := ValidateProviderConfig(config); err == nil {
			t.Fatalf("unsafe provider endpoint was accepted: %s", endpoint)
		}
	}
	config := providerConfigFixture()
	config.SecretRef = "plaintext-api-key"
	if err := ValidateProviderConfig(config); err == nil {
		t.Fatal("inline algorithm provider secret was accepted")
	}
}

func TestProviderRuntimePinsEndpointFiltersHeadersAndRedactsSecret(t *testing.T) {
	config := providerConfigFixture()
	executor := &executorFixture{}
	validator := &endpointValidatorFixture{}
	runtime, err := NewProviderRuntime(config, executor, secretResolverFixture{value: "runtime-only-secret"}, validator)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.Execute(context.Background(), providerRequestFixture()); err != nil {
		t.Fatal(err)
	}
	request := executor.requests[0]
	if request.Endpoint != config.BaseURL || request.Timeout != config.Timeout || request.Headers["Authorization"] != "Bearer runtime-only-secret" || validator.calls != 1 {
		t.Fatalf("provider runtime did not enforce stored transport policy: %+v", request)
	}
	public := RedactProvider(config)
	if !public.SecretConfigured || strings.Contains(strings.Join(public.AllowedHeaders, ","), config.SecretRef) {
		t.Fatalf("provider public projection leaked or lost secret state: %+v", public)
	}
	bad := providerRequestFixture()
	bad.Headers["X-Untrusted"] = "value"
	if _, err := runtime.Execute(context.Background(), bad); err == nil {
		t.Fatal("non-allowlisted provider header was accepted")
	}
}

func TestProviderRuntimeEnforcesConcurrencyAndContextCancellation(t *testing.T) {
	config := providerConfigFixture()
	executor := &executorFixture{release: make(chan struct{})}
	runtime, err := NewProviderRuntime(config, executor, secretResolverFixture{value: "secret"}, &endpointValidatorFixture{})
	if err != nil {
		t.Fatal(err)
	}
	firstDone := make(chan error, 1)
	go func() { _, err := runtime.Execute(context.Background(), providerRequestFixture()); firstDone <- err }()
	deadline := time.Now().Add(time.Second)
	for {
		executor.mu.Lock()
		active := executor.active
		executor.mu.Unlock()
		if active == 1 || time.Now().After(deadline) {
			break
		}
		time.Sleep(time.Millisecond)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	if _, err := runtime.Execute(ctx, providerRequestFixture()); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("queued provider execution ignored context cancellation: %v", err)
	}
	close(executor.release)
	if err := <-firstDone; err != nil {
		t.Fatal(err)
	}
	if executor.maximum != 1 {
		t.Fatalf("provider concurrency exceeded configured limit: %d", executor.maximum)
	}
}

func TestProviderRuntimeRejectsProjectScopeAndEndpointPolicyFailure(t *testing.T) {
	config := providerConfigFixture()
	validator := &endpointValidatorFixture{err: errors.New("resolved private address")}
	runtime, _ := NewProviderRuntime(config, &executorFixture{}, secretResolverFixture{value: "secret"}, validator)
	request := providerRequestFixture()
	request.Input.ProjectID = 18
	if _, err := runtime.Execute(context.Background(), request); err == nil {
		t.Fatal("cross-project algorithm provider execution was accepted")
	}
	request.Input.ProjectID = 17
	if _, err := runtime.Execute(context.Background(), request); err == nil || !strings.Contains(err.Error(), "endpoint policy") {
		t.Fatalf("runtime endpoint validation failure was ignored: %v", err)
	}
}
