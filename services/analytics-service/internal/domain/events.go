package domain

import "time"

const (
	EventTypeTaskChecked                     = "task.checked"
	EventTypeTaskCompleted                   = "task.completed"
	EventTypeTaskRecommended                 = "task.recommended"
	EventTypeLearnerModelUpdated             = "learner_model.updated"
	EventTypeAnalyticsSkillAssessmentUpdated = "analytics.skill_assessment.updated"
	EventTypeCareerRecommendationCreated     = "analytics.career_recommendation.created"
)

type EventEnvelope struct {
	EventID   string    `json:"event_id"`
	EventType string    `json:"event_type"`
	UserID    string    `json:"user_id,omitempty"`
	TaskID    string    `json:"task_id,omitempty"`
	AttemptID string    `json:"attempt_id,omitempty"`
	CreatedAt time.Time `json:"created_at,omitempty"`
}

type QueryResult struct {
	Columns         []string `json:"columns,omitempty"`
	Rows            [][]any  `json:"rows,omitempty"`
	RowCount        int      `json:"row_count,omitempty"`
	Truncated       bool     `json:"truncated,omitempty"`
	ExecutionTimeMS int64    `json:"execution_time_ms,omitempty"`
	QueryTimeMS     int64    `json:"query_time_ms,omitempty"`
}

type TaskSkill struct {
	TaskID    string  `json:"task_id,omitempty"`
	SkillID   string  `json:"skill_id"`
	SkillCode string  `json:"skill_code,omitempty"`
	SkillName string  `json:"skill_name,omitempty"`
	Weight    float64 `json:"weight"`
}

type TaskContext struct {
	TaskID       string      `json:"task_id"`
	Title        string      `json:"title,omitempty"`
	Description  string      `json:"description,omitempty"`
	Difficulty   string      `json:"difficulty,omitempty"`
	DatasetID    string      `json:"dataset_id,omitempty"`
	ReferenceSQL string      `json:"reference_sql,omitempty"`
	Skills       []TaskSkill `json:"skills,omitempty"`
}

type AttemptExecutionContext struct {
	AttemptID        string       `json:"attempt_id"`
	AttemptNumber    int          `json:"attempt_number"`
	ExecutionSuccess bool         `json:"execution_success"`
	IsCorrect        bool         `json:"is_correct"`
	SubmittedSQL     string       `json:"submitted_sql,omitempty"`
	QueryHash        string       `json:"query_hash,omitempty"`
	ExecutionTimeMS  int64        `json:"execution_time_ms,omitempty"`
	RowCount         int          `json:"row_count,omitempty"`
	ErrorType        string       `json:"error_type,omitempty"`
	ErrorMessage     string       `json:"error_message,omitempty"`
	ActualResult     *QueryResult `json:"actual_result,omitempty"`
	ExpectedResult   *QueryResult `json:"expected_result,omitempty"`
}

type HintContext struct {
	HintRequested bool   `json:"hint_requested"`
	HintUsed      bool   `json:"hint_used"`
	HintID        string `json:"hint_id,omitempty"`
	HintType      string `json:"hint_type,omitempty"`
	HintsCount    int    `json:"hints_count,omitempty"`
}

type AffectedSkill struct {
	SkillID   string  `json:"skill_id"`
	SkillCode string  `json:"skill_code,omitempty"`
	Weight    float64 `json:"weight"`
	Delta     float64 `json:"delta"`
}

type TaskCheckedEvent struct {
	EventID          string                  `json:"event_id"`
	EventVersion     int                     `json:"event_version"`
	EventType        string                  `json:"event_type"`
	UserID           string                  `json:"user_id"`
	TaskID           string                  `json:"task_id"`
	TaskTitle        string                  `json:"task_title,omitempty"`
	TaskDescription  string                  `json:"task_description,omitempty"`
	Difficulty       string                  `json:"difficulty,omitempty"`
	DatasetID        string                  `json:"dataset_id,omitempty"`
	ReferenceSQL     string                  `json:"reference_sql,omitempty"`
	AttemptID        string                  `json:"attempt_id"`
	AttemptNumber    int                     `json:"attempt_number"`
	ExecutionSuccess bool                    `json:"execution_success"`
	IsCorrect        bool                    `json:"is_correct"`
	ExecutionTimeMS  int64                   `json:"execution_time_ms,omitempty"`
	RowCount         int                     `json:"row_count,omitempty"`
	ErrorType        string                  `json:"error_type,omitempty"`
	ErrorMessage     string                  `json:"error_message,omitempty"`
	QueryHash        string                  `json:"query_hash,omitempty"`
	SubmittedSQL     string                  `json:"submitted_sql,omitempty"`
	ActualResult     *QueryResult            `json:"actual_result,omitempty"`
	ExpectedResult   *QueryResult            `json:"expected_result,omitempty"`
	Hint             HintContext             `json:"hint"`
	Task             TaskContext             `json:"task"`
	Execution        AttemptExecutionContext `json:"execution"`
	AffectedSkills   []AffectedSkill         `json:"affected_skills"`
	SourceEventID    string                  `json:"source_event_id,omitempty"`
	CreatedAt        time.Time               `json:"created_at"`
}

type TaskCompletedEvent struct {
	EventID         string                  `json:"event_id"`
	EventVersion    int                     `json:"event_version"`
	EventType       string                  `json:"event_type"`
	UserID          string                  `json:"user_id"`
	TaskID          string                  `json:"task_id"`
	TaskTitle       string                  `json:"task_title,omitempty"`
	TaskDescription string                  `json:"task_description,omitempty"`
	Difficulty      string                  `json:"difficulty,omitempty"`
	DatasetID       string                  `json:"dataset_id,omitempty"`
	ReferenceSQL    string                  `json:"reference_sql,omitempty"`
	AttemptID       string                  `json:"attempt_id"`
	AttemptNumber   int                     `json:"attempt_number"`
	ExecutionTimeMS int64                   `json:"execution_time_ms,omitempty"`
	QueryHash       string                  `json:"query_hash,omitempty"`
	SubmittedSQL    string                  `json:"submitted_sql,omitempty"`
	ActualResult    *QueryResult            `json:"actual_result,omitempty"`
	ExpectedResult  *QueryResult            `json:"expected_result,omitempty"`
	Hint            HintContext             `json:"hint"`
	Task            TaskContext             `json:"task"`
	Execution       AttemptExecutionContext `json:"execution"`
	AffectedSkills  []AffectedSkill         `json:"affected_skills"`
	GraphState      *GraphStateSnapshot     `json:"graph_state,omitempty"`
	AnalysisState   *UserAnalysisState      `json:"analysis_state,omitempty"`
	SourceEventID   string                  `json:"source_event_id,omitempty"`
	CreatedAt       time.Time               `json:"created_at"`
}

type TaskRecommendedEvent struct {
	EventID            string                    `json:"event_id"`
	EventVersion       int                       `json:"event_version"`
	EventType          string                    `json:"event_type"`
	UserID             string                    `json:"user_id"`
	TaskID             string                    `json:"task_id"`
	GraphID            string                    `json:"graph_id,omitempty"`
	GraphCode          string                    `json:"graph_code"`
	ProfessionalTrack  string                    `json:"professional_track"`
	Score              float64                   `json:"score"`
	Reason             string                    `json:"reason"`
	RepeatMode         bool                      `json:"repeat_mode"`
	CandidateSource    string                    `json:"candidate_source"`
	NewCandidates      int                       `json:"new_candidates"`
	RecallCandidates   int                       `json:"recall_candidates"`
	FallbackCandidates int                       `json:"fallback_candidates"`
	RecommendedSkills  []UserSkillRecommendation `json:"recommended_skills,omitempty"`
	CreatedAt          time.Time                 `json:"created_at"`
}

type LearnerModelUpdatedEvent struct {
	EventID         string            `json:"event_id"`
	EventVersion    int               `json:"event_version"`
	EventType       string            `json:"event_type"`
	UserID          string            `json:"user_id"`
	SkillID         string            `json:"skill_id"`
	SkillCode       string            `json:"skill_code,omitempty"`
	OldMasteryScore float64           `json:"old_mastery_score"`
	NewMasteryScore float64           `json:"new_mastery_score"`
	OldConfidence   float64           `json:"old_confidence"`
	NewConfidence   float64           `json:"new_confidence"`
	Source          string            `json:"source"`
	Reason          string            `json:"reason,omitempty"`
	Analysis        *AnalysisMetadata `json:"analysis,omitempty"`
	CreatedAt       time.Time         `json:"created_at"`
}

type LearningGraph struct {
	ID          string `json:"id"`
	Code        string `json:"code"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	IsActive    bool   `json:"is_active"`
}

type UserLearningProfile struct {
	UserID               string    `json:"user_id"`
	GraphID              string    `json:"graph_id"`
	GraphCode            string    `json:"graph_code,omitempty"`
	ProfessionalTrack    string    `json:"professional_track"`
	RecommendationReason string    `json:"recommendation_reason,omitempty"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type GraphStateSnapshot struct {
	UserID                string                    `json:"user_id"`
	Profile               UserLearningProfile       `json:"profile"`
	Graph                 LearningGraph             `json:"graph"`
	CompletedTasks        int                       `json:"completed_tasks"`
	TotalAttempts         int                       `json:"total_attempts"`
	AverageMasteryScore   float64                   `json:"average_mastery_score"`
	AverageEffectiveScore float64                   `json:"average_effective_score"`
	AverageThresholdGap   float64                   `json:"average_threshold_gap"`
	MasteredSkillsCount   int                       `json:"mastered_skills_count"`
	WeakSkillsCount       int                       `json:"weak_skills_count"`
	RecallSkillsCount     int                       `json:"recall_skills_count"`
	Skills                []GraphSkillState         `json:"skills"`
	Dependencies          []GraphDependencyState    `json:"dependencies,omitempty"`
	RecommendedSkills     []UserSkillRecommendation `json:"recommended_skills,omitempty"`
	CreatedAt             time.Time                 `json:"created_at"`
}

type GraphSkillState struct {
	SkillID            string     `json:"skill_id"`
	SkillCode          string     `json:"skill_code"`
	SkillName          string     `json:"skill_name,omitempty"`
	Position           int        `json:"position"`
	IsRequired         bool       `json:"is_required"`
	PriorityWeight     float64    `json:"priority_weight"`
	MasteryThreshold   float64    `json:"mastery_threshold"`
	MasteryScore       float64    `json:"mastery_score"`
	Confidence         float64    `json:"confidence"`
	AttemptsCount      int        `json:"attempts_count"`
	SuccessCount       int        `json:"success_count"`
	Retention          float64    `json:"retention"`
	ForgettingPriority float64    `json:"forgetting_priority"`
	EffectiveMastery   float64    `json:"effective_mastery"`
	ThresholdGap       float64    `json:"threshold_gap"`
	Mastered           bool       `json:"mastered"`
	LastUsedAt         *time.Time `json:"last_used_at,omitempty"`
	UpdatedAt          *time.Time `json:"updated_at,omitempty"`
}

type GraphDependencyState struct {
	GraphID            string   `json:"graph_id"`
	SkillID            string   `json:"skill_id"`
	SkillCode          string   `json:"skill_code,omitempty"`
	DependsOnSkillID   string   `json:"depends_on_skill_id"`
	DependsOnSkillCode string   `json:"depends_on_skill_code,omitempty"`
	Strength           float64  `json:"strength"`
	RequiredMastery    *float64 `json:"required_mastery,omitempty"`
}

type UserSkillRecommendation struct {
	UserID            string     `json:"user_id,omitempty"`
	GraphID           string     `json:"graph_id,omitempty"`
	SkillID           string     `json:"skill_id"`
	SkillCode         string     `json:"skill_code,omitempty"`
	Priority          float64    `json:"priority"`
	RecommendedAction string     `json:"recommended_action,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	SourceEventID     string     `json:"source_event_id,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
	UpdatedAt         time.Time  `json:"updated_at"`
}

type UserAnalysisState struct {
	UserID           string     `json:"user_id"`
	PendingTaskID    string     `json:"pending_task_id,omitempty"`
	PendingAttemptID string     `json:"pending_attempt_id,omitempty"`
	Status           string     `json:"status"`
	PendingSince     time.Time  `json:"pending_since"`
	WaitUntil        *time.Time `json:"wait_until,omitempty"`
	SourceEventID    string     `json:"source_event_id,omitempty"`
	UpdatedAt        time.Time  `json:"updated_at"`
}

// Outgoing event to task-progress.
type AnalyticsSkillAssessmentUpdatedEvent struct {
	EventID              string             `json:"event_id"`
	EventVersion         int                `json:"event_version"`
	EventType            string             `json:"event_type"`
	UserID               string             `json:"user_id"`
	Source               string             `json:"source,omitempty"`
	GraphID              string             `json:"graph_id,omitempty"`
	GraphCode            string             `json:"graph_code,omitempty"`
	ProfessionalTrack    string             `json:"professional_track,omitempty"`
	RecommendationReason string             `json:"recommendation_reason,omitempty"`
	SourceTaskID         string             `json:"source_task_id,omitempty"`
	SourceAttemptID      string             `json:"source_attempt_id,omitempty"`
	ObservedUntil        *time.Time         `json:"observed_until,omitempty"`
	AssessmentMode       string             `json:"assessment_mode,omitempty"`
	Analysis             *AnalysisMetadata  `json:"analysis,omitempty"`
	UserGraphOverlay     *GraphPatch        `json:"user_graph_overlay,omitempty"`
	RecommendedSkills    []RecommendedSkill `json:"recommended_skills,omitempty"`
	SkillScores          []SkillAssessment  `json:"skill_scores"`
	CreatedAt            time.Time          `json:"created_at"`
}

type CareerRecommendationCreatedEvent struct {
	EventID              string              `json:"event_id"`
	EventVersion         int                 `json:"event_version"`
	EventType            string              `json:"event_type"`
	UserID               string              `json:"user_id"`
	SourceTaskID         string              `json:"source_task_id,omitempty"`
	SourceAttemptID      string              `json:"source_attempt_id,omitempty"`
	Analysis             *AnalysisMetadata   `json:"analysis,omitempty"`
	PrimaryTrack         string              `json:"primary_track"`
	PrimaryTitle         string              `json:"primary_title"`
	Score                float64             `json:"score"`
	Alternatives         []CareerAlternative `json:"alternatives,omitempty"`
	StrongSkills         []string            `json:"strong_skills,omitempty"`
	WeakSkills           []string            `json:"weak_skills,omitempty"`
	RecommendedGraphCode string              `json:"recommended_graph_code,omitempty"`
	Explanation          string              `json:"explanation"`
	CreatedAt            time.Time           `json:"created_at"`
}

type CareerAlternative struct {
	Track string  `json:"track"`
	Title string  `json:"title"`
	Score float64 `json:"score"`
}

type AnalysisMetadata struct {
	AnalysisRunID string   `json:"analysis_run_id,omitempty"`
	ModelVersion  string   `json:"model_version,omitempty"`
	PromptVersion string   `json:"prompt_version,omitempty"`
	Seed          *int64   `json:"seed,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
}

type GraphPatch struct {
	GraphID           string                 `json:"graph_id,omitempty"`
	GraphCode         string                 `json:"graph_code,omitempty"`
	Code              string                 `json:"code,omitempty"`
	ProfessionalTrack string                 `json:"professional_track,omitempty"`
	Skills            []GraphPatchSkill      `json:"skills,omitempty"`
	Dependencies      []GraphPatchDependency `json:"dependencies,omitempty"`
}

type GraphPatchSkill struct {
	SkillID          string   `json:"skill_id,omitempty"`
	SkillCode        string   `json:"skill_code,omitempty"`
	Position         int      `json:"position,omitempty"`
	IsRequired       *bool    `json:"is_required,omitempty"`
	PriorityWeight   *float64 `json:"priority_weight,omitempty"`
	MasteryThreshold *float64 `json:"mastery_threshold,omitempty"`
	Reason           string   `json:"reason,omitempty"`
}

type GraphPatchDependency struct {
	SkillID            string   `json:"skill_id,omitempty"`
	SkillCode          string   `json:"skill_code,omitempty"`
	DependsOnSkillID   string   `json:"depends_on_skill_id,omitempty"`
	DependsOnSkillCode string   `json:"depends_on_skill_code,omitempty"`
	Strength           *float64 `json:"strength,omitempty"`
	RequiredMastery    *float64 `json:"required_mastery,omitempty"`
	Reason             string   `json:"reason,omitempty"`
}

type RecommendedSkill struct {
	SkillID           string     `json:"skill_id,omitempty"`
	SkillCode         string     `json:"skill_code,omitempty"`
	Priority          float64    `json:"priority"`
	RecommendedAction string     `json:"recommended_action,omitempty"`
	Reason            string     `json:"reason,omitempty"`
	ExpiresAt         *time.Time `json:"expires_at,omitempty"`
}

type SkillAssessment struct {
	SkillID      string                `json:"skill_id,omitempty"`
	SkillCode    string                `json:"skill_code,omitempty"`
	MasteryScore *float64              `json:"mastery_score,omitempty"`
	Delta        *float64              `json:"delta,omitempty"`
	Confidence   *float64              `json:"confidence,omitempty"`
	Reason       string                `json:"reason,omitempty"`
	Components   *AssessmentComponents `json:"components,omitempty"`
}

type AssessmentComponents struct {
	Correctness  *float64 `json:"correctness,omitempty"`
	Independence *float64 `json:"independence,omitempty"`
	Efficiency   *float64 `json:"efficiency,omitempty"`
}
