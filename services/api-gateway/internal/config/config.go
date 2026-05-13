package config

import (
	"os"
	"strings"
	"time"
)

type Config struct {
	Env                    string
	HTTPAddr               string
	JWTSecret              string
	RequestTimeout         time.Duration
	ShutdownTimeout        time.Duration
	AuthServiceURL         string
	TaskProgressServiceURL string
	PlaygroundServiceURL   string
	AnalyticsServiceURL    string
	ForwardAuthorization   bool
	CheckDownstreamReady   bool
	CORSAllowedOrigins     []string
	CORSAllowedHeaders     string
	CORSAllowedMethods     string
}

func Load() Config {
	return Config{
		Env:                    getEnv("ENV", "local"),
		HTTPAddr:               getEnv("HTTP_ADDR", ":8080"),
		JWTSecret:              getEnv("JWT_SECRET", "change-me-in-production"),
		RequestTimeout:         getDuration("REQUEST_TIMEOUT", 30*time.Second),
		ShutdownTimeout:        getDuration("SHUTDOWN_TIMEOUT", 10*time.Second),
		AuthServiceURL:         trimURL(getEnv("AUTH_SERVICE_URL", "http://localhost:8081")),
		TaskProgressServiceURL: trimURL(getEnv("TASK_PROGRESS_SERVICE_URL", "http://localhost:8082")),
		PlaygroundServiceURL:   trimURL(getEnv("PLAYGROUND_SERVICE_URL", "http://localhost:8083")),
		AnalyticsServiceURL:    trimURL(getEnv("ANALYTICS_SERVICE_URL", "http://localhost:8084")),
		ForwardAuthorization:   getBool("FORWARD_AUTHORIZATION", true),
		CheckDownstreamReady:   getBool("CHECK_DOWNSTREAM_READY", true),
		CORSAllowedOrigins:     getCSV("CORS_ALLOWED_ORIGINS", "*"),
		CORSAllowedHeaders:     getEnv("CORS_ALLOWED_HEADERS", "Authorization,Content-Type,X-Request-ID"),
		CORSAllowedMethods:     getEnv("CORS_ALLOWED_METHODS", "GET,POST,PUT,PATCH,DELETE,OPTIONS"),
	}
}

func getEnv(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}

func getDuration(key string, fallback time.Duration) time.Duration {
	raw := strings.TrimSpace(os.Getenv(key))
	if raw == "" {
		return fallback
	}
	d, err := time.ParseDuration(raw)
	if err != nil {
		return fallback
	}
	return d
}

func getBool(key string, fallback bool) bool {
	raw := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	switch raw {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return fallback
	}
}

func getCSV(key, fallback string) []string {
	raw := getEnv(key, fallback)
	parts := strings.Split(raw, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		return []string{fallback}
	}
	return result
}

func trimURL(raw string) string {
	return strings.TrimRight(strings.TrimSpace(raw), "/")
}
