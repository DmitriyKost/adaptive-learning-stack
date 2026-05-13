package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env             string
	HTTPAddr        string
	DatabaseURL     string
	JWTSecret       string
	AccessTokenTTL  time.Duration
	RefreshTokenTTL time.Duration
	BCryptCost      int
}

func Load() (Config, error) {
	cfg := Config{
		Env:             getEnv("ENV", "local"),
		HTTPAddr:        getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:     getEnv("DATABASE_URL", "postgres://auth:auth@localhost:5432/auth?sslmode=disable"),
		JWTSecret:       getEnv("JWT_SECRET", "change-me-in-production"),
		AccessTokenTTL:  15 * time.Minute,
		RefreshTokenTTL: 30 * 24 * time.Hour,
		BCryptCost:      12,
	}

	if raw := os.Getenv("ACCESS_TOKEN_TTL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse ACCESS_TOKEN_TTL: %w", err)
		}
		cfg.AccessTokenTTL = d
	}

	if raw := os.Getenv("REFRESH_TOKEN_TTL"); raw != "" {
		d, err := time.ParseDuration(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse REFRESH_TOKEN_TTL: %w", err)
		}
		cfg.RefreshTokenTTL = d
	}

	if raw := os.Getenv("BCRYPT_COST"); raw != "" {
		cost, err := strconv.Atoi(raw)
		if err != nil {
			return Config{}, fmt.Errorf("parse BCRYPT_COST: %w", err)
		}
		cfg.BCryptCost = cost
	}

	if cfg.JWTSecret == "" {
		return Config{}, fmt.Errorf("JWT_SECRET is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := os.Getenv(key)
	if value == "" {
		return fallback
	}
	return value
}
