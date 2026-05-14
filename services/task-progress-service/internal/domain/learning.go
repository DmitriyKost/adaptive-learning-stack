package domain

import "time"

const (
	DifficultyEasy   = "easy"
	DifficultyMedium = "medium"
	DifficultyHard   = "hard"

	TaskStatusNotStarted = "not_started"
	TaskStatusInProgress = "in_progress"
	TaskStatusSolved     = "solved"

	AnalysisStatusPending   = "pending"
	AnalysisStatusCompleted = "completed"
	AnalysisStatusExpired   = "expired"
	AnalysisStatusFailed    = "failed"

	NextTaskStatusPending = "pending"
	NextTaskStatusReady   = "ready"
	NextTaskStatusFailed  = "failed"
)

type Skill struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	Domain      string    `json:"domain,omitempty"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type LearningGraph struct {
	ID          string    `json:"id"`
	Code        string    `json:"code"`
	Name        string    `json:"name"`
	Description string    `json:"description,omitempty"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type GraphSkill struct {
	GraphID          string    `json:"graph_id"`
	SkillID          string    `json:"skill_id"`
	SkillCode        string    `json:"skill_code"`
	SkillName        string    `json:"skill_name,omitempty"`
	Position         int       `json:"position"`
	IsRequired       bool      `json:"is_required"`
	PriorityWeight   float64   `json:"priority_weight"`
	MasteryThreshold float64   `json:"mastery_threshold"`
	CreatedAt        time.Time `json:"created_at"`
}

type SkillDependency struct {
	GraphID            string    `json:"graph_id"`
	SkillID            string    `json:"skill_id"`
	SkillCode          string    `json:"skill_code,omitempty"`
	DependsOnSkillID   string    `json:"depends_on_skill_id"`
	DependsOnSkillCode string    `json:"depends_on_skill_code,omitempty"`
	Strength           float64   `json:"strength"`
	RequiredMastery    *float64  `json:"required_mastery,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

type Task struct {
	ID           string      `json:"id"`
	Title        string      `json:"title"`
	Description  string      `json:"description"`
	Difficulty   string      `json:"difficulty"`
	ReferenceSQL string      `json:"-"`
	DatasetID    string      `json:"dataset_id,omitempty"`
	IsActive     bool        `json:"is_active"`
	Skills       []TaskSkill `json:"skills,omitempty"`
	CreatedAt    time.Time   `json:"created_at"`
	UpdatedAt    time.Time   `json:"updated_at"`
}

type TaskSkill struct {
	TaskID    string  `json:"task_id"`
	SkillID   string  `json:"skill_id"`
	SkillCode string  `json:"skill_code,omitempty"`
	Weight    float64 `json:"weight"`
}

type UserLearningProfile struct {
	UserID               string    `json:"user_id"`
	GraphID              string    `json:"graph_id"`
	GraphCode            string    `json:"graph_code"`
	ProfessionalTrack    string    `json:"professional_track"`
	RecommendationReason string    `json:"recommendation_reason,omitempty"`
	CreatedAt            time.Time `json:"created_at"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type UserSkill struct {
	UserID        string     `json:"user_id"`
	SkillID       string     `json:"skill_id"`
	SkillCode     string     `json:"skill_code,omitempty"`
	SkillName     string     `json:"skill_name,omitempty"`
	MasteryScore  float64    `json:"mastery_score"`
	Confidence    float64    `json:"confidence"`
	AttemptsCount int        `json:"attempts_count"`
	SuccessCount  int        `json:"success_count"`
	LastUsedAt    *time.Time `json:"last_used_at,omitempty"`
	UpdatedAt     time.Time  `json:"updated_at"`
}

type UserSkillView struct {
	UserSkill
	Retention           float64 `json:"retention"`
	ForgettingPriority  float64 `json:"forgetting_priority"`
	EffectiveMastery    float64 `json:"effective_mastery"`
	MasteryThreshold    float64 `json:"mastery_threshold"`
	GraphPriorityWeight float64 `json:"graph_priority_weight"`
	GraphRequired       bool    `json:"graph_required"`
}

type TaskAttempt struct {
	ID               string    `json:"id"`
	UserID           string    `json:"user_id"`
	TaskID           string    `json:"task_id"`
	SubmittedSQL     string    `json:"submitted_sql,omitempty"`
	ExecutionSuccess bool      `json:"execution_success"`
	IsCorrect        bool      `json:"is_correct"`
	ExecutionTimeMS  int64     `json:"execution_time_ms"`
	RowCount         int       `json:"row_count"`
	ErrorType        string    `json:"error_type,omitempty"`
	ErrorMessage     string    `json:"error_message,omitempty"`
	QueryHash        string    `json:"query_hash,omitempty"`
	CreatedAt        time.Time `json:"created_at"`
}

type UserTaskStatus struct {
	UserID        string     `json:"user_id"`
	TaskID        string     `json:"task_id"`
	Status        string     `json:"status"`
	AttemptsCount int        `json:"attempts_count"`
	SolvedAt      *time.Time `json:"solved_at,omitempty"`
	LastAttemptAt time.Time  `json:"last_attempt_at"`
	LastAttemptID string     `json:"last_attempt_id"`
}

type NextTaskRecommendation struct {
	Task              Task                      `json:"task"`
	Reason            string                    `json:"reason"`
	Score             float64                   `json:"score"`
	GraphCode         string                    `json:"graph_code"`
	Track             string                    `json:"professional_track"`
	RepeatMode        bool                      `json:"repeat_mode"`
	RecommendedSkills []UserSkillRecommendation `json:"recommended_skills,omitempty"`
}

type UserTrajectoryRecommendation struct {
	ID         string    `json:"id"`
	UserID     string    `json:"user_id"`
	TaskID     string    `json:"task_id"`
	GraphID    string    `json:"graph_id"`
	Reason     string    `json:"reason"`
	Score      float64   `json:"score"`
	RepeatMode bool      `json:"repeat_mode"`
	Status     string    `json:"status"`
	CreatedAt  time.Time `json:"created_at"`
}

type NextTaskCacheEntry struct {
	UserID              string                    `json:"user_id"`
	Status              string                    `json:"status"`
	TaskID              string                    `json:"task_id,omitempty"`
	GraphID             string                    `json:"graph_id,omitempty"`
	Reason              string                    `json:"reason,omitempty"`
	Score               float64                   `json:"score"`
	GraphCode           string                    `json:"graph_code,omitempty"`
	ProfessionalTrack   string                    `json:"professional_track,omitempty"`
	RepeatMode          bool                      `json:"repeat_mode"`
	RecommendedSkills   []UserSkillRecommendation `json:"recommended_skills,omitempty"`
	SourceEventID       string                    `json:"source_event_id,omitempty"`
	SourceAttemptID     string                    `json:"source_attempt_id,omitempty"`
	SourceAnalysisRunID string                    `json:"source_analysis_run_id,omitempty"`
	WaitUntil           *time.Time                `json:"wait_until,omitempty"`
	ExpiresAt           *time.Time                `json:"expires_at,omitempty"`
	CreatedAt           time.Time                 `json:"created_at"`
	UpdatedAt           time.Time                 `json:"updated_at"`
}

type ProgressSummary struct {
	UserID                string                    `json:"user_id"`
	Profile               UserLearningProfile       `json:"profile"`
	CompletedTasks        int                       `json:"completed_tasks"`
	TotalAttempts         int                       `json:"total_attempts"`
	AverageMasteryScore   float64                   `json:"average_mastery_score"`
	AverageEffectiveScore float64                   `json:"average_effective_score"`
	Skills                []UserSkillView           `json:"skills"`
	WeakSkills            []UserSkillView           `json:"weak_skills"`
	RecallSkills          []UserSkillView           `json:"recall_skills"`
	RecommendedSkills     []UserSkillRecommendation `json:"recommended_skills,omitempty"`
	AnalysisState         *UserAnalysisState        `json:"analysis_state,omitempty"`
}

// GraphStateSnapshot is an aggregate, analytics-facing view of the user's current
// state inside the active learning graph. It is not persisted by task-progress;
// it is emitted after successful task completion so analytics/LLM modules can
// use current graph context without querying this service synchronously.
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
