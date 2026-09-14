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
	AlgorithmDevelopmentEndpoint                                  string
	MediaAdminUser, MediaAdminPassword                            string
	AIRequestTimeout                                              time.Duration
	CSPMapOrigins, CSPMediaOrigins                                []string
}

func LoadHTTP(get func(string) string) (HTTP, error) {
	cfg := HTTP{PublicOrigin: strings.TrimRight(get("PUBLIC_ORIGIN"), "/"), Development: get("AEROSIGHT_ENV") == "development", HTTPPool: 20, WorkerPool: 10, SessionLifetime: 7 * 24 * time.Hour, SessionIdle: 24 * time.Hour, RequestTimeout: 30 * time.Second, ShutdownTimeout: 30 * time.Second, LoginLimit: 10, WriteLimit: 120, MetricsToken: get("METRICS_TOKEN")}
	cfg.AIRequestTimeout = 120 * time.Second
	cfg.SSELimit = 30
	host, port := get("HOST"), get("PORT")
	if host == "" {
		host = "127.0.0.1"
	}
	if port == "" {
		port = "8080"
	}
	if strings.ContainsAny(host, "/[] \t\r\n") || (strings.Contains(host, ":") && net.ParseIP(host) == nil) {
		return cfg, fmt.Errorf("HOST must be a hostname or an unbracketed IP address")
	}
	portNumber, portErr := strconv.Atoi(port)
	if portErr != nil || portNumber < 0 || portNumber > 65535 || strings.Trim(port, "0123456789") != "" {
		return cfg, fmt.Errorf("PORT must be an integer between 0 and 65535")
	}
	cfg.Address = net.JoinHostPort(host, port)
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
	cfg.CSRFKey, err = base64.StdEncoding.DecodeString(get("CSRF_SECRET"))
	if err != nil || len(cfg.CSRFKey) != 32 {
		return cfg, fmt.Errorf("CSRF_SECRET must encode 32 bytes in base64")
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
	cfg.AlgorithmDevelopmentEndpoint = get("ALGORITHM_DEVELOPMENT_ENDPOINT")
	if cfg.AlgorithmDevelopmentEndpoint != "" {
		u, e := url.Parse(cfg.AlgorithmDevelopmentEndpoint)
		if !cfg.Development || e != nil || u.Scheme != "https" || u.Hostname() != "127.0.0.1" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Port() == "" {
			return cfg, fmt.Errorf("ALGORITHM_DEVELOPMENT_ENDPOINT requires development mode and an exact HTTPS 127.0.0.1 endpoint with port")
		}
	}
	cfg.MediaAdminUser = get("MEDIA_ADMIN_USER")
	cfg.MediaAdminPassword = get("MEDIA_ADMIN_PASSWORD")
	mapOrigins := get("CSP_MAP_ORIGINS")
	if mapOrigins == "" {
		mapOrigins = "https://tiles.openfreemap.org,https://demotiles.maplibre.org"
	}
	if cfg.CSPMapOrigins, err = cspOrigins(mapOrigins, cfg.Development); err != nil {
		return cfg, fmt.Errorf("CSP_MAP_ORIGINS: %w", err)
	}
	if cfg.CSPMediaOrigins, err = cspOrigins(get("CSP_MEDIA_ORIGINS"), cfg.Development); err != nil {
		return cfg, fmt.Errorf("CSP_MEDIA_ORIGINS: %w", err)
	}
	return cfg, nil
}

func cspOrigins(raw string, development bool) ([]string, error) {
	result := []string{}
	seen := map[string]bool{}
	for _, value := range strings.Split(raw, ",") {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		u, err := url.Parse(value)
		if err != nil || u.Hostname() == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || (u.Path != "" && u.Path != "/") ||
			(u.Scheme != "https" && !(development && u.Scheme == "http")) || strings.ContainsAny(value, "*;'\"<>\r\n\t ") {
			return nil, fmt.Errorf("must contain comma-separated HTTPS origins (HTTP allowed in development)")
		}
		origin := u.Scheme + "://" + u.Host
		if !seen[origin] {
			result = append(result, origin)
			seen[origin] = true
		}
	}
	return result, nil
}
