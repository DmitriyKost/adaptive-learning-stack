package service

import (
	"context"
	"log/slog"
	"strings"

	"task-progress-service/internal/domain"
)

type PlaygroundEventHandler struct {
	progress *ProgressUsecase
	log      *slog.Logger
}

func NewPlaygroundEventHandler(progress *ProgressUsecase, log *slog.Logger) *PlaygroundEventHandler {
	return &PlaygroundEventHandler{progress: progress, log: log}
}

func (h *PlaygroundEventHandler) HandleExecutionCompleted(ctx context.Context, event domain.PlaygroundExecutionCompletedEvent) error {
	if event.EventID == "" || event.UserID == "" || event.TaskID == "" {
		return domain.ErrInvalidInput
	}
	if event.EventType == "" {
		event.EventType = domain.EventTypePlaygroundExecutionCompleted
	}

	columns := event.Columns
	rows := event.Rows
	expectedColumns := event.ExpectedColumns
	expectedRows := event.ExpectedRows
	executionTimeMS := event.ExecutionTimeMS
	rowCount := event.RowCount
	errorType := event.ErrorType
	errorMessage := event.ErrorMessage
	executionSuccess := event.Success || event.ExecutionSuccess
	truncated := event.Truncated

	if event.ExecuteResult != nil {
		columns = event.ExecuteResult.Columns
		rows = event.ExecuteResult.Rows
		if executionTimeMS == 0 {
			executionTimeMS = event.ExecuteResult.ExecutionTimeMS
			if executionTimeMS == 0 {
				executionTimeMS = event.ExecuteResult.QueryTimeMS
			}
		}
		if rowCount == 0 {
			rowCount = event.ExecuteResult.RowCount
		}
	}
	if event.ExpectedResult != nil {
		expectedColumns = event.ExpectedResult.Columns
		expectedRows = event.ExpectedResult.Rows
	}

	// Current playground-service emits user_result/reference_result. Convert its
	// row shape ([]map[column]value) to the tabular shape used by task-progress.
	if event.UserResult != nil {
		columns = event.UserResult.Columns
		rows = playgroundRowsToTabular(event.UserResult.Columns, event.UserResult.Rows)
		truncated = event.UserResult.Truncated
		if executionTimeMS == 0 {
			executionTimeMS = event.UserResult.ExecutionTimeMS
			if executionTimeMS == 0 {
				executionTimeMS = event.UserResult.QueryTimeMS
			}
		}
		if rowCount == 0 {
			if len(event.UserResult.Rows) > 0 {
				rowCount = len(event.UserResult.Rows)
			} else if event.UserResult.RowsAffected > 0 {
				rowCount = int(event.UserResult.RowsAffected)
			}
		}
		if event.UserResult.Error != nil {
			errorType = event.UserResult.Error.Type
			errorMessage = event.UserResult.Error.Message
			executionSuccess = false
		} else {
			executionSuccess = true
		}
	}
	if event.ReferenceResult != nil {
		expectedColumns = event.ReferenceResult.Columns
		expectedRows = playgroundRowsToTabular(event.ReferenceResult.Columns, event.ReferenceResult.Rows)
	}

	submittedSQL := strings.TrimSpace(event.SubmittedSQL)
	if submittedSQL == "" {
		submittedSQL = strings.TrimSpace(event.UserQuery)
	}

	_, err := h.progress.SubmitAttempt(ctx, SubmitAttemptInput{
		SourceEventID:    event.EventID,
		AttemptID:        event.AttemptID,
		UserID:           event.UserID,
		TaskID:           event.TaskID,
		SubmittedSQL:     submittedSQL,
		ExecutionSuccess: executionSuccess,
		IsCorrect:        event.IsCorrect,
		Columns:          columns,
		Rows:             rows,
		ExpectedColumns:  expectedColumns,
		ExpectedRows:     expectedRows,
		ExecutionTimeMS:  executionTimeMS,
		RowCount:         rowCount,
		ErrorType:        errorType,
		ErrorMessage:     errorMessage,
		QueryHash:        event.QueryHash,
		HintRequested:    event.HintRequested,
		HintUsed:         event.HintUsed,
		HintID:           event.HintID,
		HintType:         event.HintType,
		HintsCount:       event.HintsCount,
		CreatedAt:        event.CreatedAt,
	})
	if err != nil {
		return err
	}
	_ = truncated // kept for forward-compatible event payloads; truncation is also present in result payloads.
	return nil
}

func playgroundRowsToTabular(columns []string, rows []map[string]any) [][]any {
	if len(rows) == 0 {
		return nil
	}
	out := make([][]any, 0, len(rows))
	for _, row := range rows {
		values := make([]any, 0, len(columns))
		for _, col := range columns {
			values = append(values, row[col])
		}
		out = append(out, values)
	}
	return out
}
