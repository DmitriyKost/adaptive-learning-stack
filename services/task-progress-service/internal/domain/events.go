package domain

import "time"

const (
	EventTypePlaygroundExecutionCompleted    = "playground.execution.completed"
	EventTypeSQLExecutionCompleted           = "sql.execution.completed" // backward-compatible alias from early playground-service versions
	EventTypeTaskChecked                     = "task.checked"
	EventTypeTaskCompleted                   = "task.completed"
	EventTypeTaskRecommended                 = "task.recommended"
	EventTypeLearnerModelUpdated             = "learner_model.updated"
	EventTypeAnalyticsSkillAssessmentUpdated = "analytics.skill_assessment.updated"
)

type QueryResult struct {
	Columns         []string `json:"columns,omitempty"`
	Rows            [][]any  `json:"rows,omitempty"`
	RowCount        int      `json:"row_count,omitempty"`
	Truncated       bool     `json:"truncated,omitempty"`
	ExecutionTimeMS int64    `json:"execution_time_ms,omitempty"`
	QueryTimeMS     int64    `json:"query_time_ms,omitempty"`
}

type PlaygroundQueryError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type PlaygroundQueryResult struct {
	Columns         []string              `json:"columns,omitempty"`
	Rows            []map[string]any      `json:"rows,omitempty"`
	RowsAffected    int64                 `json:"rows_affected,omitempty"`
	QueryTimeMS     int64                 `json:"query_time_ms,omitempty"`
	ExecutionTimeMS int64                 `json:"execution_time_ms,omitempty"`
	Truncated       bool                  `json:"truncated,omitempty"`
	Error           *PlaygroundQueryError `json:"error,omitempty"`
}

type PlaygroundExecutionCompletedEvent struct {
	EventID      string `json:"event_id"`
	EventVersion int    `json:"event_version"`
	EventType    string `json:"event_type"`
	UserID       string `json:"user_id"`
	TaskID       string `json:"task_id"`
	AttemptID    string `json:"attempt_id,omitempty"`
	DatasetID    string `json:"dataset_id,omitempty"`

	// success is emitted by playground-service when the SQL was executed without runtime errors.
	Success          bool                   `json:"success"`
	ExecutionSuccess bool                   `json:"execution_success,omitempty"`
	ExecutionTimeMS  int64                  `json:"execution_time_ms"`
	RowCount         int                    `json:"row_count"`
	Truncated        bool                   `json:"truncated"`
	ErrorType        string                 `json:"error_type,omitempty"`
	ErrorMessage     string                 `json:"error_message,omitempty"`
	QueryHash        string                 `json:"query_hash,omitempty"`
	SubmittedSQL     string                 `json:"submitted_sql,omitempty"`
	UserQuery        string                 `json:"user_query,omitempty"`
	UserResult       *PlaygroundQueryResult `json:"user_result,omitempty"`
	ReferenceResult  *PlaygroundQueryResult `json:"reference_result,omitempty"`

	HintRequested bool   `json:"hint_requested,omitempty"`
	HintUsed      bool   `json:"hint_used,omitempty"`
	HintID        string `json:"hint_id,omitempty"`
	HintType      string `json:"hint_type,omitempty"`
	HintsCount    int    `json:"hints_count,omitempty"`

	// task-progress can verify correctness using either an explicit is_correct flag
	// or actual/expected tabular results. The latter matches the first prototype shape.
	IsCorrect       *bool        `json:"is_correct,omitempty"`
	Columns         []string     `json:"columns,omitempty"`
	Rows            [][]any      `json:"rows,omitempty"`
	ExpectedColumns []string     `json:"expected_columns,omitempty"`
	ExpectedRows    [][]any      `json:"expected_rows,omitempty"`
	ExecuteResult   *QueryResult `json:"execute_result,omitempty"`
	ExpectedResult  *QueryResult `json:"expected_result,omitempty"`

	CreatedAt time.Time `json:"created_at"`
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

type TaskCheckedEvent struct {
	EventID      string `json:"event_id"`
	EventVersion int    `json:"event_version"`
	EventType    string `json:"event_type"`
	UserID       string `json:"user_id"`

	TaskID          string `json:"task_id"`
	TaskTitle       string `json:"task_title,omitempty"`
	TaskDescription string `json:"task_description,omitempty"`
	Difficulty      string `json:"difficulty,omitempty"`
	DatasetID       string `json:"dataset_id,omitempty"`
	ReferenceSQL    string `json:"reference_sql,omitempty"`

	AttemptID        string `json:"attempt_id"`
	AttemptNumber    int    `json:"attempt_number"`
	ExecutionSuccess bool   `json:"execution_success"`
	IsCorrect        bool   `json:"is_correct"`
	ExecutionTimeMS  int64  `json:"execution_time_ms,omitempty"`
	RowCount         int    `json:"row_count,omitempty"`
	ErrorType        string `json:"error_type,omitempty"`
	ErrorMessage     string `json:"error_message,omitempty"`
	QueryHash        string `json:"query_hash,omitempty"`
	SubmittedSQL     string `json:"submitted_sql,omitempty"`

	ActualResult   *QueryResult            `json:"actual_result,omitempty"`
	ExpectedResult *QueryResult            `json:"expected_result,omitempty"`
	Hint           HintContext             `json:"hint"`
	Task           TaskContext             `json:"task"`
	Execution      AttemptExecutionContext `json:"execution"`
	AffectedSkills []AffectedSkill         `json:"affected_skills"`
	SourceEventID  string                  `json:"source_event_id,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
}

type TaskCompletedEvent struct {
	EventID      string `json:"event_id"`
	EventVersion int    `json:"event_version"`
	EventType    string `json:"event_type"`
	UserID       string `json:"user_id"`

	TaskID          string `json:"task_id"`
	TaskTitle       string `json:"task_title,omitempty"`
	TaskDescription string `json:"task_description,omitempty"`
	Difficulty      string `json:"difficulty,omitempty"`
	DatasetID       string `json:"dataset_id,omitempty"`
	ReferenceSQL    string `json:"reference_sql,omitempty"`

	AttemptID       string `json:"attempt_id"`
	AttemptNumber   int    `json:"attempt_number"`
	ExecutionTimeMS int64  `json:"execution_time_ms,omitempty"`
	QueryHash       string `json:"query_hash,omitempty"`
	SubmittedSQL    string `json:"submitted_sql,omitempty"`

	ActualResult   *QueryResult            `json:"actual_result,omitempty"`
	ExpectedResult *QueryResult            `json:"expected_result,omitempty"`
	Hint           HintContext             `json:"hint"`
	Task           TaskContext             `json:"task"`
	Execution      AttemptExecutionContext `json:"execution"`
	AffectedSkills []AffectedSkill         `json:"affected_skills"`
	GraphState     *GraphStateSnapshot     `json:"graph_state,omitempty"`
	AnalysisState  *UserAnalysisState      `json:"analysis_state,omitempty"`
	SourceEventID  string                  `json:"source_event_id,omitempty"`
	CreatedAt      time.Time               `json:"created_at"`
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

type AffectedSkill struct {
	SkillID   string  `json:"skill_id"`
	SkillCode string  `json:"skill_code,omitempty"`
	Weight    float64 `json:"weight"`
	Delta     float64 `json:"delta"`
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

type AnalyticsSkillAssessmentUpdatedEvent struct {
	EventID              string            `json:"event_id"`
	EventVersion         int               `json:"event_version"`
	EventType            string            `json:"event_type"`
	UserID               string            `json:"user_id"`
	Source               string            `json:"source,omitempty"`
	GraphID              string            `json:"graph_id,omitempty"`
	GraphCode            string            `json:"graph_code,omitempty"`
	ProfessionalTrack    string            `json:"professional_track,omitempty"`
	RecommendationReason string            `json:"recommendation_reason,omitempty"`
	SourceTaskID         string            `json:"source_task_id,omitempty"`
	SourceAttemptID      string            `json:"source_attempt_id,omitempty"`
	ObservedUntil        *time.Time        `json:"observed_until,omitempty"`
	AssessmentMode       string            `json:"assessment_mode,omitempty"` // replace, delta, merge
	Analysis             *AnalysisMetadata `json:"analysis,omitempty"`

	// graph_patch is kept as a backward-compatible JSON field, but task-progress treats
	// it as a user-specific overlay. Reference graphs are only changed by admin API.
	GraphPatch        *GraphPatch        `json:"graph_patch,omitempty"`
	UserGraphOverlay  *GraphPatch        `json:"user_graph_overlay,omitempty"`
	RecommendedSkills []RecommendedSkill `json:"recommended_skills,omitempty"`
	SkillScores       []SkillAssessment  `json:"skill_scores"`
	CreatedAt         time.Time          `json:"created_at"`
}

type AnalysisMetadata struct {
	AnalysisRunID string   `json:"analysis_run_id,omitempty"`
	ModelVersion  string   `json:"model_version,omitempty"`
	PromptVersion string   `json:"prompt_version,omitempty"`
	Seed          *int64   `json:"seed,omitempty"`
	Temperature   *float64 `json:"temperature,omitempty"`
}

// GraphPatch represents user-specific graph overrides from analytics/LLM. It never
// mutates reference learning_graphs, graph_skills or skill_dependencies; only per-user
// override tables are changed. Skill references must already exist in the reference catalog.
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
