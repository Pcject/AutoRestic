// backend/internal/config/config.go
package config

import "os"

type Config struct {
	Port          string
	DBPath        string
	ResticBin     string
	EncKeyPath    string
	AuthToken     string
	CORSOrigins   string
	LogRetainDays int
}

func Load() *Config {
	return &Config{
		Port:          getEnv("AUTORESTIC_PORT", "8080"),
		DBPath:        getEnv("AUTORESTIC_DB_PATH", "data/autorestic.db"),
		ResticBin:     getEnv("AUTORESTIC_RESTIC_BIN", "restic"),
		EncKeyPath:    getEnv("AUTORESTIC_ENC_KEY_PATH", "data/autorestic.key"),
		AuthToken:     getEnv("AUTORESTIC_AUTH_TOKEN", ""),
		CORSOrigins:   getEnv("AUTORESTIC_CORS_ORIGINS", ""),
		LogRetainDays: 30,
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
