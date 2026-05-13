package domain

import "time"

const EventTypeAnalyticsHintGenerated = "analytics.hint.generated"

type HintGenerateRequest struct {
	UserID               string `json:"user_id"`
	TaskID               string `json:"task_id"`
	CurrentAttemptNumber int    `json:"current_attempt_number"`
	RequestID            string `json:"request_id"`
}

type HintGenerateResponse struct {
	HintID        string    `json:"hint_id"`
	UserID        string    `json:"user_id"`
	TaskID        string    `json:"task_id"`
	HintType      string    `json:"hint_type"`
	Message       string    `json:"message"`
	RelatedSkills []string  `json:"related_skills,omitempty"`
	Severity      string    `json:"severity,omitempty"`
	AttemptNumber int       `json:"attempt_number"`
	CreatedAt     time.Time `json:"created_at"`
}

type AnalyticsHintGeneratedEvent struct {
	EventID       string    `json:"event_id"`
	EventVersion  int       `json:"event_version"`
	EventType     string    `json:"event_type"`
	UserID        string    `json:"user_id"`
	TaskID        string    `json:"task_id"`
	HintID        string    `json:"hint_id"`
	HintType      string    `json:"hint_type"`
	AttemptNumber int       `json:"attempt_number"`
	RelatedSkills []string  `json:"related_skills,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}

type HintContextForLLM struct {
	UserID               string         `json:"user_id"`
	TaskID               string         `json:"task_id"`
	CurrentAttemptNumber int            `json:"current_attempt_number"`
	Task                 TaskContext    `json:"task"`
	Attempts             []AttemptLog   `json:"attempts"`
	PreviousHints        []HintEventLog `json:"previous_hints,omitempty"`
	PreparedAt           time.Time      `json:"prepared_at"`
}

type HintEventLog struct {
	EventID       string    `json:"event_id"`
	UserID        string    `json:"user_id"`
	TaskID        string    `json:"task_id"`
	HintID        string    `json:"hint_id"`
	EventType     string    `json:"event_type"`
	HintType      string    `json:"hint_type"`
	AttemptNumber int       `json:"attempt_number"`
	RelatedSkills []string  `json:"related_skills,omitempty"`
	Message       string    `json:"message,omitempty"`
	RequestJSON   string    `json:"request_json,omitempty"`
	ResponseJSON  string    `json:"response_json,omitempty"`
	CreatedAt     time.Time `json:"created_at"`
}
