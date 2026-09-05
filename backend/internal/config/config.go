package config

import (
	"fmt"
	"os"
)

type Config struct {
	Port                    string
	DSN                     string
	ExporterURL             string
	FrontendRenderURL       string
	DatasourceEncryptionKey string
	QueryTimeoutSeconds     int
	QueryMaxRows            int
	HTTPMaxBodyBytes        int64
}

func Load() Config {
	return Config{
		Port:                    env("PORT", "18085"),
		DSN:                     env("DATABASE_DSN", "dashboard:dashboard@tcp(127.0.0.1:3306)/dashboard?charset=utf8mb4&parseTime=True&loc=Local"),
		ExporterURL:             env("EXPORTER_URL", "http://127.0.0.1:3000"),
		FrontendRenderURL:       env("FRONTEND_RENDER_URL", "http://localhost:5173"),
		DatasourceEncryptionKey: env("DATASOURCE_ENCRYPTION_KEY", "data-vision-development-key-change-me"),
		QueryTimeoutSeconds:     envInt("QUERY_TIMEOUT_SECONDS", 30),
		QueryMaxRows:            envInt("QUERY_MAX_ROWS", 10000),
		HTTPMaxBodyBytes:        int64(envInt("HTTP_MAX_BODY_BYTES", 5*1024*1024)),
	}
}

func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envInt(key string, fallback int) int {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	var parsed int
	if _, err := fmt.Sscan(value, &parsed); err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}
