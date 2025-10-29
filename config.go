package main

import (
	"fmt"
	"os"
	"time"

	"github.com/joho/godotenv"
)

// Config holds SMPP connection and runtime configuration.
type Config struct {
	Host        string
	Port        string
	SystemID    string
	Password    string
	SystemType  string
	EnquireLink time.Duration
	ReadTimeout time.Duration
	TLS         bool

	// optional defaults for demonstration
	SourceAddr string
	DestAddr   string
}

// LoadConfigFromEnv loads configuration from environment variables and .env (if present).
// Required: SMPP_HOST (SMPP_PORT has a default of 2775)
func LoadConfigFromEnv() (Config, error) {
	_ = godotenv.Load() // ignore error; .env is optional

	cfg := Config{
		Host:        os.Getenv("SMPP_HOST"),
		Port:        os.Getenv("SMPP_PORT"),
		SystemID:    os.Getenv("SYSTEM_ID"),
		Password:    os.Getenv("PASSWORD"),
		SystemType:  os.Getenv("SYSTEM_TYPE"),
		EnquireLink: 20 * time.Second,
		ReadTimeout: 22 * time.Second,
		TLS:         true,
		SourceAddr:  os.Getenv("SMPP_SOURCE"),
		DestAddr:    os.Getenv("SMPP_DEST"),
	}

	if cfg.Host == "" {
		return cfg, fmt.Errorf("SMPP_HOST is required")
	}
	if cfg.Port == "" {
		cfg.Port = "2775"
	}
	return cfg, nil
}
