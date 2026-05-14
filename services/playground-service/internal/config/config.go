package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                   string
	HTTPAddr              string
	DatabaseURL           string
	JWTSecret             string
	TrustedGatewayHeaders bool

	KafkaBrokers          []string
	KafkaClientID         string
	KafkaExecutionTopic   string
	KafkaAutoCreateTopics bool

	ExecutionTimeout time.Duration
	StatementTimeout time.Duration
	LockTimeout      time.Duration
	MaxResultRows    int

	TaskSchema     string
	InternalSchema string
	ReadonlyRole   string

	TaskProgressBaseURL     string
	TaskProgressHTTPTimeout time.Duration
}

func Load() (Config, error) {
	cfg := Config{
		Env:                     getEnv("ENV", "local"),
		HTTPAddr:                getEnv("HTTP_ADDR", ":8080"),
		DatabaseURL:             getEnv("DATABASE_URL", "postgres://playground_app:playground_app@localhost:5432/playground?sslmode=disable"),
		JWTSecret:               getEnv("JWT_SECRET", "change-me-in-production"),
		TrustedGatewayHeaders:   getBool("TRUSTED_GATEWAY_HEADERS", false),
		KafkaBrokers:            splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaClientID:           getEnv("KAFKA_CLIENT_ID", "playground-service"),
		KafkaExecutionTopic:     getEnv("KAFKA_EXECUTION_TOPIC", "playground.execution.completed"),
		KafkaAutoCreateTopics:   getBool("KAFKA_AUTO_CREATE_TOPICS", true),
		ExecutionTimeout:        5 * time.Second,
		StatementTimeout:        3 * time.Second,
		LockTimeout:             time.Second,
		MaxResultRows:           500,
		TaskSchema:              getEnv("TASK_SCHEMA", "task_data"),
		InternalSchema:          getEnv("INTERNAL_SCHEMA", "playground_internal"),
		ReadonlyRole:            getEnv("READONLY_ROLE", "playground_readonly"),
		TaskProgressBaseURL:     getEnv("TASK_PROGRESS_BASE_URL", "http://localhost:8083"),
		TaskProgressHTTPTimeout: 3 * time.Second,
	}

	if cfg.JWTSecret == "" && !cfg.TrustedGatewayHeaders {
		return Config{}, fmt.Errorf("JWT_SECRET is required unless TRUSTED_GATEWAY_HEADERS=true")
	}

	var err error
	if cfg.ExecutionTimeout, err = getDuration("EXECUTION_TIMEOUT", cfg.ExecutionTimeout); err != nil {
		return Config{}, err
	}
	if cfg.StatementTimeout, err = getDuration("STATEMENT_TIMEOUT", cfg.StatementTimeout); err != nil {
		return Config{}, err
	}
	if cfg.LockTimeout, err = getDuration("LOCK_TIMEOUT", cfg.LockTimeout); err != nil {
		return Config{}, err
	}
	if cfg.MaxResultRows, err = getInt("MAX_RESULT_ROWS", cfg.MaxResultRows); err != nil {
		return Config{}, err
	}
	if cfg.MaxResultRows <= 0 {
		return Config{}, fmt.Errorf("MAX_RESULT_ROWS must be positive")
	}
	if cfg.TaskProgressHTTPTimeout, err = getDuration("TASK_PROGRESS_HTTP_TIMEOUT", cfg.TaskProgressHTTPTimeout); err != nil {
		return Config{}, err
	}
	if cfg.TaskProgressBaseURL == "" {
		return Config{}, fmt.Errorf("TASK_PROGRESS_BASE_URL is required")
	}
	if len(cfg.KafkaBrokers) == 0 {
		return Config{}, fmt.Errorf("KAFKA_BROKERS is required")
	}
	if cfg.KafkaExecutionTopic == "" {
		return Config{}, fmt.Errorf("KAFKA_EXECUTION_TOPIC is required")
	}

	return cfg, nil
}

func getEnv(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func getBool(key string, fallback bool) bool {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return fallback
	}
	return parsed
}

func getDuration(key string, fallback time.Duration) (time.Duration, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func getInt(key string, fallback int) (int, error) {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback, nil
	}
	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return parsed, nil
}

func splitCSV(value string) []string {
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		trimmed := strings.TrimSpace(part)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	return result
}
