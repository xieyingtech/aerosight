package algorithm

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/url"
	"regexp"
	"strings"
	"time"
)

type ProviderConfig struct {
	ID               int64
	ProjectID        int
	AdapterType      string
	BaseURL          string
	SecretRef        string
	AuthType         string
	AllowedHeaders   []string
	Timeout          time.Duration
	ConcurrencyLimit int
	Status           string
}

type PublicProvider struct {
	ID               int64
	ProjectID        int
	AdapterType      string
	BaseURL          string
	AuthType         string
	AllowedHeaders   []string
	Timeout          time.Duration
	ConcurrencyLimit int
	Status           string
	SecretConfigured bool
}

type SecretResolver interface {
	ResolveSecret(context.Context, string) (string, error)
}

type EndpointValidator interface {
	ValidateEndpoint(context.Context, *url.URL) error
}

type ProviderExecutor interface {
	Execute(context.Context, Request) (Outcome, error)
}

var secretReferencePattern = regexp.MustCompile(`^secret://[A-Za-z0-9._/-]+$`)

func ValidateProviderConfig(config ProviderConfig) error {
	if config.ID <= 0 || config.ProjectID <= 0 || config.AdapterType == "" || config.BaseURL == "" {
		return errors.New("algorithm provider identity, project, adapter type, and endpoint are required")
	}
	target, err := url.Parse(config.BaseURL)
	if err != nil || target.Scheme != "https" || target.Hostname() == "" || target.User != nil {
		return errors.New("algorithm provider endpoint must be an HTTPS URL without embedded credentials")
	}
	if restrictedProviderHost(target.Hostname()) {
		return errors.New("algorithm provider endpoint host is restricted")
	}
	switch config.AuthType {
	case "none":
		if config.SecretRef != "" {
			return errors.New("unauthenticated provider cannot retain a secret reference")
		}
	case "bearer", "api-key-header", "basic", "signed":
		if !secretReferencePattern.MatchString(config.SecretRef) {
			return errors.New("authenticated provider requires a secret reference")
		}
	default:
		return errors.New("unsupported algorithm provider authentication type")
	}
	if config.Timeout < time.Second || config.Timeout > time.Hour || config.ConcurrencyLimit < 1 || config.ConcurrencyLimit > 1000 {
		return errors.New("algorithm provider timeout or concurrency limit is invalid")
	}
	seen := map[string]struct{}{}
	for _, header := range config.AllowedHeaders {
		header = strings.ToLower(strings.TrimSpace(header))
		if header == "" || header == "authorization" || header == "proxy-authorization" || header == "host" || header == "cookie" {
			return fmt.Errorf("algorithm provider header %q cannot be allowlisted", header)
		}
		if _, exists := seen[header]; exists {
			return fmt.Errorf("algorithm provider header %q is duplicated", header)
		}
		seen[header] = struct{}{}
	}
	return nil
}

func restrictedProviderHost(host string) bool {
	host = strings.TrimSuffix(strings.ToLower(host), ".")
	if host == "localhost" || strings.HasSuffix(host, ".localhost") || host == "metadata.google.internal" {
		return true
	}
	address := net.ParseIP(host)
	return address != nil && (address.IsLoopback() || address.IsPrivate() || address.IsLinkLocalUnicast() || address.IsUnspecified())
}

func RedactProvider(config ProviderConfig) PublicProvider {
	return PublicProvider{
		ID: config.ID, ProjectID: config.ProjectID, AdapterType: config.AdapterType, BaseURL: config.BaseURL,
		AuthType: config.AuthType, AllowedHeaders: append([]string(nil), config.AllowedHeaders...), Timeout: config.Timeout,
		ConcurrencyLimit: config.ConcurrencyLimit, Status: config.Status, SecretConfigured: config.SecretRef != "",
	}
}

type ProviderRuntime struct {
	config    ProviderConfig
	executor  ProviderExecutor
	secrets   SecretResolver
	endpoints EndpointValidator
	semaphore chan struct{}
	allowed   map[string]struct{}
}

func NewProviderRuntime(config ProviderConfig, executor ProviderExecutor, secrets SecretResolver, endpoints EndpointValidator) (*ProviderRuntime, error) {
	if err := ValidateProviderConfig(config); err != nil {
		return nil, err
	}
	if executor == nil || endpoints == nil || (config.AuthType != "none" && secrets == nil) {
		return nil, errors.New("algorithm provider runtime dependencies are incomplete")
	}
	allowed := make(map[string]struct{}, len(config.AllowedHeaders))
	for _, header := range config.AllowedHeaders {
		allowed[strings.ToLower(header)] = struct{}{}
	}
	return &ProviderRuntime{config: config, executor: executor, secrets: secrets, endpoints: endpoints,
		semaphore: make(chan struct{}, config.ConcurrencyLimit), allowed: allowed}, nil
}

func (runtime *ProviderRuntime) Execute(ctx context.Context, request Request) (Outcome, error) {
	if request.Input.ProjectID != runtime.config.ProjectID {
		return Outcome{}, errors.New("algorithm provider project scope mismatch")
	}
	if runtime.config.Status != "active" && runtime.config.Status != "testing" {
		return Outcome{}, errors.New("algorithm provider is not enabled")
	}
	target, _ := url.Parse(runtime.config.BaseURL)
	if err := runtime.endpoints.ValidateEndpoint(ctx, target); err != nil {
		return Outcome{}, fmt.Errorf("algorithm provider endpoint policy: %w", err)
	}
	request.Endpoint = runtime.config.BaseURL
	request.Timeout = runtime.config.Timeout
	for header := range request.Headers {
		if _, allowed := runtime.allowed[strings.ToLower(header)]; !allowed {
			return Outcome{}, fmt.Errorf("algorithm provider header %q is not allowlisted", header)
		}
	}
	if request.Headers == nil {
		request.Headers = map[string]string{}
	}
	if runtime.config.AuthType != "none" {
		secret, err := runtime.secrets.ResolveSecret(ctx, runtime.config.SecretRef)
		if err != nil {
			return Outcome{}, errors.New("resolve algorithm provider secret")
		}
		if secret == "" {
			return Outcome{}, errors.New("algorithm provider secret is empty")
		}
		switch runtime.config.AuthType {
		case "bearer":
			request.Headers["Authorization"] = "Bearer " + secret
		case "api-key-header":
			request.Headers["X-API-Key"] = secret
		default:
			return Outcome{}, errors.New("algorithm provider authentication adapter is not implemented")
		}
	}
	select {
	case runtime.semaphore <- struct{}{}:
		defer func() { <-runtime.semaphore }()
	case <-ctx.Done():
		return Outcome{}, ctx.Err()
	}
	return runtime.executor.Execute(ctx, request)
}
