package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidInput         = errors.New("invalid_input")
	ErrUnauthorized         = errors.New("unauthorized")
	ErrForbidden            = errors.New("forbidden")
	ErrNotFound             = errors.New("not_found")
	ErrConflict             = errors.New("conflict")
	ErrAnalysisPending      = errors.New("analysis_pending")
	ErrDuplicateEvent       = errors.New("duplicate_event")
	ErrStaleAssessment      = errors.New("stale_assessment")
	ErrHintUnavailable      = errors.New("hint_unavailable")
	ErrHintGenerationFailed = errors.New("hint_generation_failed")
)

// AnalysisPendingError carries enough context for clients/API Gateway to poll
// /tasks/next without losing the pending LLM/analytics state.
type AnalysisPendingError struct {
	State             UserAnalysisState `json:"analysis_state"`
	RetryAfterSeconds int               `json:"retry_after_seconds"`
}

func (e *AnalysisPendingError) Error() string {
	if e == nil {
		return ErrAnalysisPending.Error()
	}
	if e.State.WaitUntil != nil {
		return fmt.Sprintf("%s: retry after %ds, wait until %s", ErrAnalysisPending, e.RetryAfterSeconds, e.State.WaitUntil.Format(time.RFC3339))
	}
	return fmt.Sprintf("%s: retry after %ds", ErrAnalysisPending, e.RetryAfterSeconds)
}

func (e *AnalysisPendingError) Unwrap() error {
	return ErrAnalysisPending
}
