package domain

import "time"

const EventTypeAnalyticsHintGenerated = "analytics.hint.generated"

type UserTaskHintState struct {
	UserID                string    `json:"user_id"`
	TaskID                string    `json:"task_id"`
	HintCount             int       `json:"hint_count"`
	LastHintID            string    `json:"last_hint_id,omitempty"`
	LastHintType          string    `json:"last_hint_type,omitempty"`
	LastHintAttemptNumber int       `json:"last_hint_attempt_number,omitempty"`
	LastHintAt            time.Time `json:"last_hint_at"`
	UpdatedAt             time.Time `json:"updated_at"`
}

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
