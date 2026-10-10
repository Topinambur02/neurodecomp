package config

import (
	"encoding/json"
	"fmt"
	"os"
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

type Config struct {
	Port                int                `json:"port"`
	Host                string             `json:"host"`
	Services            map[string]Service `json:"services"`
	RateLimit           RateLimit          `json:"rate_limit"`
	Upstream            Upstream           `json:"upstream"`
	WriteTimeoutSeconds int                `json:"write_timeout_seconds"`
}

func LoadConfig(path string) (cfg Config, err error) {
	cfg = Config{
		Port:                8080,
		Host:                "localhost",
		Services:            map[string]Service{},
		RateLimit:           RateLimit{RequestsPerSecond: 10, Burst: 20},
		Upstream:            Upstream{DialTimeoutSeconds: 3, ResponseHeaderTimeoutSeconds: 10, MaxConnectionsPerHost: 100, MaxIdleConnectionsPerHost: 20},
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

	jsonParser := json.NewDecoder(configFile)
	if err := jsonParser.Decode(&cfg); err != nil {
		return Config{}, fmt.Errorf("decode config %q: %w", path, err)
	}

	if cfg.WriteTimeoutSeconds <= 0 {
		return Config{}, fmt.Errorf("write_timeout_seconds must be positive")
	}
	if err := cfg.Upstream.Validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

func (u Upstream) Validate() error {
	if u.DialTimeoutSeconds <= 0 || u.ResponseHeaderTimeoutSeconds <= 0 || u.MaxConnectionsPerHost <= 0 || u.MaxIdleConnectionsPerHost <= 0 {
		return fmt.Errorf("upstream timeouts and connection limits must be positive")
	}
	return nil
}
