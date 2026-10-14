package config

import (
	"errors"
	"os"
)

type Config struct {
	AppEnv      string
	HTTPPort    string
	DatabaseURL string
}

func Load() (Config, error) {
	appEnv := os.Getenv("appEnv")
	if appEnv == "" {
		appEnv = "development"
	}

	httpPort := os.Getenv("httpPort")
	if httpPort == "" {
		httpPort = "8080"
	}

	databaseURL := os.Getenv("databaseURL")
	if databaseURL == "" {
		return Config{}, errors.New("DATABASE_URL is required")
	}

	return Config{
		AppEnv:      appEnv,
		HTTPPort:    httpPort,
		DatabaseURL: databaseURL,
	}, nil
}
