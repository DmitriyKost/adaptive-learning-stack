package domain

import "time"

type RawEventLog struct {
	EventID        string
	EventType      string
	UserID         string
	TaskID         string
	AttemptID      string
	SourceTopic    string
	KafkaPartition int
	KafkaOffset    int64
	EventTime      time.Time
	Payload        string
}

type AttemptLog struct {
	EventID            string
	UserID             string
	TaskID             string
	AttemptID          string
	AttemptNumber      int
	ExecutionSuccess   bool
	IsCorrect          bool
	SubmittedSQL       string
	ReferenceSQL       string
	ErrorType          string
	ErrorMessage       string
	ExecutionTimeMS    int64
	RowCount           int
	HintRequested      bool
	HintUsed           bool
	HintType           string
	HintsCount         int
	Skills             []string
	TaskJSON           string
	ExecutionJSON      string
	ActualResultJSON   string
	ExpectedResultJSON string
	CreatedAt          time.Time
}

type CompletionLog struct {
	EventID           string
	UserID            string
	TaskID            string
	AttemptID         string
	GraphCode         string
	ProfessionalTrack string
	GraphStateJSON    string
	TaskJSON          string
	ExecutionJSON     string
	HintJSON          string
	CreatedAt         time.Time
}

type RecommendationLog struct {
	EventID         string
	UserID          string
	TaskID          string
	GraphCode       string
	Score           float64
	Reason          string
	RepeatMode      bool
	CandidateSource string
	Payload         string
	CreatedAt       time.Time
}

type LearnerModelUpdateLog struct {
	EventID         string
	UserID          string
	SkillID         string
	SkillCode       string
	OldMasteryScore float64
	NewMasteryScore float64
	OldConfidence   float64
	NewConfidence   float64
	Source          string
	Reason          string
	AnalysisRunID   string
	CreatedAt       time.Time
}

type LLMContext struct {
	UserID                string                  `json:"user_id"`
	TaskID                string                  `json:"task_id"`
	AttemptID             string                  `json:"attempt_id"`
	Task                  TaskContext             `json:"task"`
	Attempts              []AttemptLog            `json:"attempts"`
	CompletedEvent        TaskCompletedEvent      `json:"completed_event"`
	GraphState            *GraphStateSnapshot     `json:"graph_state,omitempty"`
	RecentRecommendations []RecommendationLog     `json:"recent_recommendations,omitempty"`
	RecentModelUpdates    []LearnerModelUpdateLog `json:"recent_model_updates,omitempty"`
	PreparedAt            time.Time               `json:"prepared_at"`
}

type IntelligenceResult struct {
	Assessment         AnalyticsSkillAssessmentUpdatedEvent
	Career             *CareerRecommendationCreatedEvent
	RequestContextJSON string
	ResponseJSON       string
}

type AnalysisRunLog struct {
	RunID           string
	UserID          string
	SourceTaskID    string
	SourceAttemptID string
	Status          string
	ModelVersion    string
	PromptVersion   string
	RequestContext  string
	ResponsePayload string
	ErrorMessage    string
	StartedAt       time.Time
	CompletedAt     time.Time
}
