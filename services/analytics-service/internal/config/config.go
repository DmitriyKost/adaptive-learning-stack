package config

import (
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	Env      string
	HTTPAddr string

	ClickHouseAddr        string
	ClickHouseDatabase    string
	ClickHouseUsername    string
	ClickHousePassword    string
	ClickHouseSecure      bool
	ClickHouseAutoMigrate bool

	KafkaEnabled                         bool
	KafkaBrokers                         []string
	KafkaClientID                        string
	KafkaGroupID                         string
	TopicTaskChecked                     string
	TopicTaskCompleted                   string
	TopicTaskRecommended                 string
	TopicLearnerModelUpdated             string
	TopicAnalyticsSkillAssessmentUpdated string
	TopicCareerRecommendationCreated     string
	TopicHintGenerated                   string

	HintGenerationTimeout time.Duration

	// External Python intelligence service.
	IntelligenceBaseURL       string
	IntelligenceAPIKey        string
	IntelligenceReadyPath     string
	IntelligenceEvaluatePath  string
	IntelligenceHintPath      string
	IntelligenceHTTPTimeout   time.Duration
	IntelligenceIncludeCareer bool
}

func Load() Config {
	return Config{
		Env:                                  getEnv("ENV", "local"),
		HTTPAddr:                             getEnv("HTTP_ADDR", ":8083"),
		ClickHouseAddr:                       getEnv("CLICKHOUSE_ADDR", "localhost:9000"),
		ClickHouseDatabase:                   getEnv("CLICKHOUSE_DATABASE", "analytics"),
		ClickHouseUsername:                   getEnv("CLICKHOUSE_USERNAME", "analytics"),
		ClickHousePassword:                   getEnv("CLICKHOUSE_PASSWORD", "analytics"),
		ClickHouseSecure:                     getBool("CLICKHOUSE_SECURE", false),
		ClickHouseAutoMigrate:                getBool("CLICKHOUSE_AUTO_MIGRATE", false),
		KafkaEnabled:                         getBool("KAFKA_ENABLED", true),
		KafkaBrokers:                         splitCSV(getEnv("KAFKA_BROKERS", "localhost:9092")),
		KafkaClientID:                        getEnv("KAFKA_CLIENT_ID", "analytics-service"),
		KafkaGroupID:                         getEnv("KAFKA_GROUP_ID", "analytics-service"),
		TopicTaskChecked:                     getEnv("KAFKA_TOPIC_TASK_CHECKED", "task.checked"),
		TopicTaskCompleted:                   getEnv("KAFKA_TOPIC_TASK_COMPLETED", "task.completed"),
		TopicTaskRecommended:                 getEnv("KAFKA_TOPIC_TASK_RECOMMENDED", "task.recommended"),
		TopicLearnerModelUpdated:             getEnv("KAFKA_TOPIC_LEARNER_MODEL_UPDATED", "learner_model.updated"),
		TopicAnalyticsSkillAssessmentUpdated: getEnv("KAFKA_TOPIC_ANALYTICS_SKILL_ASSESSMENT_UPDATED", "analytics.skill_assessment.updated"),
		TopicCareerRecommendationCreated:     getEnv("KAFKA_TOPIC_CAREER_RECOMMENDATION_CREATED", "analytics.career_recommendation.created"),
		TopicHintGenerated:                   getEnv("KAFKA_TOPIC_ANALYTICS_HINT_GENERATED", "analytics.hint.generated"),
		HintGenerationTimeout:                getDuration("HINT_GENERATION_TIMEOUT", 600*time.Second),
		IntelligenceBaseURL:                  getEnv("INTELLIGENCE_BASE_URL", ""),
		IntelligenceAPIKey:                   getEnv("INTELLIGENCE_API_KEY", ""),
		IntelligenceReadyPath:                getEnv("INTELLIGENCE_READY_PATH", "/ready"),
		IntelligenceEvaluatePath:             getEnv("INTELLIGENCE_EVALUATE_PATH", "/v1/assessments/evaluate"),
		IntelligenceHintPath:                 getEnv("INTELLIGENCE_HINT_PATH", "/v1/hints/generate"),
		IntelligenceHTTPTimeout:              getDuration("INTELLIGENCE_HTTP_TIMEOUT", 600*time.Second),
		IntelligenceIncludeCareer:            getBool("INTELLIGENCE_INCLUDE_CAREER", false),
	}
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func splitCSV(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		p := strings.TrimSpace(part)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func getBool(key string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func getDuration(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
