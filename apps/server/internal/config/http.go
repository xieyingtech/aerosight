package config

import (
	"encoding/base64"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"
	"time"
)

type HTTP struct {
	Address, PublicOrigin                                         string
	CSRFKey                                                       []byte
	Development                                                   bool
	TrustedProxies                                                []string
	HTTPPool, WorkerPool                                          int
	SessionLifetime, SessionIdle, RequestTimeout, ShutdownTimeout time.Duration
	LoginLimit, WriteLimit                                        int
	SSELimit                                                      int
	MetricsToken                                                  string
	AlgorithmAllowedHosts                                         []string
	MediaAdminUser, MediaAdminPassword                            string
	AIRequestTimeout                                              time.Duration
}

func LoadHTTP(get func(string) string) (HTTP, error) {
	cfg := HTTP{Address: get("HTTP_LISTEN_ADDRESS"), PublicOrigin: strings.TrimRight(get("PUBLIC_ORIGIN"), "/"), Development: get("AEROSIGHT_ENV") == "development", HTTPPool: 20, WorkerPool: 10, SessionLifetime: 7 * 24 * time.Hour, SessionIdle: 24 * time.Hour, RequestTimeout: 30 * time.Second, ShutdownTimeout: 30 * time.Second, LoginLimit: 10, WriteLimit: 120, MetricsToken: get("METRICS_TOKEN")}
	cfg.AIRequestTimeout = 120 * time.Second
	cfg.SSELimit = 30
	if cfg.Address == "" {
		cfg.Address = "127.0.0.1:8080"
	}
	if _, _, err := net.SplitHostPort(cfg.Address); err != nil {
		return cfg, fmt.Errorf("HTTP_LISTEN_ADDRESS must be host:port")
	}
	if cfg.PublicOrigin == "" {
		if cfg.Development {
			cfg.PublicOrigin = "http://localhost:3000"
		} else {
			return cfg, fmt.Errorf("PUBLIC_ORIGIN is required")
		}
	}
	origin, err := url.Parse(cfg.PublicOrigin)
	if err != nil || origin.Host == "" || origin.User != nil || (origin.Scheme != "https" && origin.Scheme != "http") || origin.Path != "" || origin.RawQuery != "" || origin.Fragment != "" {
		return cfg, fmt.Errorf("PUBLIC_ORIGIN must be an HTTP(S) origin")
	}
	if !cfg.Development && origin.Scheme != "https" {
		return cfg, fmt.Errorf("production PUBLIC_ORIGIN must use HTTPS")
	}
	cfg.CSRFKey, err = base64.StdEncoding.DecodeString(get("CSRF_AUTH_KEY"))
	if err != nil || len(cfg.CSRFKey) != 32 {
		return cfg, fmt.Errorf("CSRF_AUTH_KEY must encode 32 bytes in base64")
	}
	for key, dest := range map[string]*int{"HTTP_DB_MAX_CONNECTIONS": &cfg.HTTPPool, "WORKER_DB_MAX_CONNECTIONS": &cfg.WorkerPool, "LOGIN_RATE_LIMIT": &cfg.LoginLimit, "WRITE_RATE_LIMIT": &cfg.WriteLimit, "SSE_RATE_LIMIT": &cfg.SSELimit} {
		if raw := get(key); raw != "" {
			v, err := strconv.Atoi(raw)
			if err != nil || v < 1 || v > 10000 {
				return cfg, fmt.Errorf("%s must be between 1 and 10000", key)
			}
			*dest = v
		}
	}
	for key, dest := range map[string]*time.Duration{"SESSION_LIFETIME": &cfg.SessionLifetime, "SESSION_IDLE_TIMEOUT": &cfg.SessionIdle, "HTTP_REQUEST_TIMEOUT": &cfg.RequestTimeout, "AI_REQUEST_TIMEOUT": &cfg.AIRequestTimeout, "SHUTDOWN_TIMEOUT": &cfg.ShutdownTimeout} {
		if raw := get(key); raw != "" {
			v, err := time.ParseDuration(raw)
			if err != nil || v <= 0 {
				return cfg, fmt.Errorf("%s must be a positive duration", key)
			}
			*dest = v
		}
	}
	if raw := strings.TrimSpace(get("TRUSTED_PROXIES")); raw != "" {
		cfg.TrustedProxies = strings.Split(raw, ",")
		for i, p := range cfg.TrustedProxies {
			p = strings.TrimSpace(p)
			cfg.TrustedProxies[i] = p
			if net.ParseIP(p) == nil {
				if _, _, err = net.ParseCIDR(p); err != nil {
					return cfg, fmt.Errorf("TRUSTED_PROXIES contains an invalid IP/CIDR")
				}
			}
		}
	}
	for _, host := range strings.Split(get("ALGORITHM_ALLOWED_HOSTS"), ",") {
		if host = strings.TrimSpace(host); host != "" {
			cfg.AlgorithmAllowedHosts = append(cfg.AlgorithmAllowedHosts, host)
		}
	}
	cfg.MediaAdminUser = get("MEDIA_ADMIN_USER")
	cfg.MediaAdminPassword = get("MEDIA_ADMIN_PASSWORD")
	return cfg, nil
}
