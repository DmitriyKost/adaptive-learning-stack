package domain

import "time"

const (
	RoleStudent = "student"
	RoleAdmin   = "admin"
)

type AccessClaims struct {
	UserID    string `json:"user_id"`
	Role      string `json:"role"`
	TokenType string `json:"typ"`
	IssuedAt  int64  `json:"iat"`
	ExpiresAt int64  `json:"exp"`
}

type ExecuteRequest struct {
	TaskID    string `json:"task_id"`
	UserQuery string `json:"user_query"`
}

type ExecuteResponse struct {
	EventID          string         `json:"event_id"`
	UserID           string         `json:"user_id"`
	TaskID           string         `json:"task_id"`
	ExecutionSuccess bool           `json:"execution_success"`
	IsCorrect        *bool          `json:"is_correct,omitempty"`
	UserResult       *ExecuteResult `json:"user_result"`
	ReferenceResult  *ExecuteResult `json:"reference_result,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}

type ExecuteResult struct {
	Columns      []string         `json:"columns,omitempty"`
	Rows         []map[string]any `json:"rows,omitempty"`
	RowsAffected int64            `json:"rows_affected"`
	QueryTimeMs  int64            `json:"query_time_ms"`
	Truncated    bool             `json:"truncated"`
	Error        *QueryError      `json:"error,omitempty"`
}

type QueryError struct {
	Type    string `json:"type"`
	Message string `json:"message"`
}

type Workspace struct {
	UserID     string
	SchemaName string
	RoleName   string
}

type ExecutionEvent struct {
	EventID          string         `json:"event_id"`
	EventType        string         `json:"event_type"`
	EventVersion     int            `json:"event_version"`
	UserID           string         `json:"user_id"`
	TaskID           string         `json:"task_id"`
	UserQuery        string         `json:"user_query"`
	ReferenceQuery   string         `json:"reference_query,omitempty"`
	ExecutionSuccess bool           `json:"execution_success"`
	IsCorrect        *bool          `json:"is_correct,omitempty"`
	UserResult       *ExecuteResult `json:"user_result"`
	ReferenceResult  *ExecuteResult `json:"reference_result,omitempty"`
	CreatedAt        time.Time      `json:"created_at"`
}
