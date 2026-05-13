package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env                 string
	HTTPAddr            string
	DatabaseURL         string
	JWTSecret           string
	AllowGatewayHeaders bool

	DefaultGraphCode          string
	MasteryThreshold          float64
	PrerequisiteThreshold     float64
	RecallThreshold           float64
	RecommendationWaitTimeout time.Duration
	HintGenerationTimeout     time.Duration
	AnalyticsInternalURL      string
	MinHalfLifeDays           float64
	MaxHalfLifeDays           float64

	KafkaEnabled  bool
	KafkaBrokers  []string
	KafkaClientID string
	KafkaGroupID  string

	TopicPlaygroundExecutionCompleted    string
	TopicAnalyticsSkillAssessmentUpdated string
	TopicTaskChecked                     string
	TopicTaskCompleted                   string
	TopicTaskRecommended                 string
	TopicLearnerModelUpdated             string
}

func Load() (Config, error) {
	cfg := Config{
		Env:                 getEnv("ENV", "local"),
		HTTPAddr:            getEnv("HTTP_ADDR", ":8082"),
		DatabaseURL:         getEnv("DATABASE_URL", "postgres://task_progress:task_progress@localhost:5434/task_progress?sslmode=disable"),
		JWTSecret:           getEnv("JWT_SECRET", "change-me-in-production"),
		AllowGatewayHeaders: getEnvBool("ALLOW_GATEWAY_HEADERS", false),

		DefaultGraphCode:          getEnv("DEFAULT_GRAPH_CODE", "core_sql"),
		MasteryThreshold:          getEnvFloat("MASTERY_THRESHOLD", 0.70),
		PrerequisiteThreshold:     getEnvFloat("PREREQUISITE_THRESHOLD", 0.60),
		RecallThreshold:           getEnvFloat("RECALL_THRESHOLD", 0.35),
		RecommendationWaitTimeout: ParseDurationEnv("RECOMMENDATION_WAIT_TIMEOUT", 2*time.Minute),
		HintGenerationTimeout:     ParseDurationEnv("HINT_GENERATION_TIMEOUT", 15*time.Second),
		AnalyticsInternalURL:      getEnv("ANALYTICS_INTERNAL_URL", "http://localhost:8083"),
		MinHalfLifeDays:           getEnvFloat("MIN_HALF_LIFE_DAYS", 1.0),
		MaxHalfLifeDays:           getEnvFloat("MAX_HALF_LIFE_DAYS", 21.0),

		KafkaEnabled:  getEnvBool("KAFKA_ENABLED", true),
		KafkaBrokers:  getEnvList("KAFKA_BROKERS", []string{"localhost:9092"}),
		KafkaClientID: getEnv("KAFKA_CLIENT_ID", "task-progress-service"),
		KafkaGroupID:  getEnv("KAFKA_GROUP_ID", "task-progress-service"),

		TopicPlaygroundExecutionCompleted:    getEnv("KAFKA_TOPIC_PLAYGROUND_EXECUTION_COMPLETED", "playground.execution.completed"),
		TopicAnalyticsSkillAssessmentUpdated: getEnv("KAFKA_TOPIC_ANALYTICS_SKILL_ASSESSMENT_UPDATED", "analytics.skill_assessment.updated"),
		TopicTaskChecked:                     getEnv("KAFKA_TOPIC_TASK_CHECKED", "task.checked"),
		TopicTaskCompleted:                   getEnv("KAFKA_TOPIC_TASK_COMPLETED", "task.completed"),
		TopicTaskRecommended:                 getEnv("KAFKA_TOPIC_TASK_RECOMMENDED", "task.recommended"),
		TopicLearnerModelUpdated:             getEnv("KAFKA_TOPIC_LEARNER_MODEL_UPDATED", "learner_model.updated"),
	}

	if cfg.DatabaseURL == "" {
		return Config{}, fmt.Errorf("DATABASE_URL is required")
	}
	if cfg.JWTSecret == "" && !cfg.AllowGatewayHeaders {
		return Config{}, fmt.Errorf("JWT_SECRET is required when ALLOW_GATEWAY_HEADERS=false")
	}
	if cfg.MinHalfLifeDays <= 0 || cfg.MaxHalfLifeDays <= 0 || cfg.MinHalfLifeDays > cfg.MaxHalfLifeDays {
		return Config{}, fmt.Errorf("invalid half-life configuration")
	}
	if cfg.MasteryThreshold <= 0 || cfg.MasteryThreshold > 1 {
		return Config{}, fmt.Errorf("MASTERY_THRESHOLD must be in (0, 1]")
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

func getEnvBool(key string, fallback bool) bool {
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

func getEnvFloat(key string, fallback float64) float64 {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := strconv.ParseFloat(value, 64)
	if err != nil {
		return fallback
	}
	return parsed
}

func getEnvList(key string, fallback []string) []string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parts := strings.Split(value, ",")
	result := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			result = append(result, part)
		}
	}
	if len(result) == 0 {
		return fallback
	}
	return result
}

func ParseDurationEnv(key string, fallback time.Duration) time.Duration {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	parsed, err := time.ParseDuration(value)
	if err != nil {
		return fallback
	}
	return parsed
}
