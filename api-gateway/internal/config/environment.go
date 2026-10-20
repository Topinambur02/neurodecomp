package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strconv"
	"strings"
)

// Compose overrides network settings while keeping the selected JSON config
// as the source of authentication, CORS, timeouts and other application settings.
func (cfg *Config) applyEnvironment() error {
	if host, ok := os.LookupEnv("GATEWAY_HOST"); ok {
		cfg.Host = host
	}
	if port, ok := os.LookupEnv("GATEWAY_PORT"); ok {
		value, err := strconv.Atoi(port)
		if err != nil {
			return fmt.Errorf("GATEWAY_PORT must be an integer")
		}
		cfg.Port = value
	}
	if services, ok := os.LookupEnv("GATEWAY_SERVICES"); ok {
		decoder := json.NewDecoder(strings.NewReader(services))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(&cfg.Services); err != nil {
			return fmt.Errorf("decode GATEWAY_SERVICES: %w", err)
		}
		var extra any
		if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
			return fmt.Errorf("GATEWAY_SERVICES must contain exactly one JSON object")
		}
	}
	return nil
}
