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

type Config struct {
	Port      int                `json:"port"`
	Host      string             `json:"host"`
	Services  map[string]Service `json:"services"`
	RateLimit RateLimit          `json:"rate_limit"`
}

func LoadConfig(path string) (cfg Config, err error) {
	cfg = Config{
		Port:      8080,
		Host:      "localhost",
		Services:  map[string]Service{},
		RateLimit: RateLimit{RequestsPerSecond: 10, Burst: 20},
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

	return cfg, nil
}
