package config

import (
	"errors"
	"os"
)

type ControlPlaneConfig struct {
	AppName                 string
	AppEnv                  string
	PublicWebURL            string
	ControlPlaneURL         string
	ControlPlaneInternalURL string
	ControlPlaneInternalKey string
	DatabaseDriver          string
	DatabaseURL             string
	QueueDriver             string
	QueueRedisURL           string
	CacheDriver             string
	CacheRedisURL           string
	AgentTokenSigningSecret string
	DNSSecretEncryptionKey  string
	LogLevel                string
}

func LoadControlPlane() (ControlPlaneConfig, error) {
	cfg := ControlPlaneConfig{
		AppName:                 os.Getenv("APP_NAME"),
		AppEnv:                  envOrDefault("APP_ENV", "development"),
		PublicWebURL:            os.Getenv("PUBLIC_WEB_URL"),
		ControlPlaneURL:         os.Getenv("CONTROL_PLANE_URL"),
		ControlPlaneInternalURL: os.Getenv("CONTROL_PLANE_INTERNAL_URL"),
		ControlPlaneInternalKey: os.Getenv("CONTROL_PLANE_INTERNAL_JWT_SECRET"),
		DatabaseDriver:          envOrDefault("DATABASE_DRIVER", "sqlite"),
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		QueueDriver:             envOrDefault("QUEUE_DRIVER", "asynq"),
		QueueRedisURL:           os.Getenv("QUEUE_REDIS_URL"),
		CacheDriver:             envOrDefault("CACHE_DRIVER", "redis"),
		CacheRedisURL:           os.Getenv("CACHE_REDIS_URL"),
		AgentTokenSigningSecret: os.Getenv("AGENT_TOKEN_SIGNING_SECRET"),
		DNSSecretEncryptionKey:  os.Getenv("DNS_SECRET_ENCRYPTION_KEY"),
		LogLevel:                envOrDefault("LOG_LEVEL", "info"),
	}
	if cfg.AppName == "" {
		return ControlPlaneConfig{}, errors.New("APP_NAME is required")
	}
	if cfg.ControlPlaneURL == "" {
		return ControlPlaneConfig{}, errors.New("CONTROL_PLANE_URL is required")
	}
	if cfg.AgentTokenSigningSecret == "" {
		return ControlPlaneConfig{}, errors.New("AGENT_TOKEN_SIGNING_SECRET is required")
	}
	return cfg, nil
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
