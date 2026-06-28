package config

import (
	"errors"
	"os"
	"strings"
)

type ControlPlaneConfig struct {
	AppName                 string
	AppEnv                  string
	PublicWebURL            string
	ControlPlaneURL         string
	ControlPlaneInternalURL string
	ControlPlaneInternalKey string
	DatabaseURL             string
	QueueDriver             string
	QueueRedisURL           string
	CacheDriver             string
	CacheRedisURL           string
	AgentTokenSigningSecret string
	AgentReleaseVersion     string
	DNSSecretEncryptionKey  string
	GeoIPDBPath             string
	TrustedAgentProxyCIDRs  []string
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
		DatabaseURL:             os.Getenv("DATABASE_URL"),
		QueueDriver:             envOrDefault("QUEUE_DRIVER", "asynq"),
		QueueRedisURL:           os.Getenv("QUEUE_REDIS_URL"),
		CacheDriver:             envOrDefault("CACHE_DRIVER", "redis"),
		CacheRedisURL:           os.Getenv("CACHE_REDIS_URL"),
		AgentTokenSigningSecret: os.Getenv("AGENT_TOKEN_SIGNING_SECRET"),
		AgentReleaseVersion:     envOrDefault("AGENT_RELEASE_VERSION", "latest"),
		DNSSecretEncryptionKey:  os.Getenv("DNS_SECRET_ENCRYPTION_KEY"),
		GeoIPDBPath:             envOrDefault("GEOIP_DB_PATH", "/data/geoip/dbip-country-lite.mmdb"),
		TrustedAgentProxyCIDRs:  splitCSV(os.Getenv("TRUSTED_AGENT_PROXY_CIDRS")),
		LogLevel:                envOrDefault("LOG_LEVEL", "info"),
	}
	if cfg.AppName == "" {
		return ControlPlaneConfig{}, errors.New("APP_NAME is required")
	}
	if cfg.ControlPlaneURL == "" {
		return ControlPlaneConfig{}, errors.New("CONTROL_PLANE_URL is required")
	}
	if cfg.DatabaseURL == "" {
		return ControlPlaneConfig{}, errors.New("DATABASE_URL is required")
	}
	if cfg.AgentTokenSigningSecret == "" {
		return ControlPlaneConfig{}, errors.New("AGENT_TOKEN_SIGNING_SECRET is required")
	}
	return cfg, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	return result
}

func envOrDefault(key string, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
