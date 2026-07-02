package httptransport

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"task-progress-service/internal/domain"
)

type errorResponse struct {
	Error string `json:"error"`
}

type analysisPendingResponse struct {
	Status            string                    `json:"status"`
	Error             string                    `json:"error"`
	PendingTaskID     string                    `json:"pending_task_id,omitempty"`
	PendingAttemptID  string                    `json:"pending_attempt_id,omitempty"`
	PendingSince      time.Time                 `json:"pending_since"`
	WaitUntil         *time.Time                `json:"wait_until,omitempty"`
	RetryAfterSeconds int                       `json:"retry_after_seconds"`
	AnalysisState     *domain.UserAnalysisState `json:"analysis_state,omitempty"`
}

func writeJSON(w http.ResponseWriter, status int, payload any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(payload)
}

func writeError(w http.ResponseWriter, err error) {
	var pendingErr *domain.AnalysisPendingError
	if errors.As(err, &pendingErr) {
		if pendingErr.RetryAfterSeconds > 0 {
			w.Header().Set("Retry-After", formatRetryAfter(pendingErr.RetryAfterSeconds))
		}
		state := pendingErr.State
		writeJSON(w, http.StatusAccepted, analysisPendingResponse{
			Status:            "pending",
			Error:             domain.ErrAnalysisPending.Error(),
			PendingTaskID:     state.PendingTaskID,
			PendingAttemptID:  state.PendingAttemptID,
			PendingSince:      state.PendingSince,
			WaitUntil:         state.WaitUntil,
			RetryAfterSeconds: pendingErr.RetryAfterSeconds,
			AnalysisState:     &state,
		})
		return
	}

	status := http.StatusInternalServerError
	code := "internal_error"
	switch {
	case errors.Is(err, domain.ErrInvalidInput):
		status = http.StatusBadRequest
		code = domain.ErrInvalidInput.Error()
	case errors.Is(err, domain.ErrUnauthorized):
		status = http.StatusUnauthorized
		code = domain.ErrUnauthorized.Error()
	case errors.Is(err, domain.ErrForbidden):
		status = http.StatusForbidden
		code = domain.ErrForbidden.Error()
	case errors.Is(err, domain.ErrNotFound):
		status = http.StatusNotFound
		code = domain.ErrNotFound.Error()
	case errors.Is(err, domain.ErrAnalysisPending):
		status = http.StatusAccepted
		code = domain.ErrAnalysisPending.Error()
	case errors.Is(err, domain.ErrConflict):
		status = http.StatusConflict
		code = domain.ErrConflict.Error()
	case errors.Is(err, domain.ErrHintUnavailable):
		status = http.StatusConflict
		code = domain.ErrHintUnavailable.Error()
	case errors.Is(err, domain.ErrHintGenerationFailed):
		status = http.StatusGatewayTimeout
		code = domain.ErrHintGenerationFailed.Error()
	}
	writeJSON(w, status, errorResponse{Error: code})
}

func formatRetryAfter(seconds int) string {
	if seconds <= 0 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
