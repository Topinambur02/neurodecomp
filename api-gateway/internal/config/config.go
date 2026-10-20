package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/rs/cors"
)

type Service struct {
	Host string `json:"host"`
	Port int    `json:"port"`
}

type RateLimit struct {
	RequestsPerSecond int `json:"requests_per_second"`
	Burst             int `json:"burst"`
}

type Upstream struct {
	DialTimeoutSeconds           int `json:"dial_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int `json:"response_header_timeout_seconds"`
	MaxConnectionsPerHost        int `json:"max_connections_per_host"`
	MaxIdleConnectionsPerHost    int `json:"max_idle_connections_per_host"`
}

type CORS struct {
	AllowedOrigins []string `json:"allowed_origins"`
	AllowedMethods []string `json:"allowed_methods"`
	AllowedHeaders []string `json:"allowed_headers"`
}

type Config struct {
	Port                int                `json:"port"`
	Host                string             `json:"host"`
	ExternalScheme      string             `json:"external_scheme"`
	Services            map[string]Service `json:"services"`
	RateLimit           RateLimit          `json:"rate_limit"`
	Upstream            Upstream           `json:"upstream"`
	CORS                CORS               `json:"cors"`
	WriteTimeoutSeconds int                `json:"write_timeout_seconds"`
	Auth                Auth               `json:"auth"`
}

func LoadConfig(path string) (cfg Config, err error) {
	cfg = Config{
		Port:           8080,
		Host:           "localhost",
		ExternalScheme: "http",
		Services:       map[string]Service{},
		RateLimit:      RateLimit{RequestsPerSecond: 10, Burst: 20},
		Upstream:       Upstream{DialTimeoutSeconds: 3, ResponseHeaderTimeoutSeconds: 10, MaxConnectionsPerHost: 100, MaxIdleConnectionsPerHost: 20},
		CORS: CORS{
			AllowedMethods: []string{http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete},
			AllowedHeaders: []string{"Content-Type", "Authorization"},
		},
		WriteTimeoutSeconds: 30,
	}
	configFile, err := os.Open(path)
	if err != nil {
		return Config{}, fmt.Errorf("open config %q: %w", path, err)
	}
	defer func() {
		if closeErr := configFile.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close config %q: %w", path, closeErr)
		}
	}()

	decoder := json.NewDecoder(configFile)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		if err == nil {
			err = fmt.Errorf("multiple JSON values")
		}
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}
	if err := cfg.applyEnvironment(); err != nil {
		return Config{}, fmt.Errorf("config environment: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

func (cfg Config) Validate() error {
	if err := cfg.Auth.Validate(); err != nil {
		return fmt.Errorf("auth: %w", err)
	}
	if !validHost(cfg.Host) {
		return fmt.Errorf("host must be an IP address or DNS name")
	}
	if !validPort(cfg.Port) {
		return fmt.Errorf("port must be between 1 and 65535")
	}
	if cfg.ExternalScheme != "http" && cfg.ExternalScheme != "https" {
		return fmt.Errorf("external_scheme must be http or https")
	}
	if !validTimeoutSeconds(cfg.WriteTimeoutSeconds) {
		return fmt.Errorf("write_timeout_seconds must be positive and fit in time.Duration")
	}
	if cfg.RateLimit.RequestsPerSecond <= 0 || cfg.RateLimit.Burst <= 0 {
		return fmt.Errorf("rate_limit requests_per_second and burst must be positive")
	}
	if err := cfg.Upstream.Validate(); err != nil {
		return fmt.Errorf("upstream: %w", err)
	}
	if cfg.WriteTimeoutSeconds <= cfg.Upstream.DialTimeoutSeconds ||
		cfg.WriteTimeoutSeconds-cfg.Upstream.DialTimeoutSeconds <= cfg.Upstream.ResponseHeaderTimeoutSeconds {
		return fmt.Errorf("write_timeout_seconds must exceed upstream dial and response header timeouts combined")
	}
	if err := cfg.CORS.Validate(); err != nil {
		return fmt.Errorf("cors: %w", err)
	}
	if len(cfg.Services) == 0 {
		return fmt.Errorf("services must contain at least one service")
	}
	for name, service := range cfg.Services {
		if !validServiceName(name) {
			return fmt.Errorf("services: invalid service name %q", name)
		}
		if !validHost(service.Host) || !validPort(service.Port) {
			return fmt.Errorf("services[%q]: invalid host or port", name)
		}
	}
	return nil
}

func (u Upstream) Validate() error {
	if !validTimeoutSeconds(u.DialTimeoutSeconds) || !validTimeoutSeconds(u.ResponseHeaderTimeoutSeconds) || u.MaxConnectionsPerHost <= 0 || u.MaxIdleConnectionsPerHost <= 0 {
		return fmt.Errorf("timeouts must be positive and fit in time.Duration; connection limits must be positive")
	}
	if u.MaxIdleConnectionsPerHost > u.MaxConnectionsPerHost {
		return fmt.Errorf("max_idle_connections_per_host must not exceed max_connections_per_host")
	}
	return nil
}

func (c CORS) Validate() error {
	seen := make(map[string]bool, len(c.AllowedOrigins))
	for _, origin := range c.AllowedOrigins {
		parsed, err := url.Parse(origin)
		if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" || parsed.User != nil || parsed.Path != "" || parsed.RawQuery != "" || parsed.ForceQuery || parsed.Fragment != "" || strings.HasSuffix(parsed.Host, ":") || strings.Contains(origin, "*") || !validHost(parsed.Hostname()) {
			return fmt.Errorf("invalid allowed_origin %q: use an exact http(s) origin without a path", origin)
		}
		if port := parsed.Port(); port != "" {
			value, err := strconv.Atoi(port)
			if err != nil || !validPort(value) {
				return fmt.Errorf("invalid allowed_origin %q: port must be between 1 and 65535", origin)
			}
		}
		if parsed.Scheme == "http" && !isLoopbackHost(parsed.Hostname()) {
			return fmt.Errorf("allowed_origin %q must use https outside localhost", origin)
		}
		key := strings.ToLower(origin)
		if seen[key] {
			return fmt.Errorf("duplicate allowed_origin %q", origin)
		}
		seen[key] = true
	}
	if len(c.AllowedMethods) == 0 {
		return fmt.Errorf("allowed_methods must not be empty")
	}
	seen = make(map[string]bool, len(c.AllowedMethods))
	for _, method := range c.AllowedMethods {
		switch method {
		case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut, http.MethodPatch, http.MethodDelete, http.MethodOptions:
		default:
			return fmt.Errorf("unsupported allowed_method %q", method)
		}
		if seen[method] {
			return fmt.Errorf("duplicate allowed_method %q", method)
		}
		seen[method] = true
	}
	if len(c.AllowedHeaders) == 0 {
		return fmt.Errorf("allowed_headers must not be empty")
	}
	seen = make(map[string]bool, len(c.AllowedHeaders))
	for _, header := range c.AllowedHeaders {
		if header == "" || strings.IndexFunc(header, func(r rune) bool {
			return !asciiLetterOrDigit(r) && r != '-'
		}) >= 0 {
			return fmt.Errorf("invalid allowed_header %q", header)
		}
		key := strings.ToLower(header)
		if seen[key] {
			return fmt.Errorf("duplicate allowed_header %q", header)
		}
		seen[key] = true
	}
	return nil
}

func (c CORS) Handler(next http.Handler) http.Handler {
	origins := make(map[string]bool, len(c.AllowedOrigins))
	for _, origin := range c.AllowedOrigins {
		origins[strings.ToLower(origin)] = true
	}
	return cors.New(cors.Options{
		AllowOriginFunc: func(origin string) bool { return origins[strings.ToLower(origin)] },
		AllowedMethods:  c.AllowedMethods,
		AllowedHeaders:  c.AllowedHeaders,
	}).Handler(next)
}

func validPort(port int) bool {
	return port >= 1 && port <= 65535
}

func validTimeoutSeconds(seconds int) bool {
	const maxSeconds = (1<<63 - 1) / int64(time.Second)
	return seconds > 0 && int64(seconds) <= maxSeconds
}

func validServiceName(name string) bool {
	if name == "" || name == "." || name == ".." {
		return false
	}
	for _, r := range name {
		if !asciiLetterOrDigit(r) && r != '_' && r != '-' && r != '.' {
			return false
		}
	}
	return true
}

func validHost(host string) bool {
	if ip := net.ParseIP(host); ip != nil {
		return true
	}
	if host == "" || len(host) > 253 {
		return false
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || len(label) > 63 || label[0] == '-' || label[len(label)-1] == '-' {
			return false
		}
		for _, r := range label {
			if !asciiLetterOrDigit(r) && r != '-' {
				return false
			}
		}
	}
	return true
}

func asciiLetterOrDigit(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9')
}

func isLoopbackHost(host string) bool {
	if strings.EqualFold(host, "localhost") {
		return true
	}
	ip := net.ParseIP(host)
	return ip != nil && ip.IsLoopback()
}
