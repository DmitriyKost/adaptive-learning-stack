package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"playground-service/internal/domain"
)

type WorkspaceStore interface {
	EnsureWorkspace(ctx context.Context, userID string) (domain.Workspace, error)
	ResetWorkspace(ctx context.Context, userID string) (domain.Workspace, error)
}

type ExecutionPublisher interface {
	PublishExecution(ctx context.Context, event domain.ExecutionEvent) error
}

type PlaygroundUsecase struct {
	workspaces            WorkspaceStore
	executor              *Executor
	publisher             ExecutionPublisher
	references            TaskReferenceProvider
	returnReferenceResult bool
}

func NewPlaygroundUsecase(workspaces WorkspaceStore, executor *Executor, publisher ExecutionPublisher, references TaskReferenceProvider, returnReferenceResult bool) *PlaygroundUsecase {
	return &PlaygroundUsecase{
		workspaces:            workspaces,
		executor:              executor,
		publisher:             publisher,
		references:            references,
		returnReferenceResult: returnReferenceResult,
	}
}

func (u *PlaygroundUsecase) Execute(ctx context.Context, userID string, req domain.ExecuteRequest) (domain.ExecuteResponse, error) {
	userID = strings.TrimSpace(userID)
	if userID == "" {
		return domain.ExecuteResponse{}, domain.ErrUnauthorized
	}
	if strings.TrimSpace(req.TaskID) == "" || strings.TrimSpace(req.UserQuery) == "" {
		return domain.ExecuteResponse{}, domain.ErrInvalidInput
	}

	workspace, err := u.workspaces.EnsureWorkspace(ctx, userID)
	if err != nil {
		return domain.ExecuteResponse{}, err
	}

	userResult, err := u.executor.ExecuteUser(ctx, workspace, req.UserQuery)
	if err != nil {
		return domain.ExecuteResponse{}, err
	}

	referenceQuery, err := u.references.ReferenceQuery(ctx, req.TaskID)
	if err != nil {
		return domain.ExecuteResponse{}, err
	}

	executionSuccess := userResult.Error == nil
	isCorrectValue := false
	isCorrect := &isCorrectValue

	var referenceResult *domain.ExecuteResult
	if executionSuccess {
		referenceResult, err = u.executor.ExecuteReference(ctx, referenceQuery)
		if err != nil {
			return domain.ExecuteResponse{}, err
		}
		isCorrectValue = compareExecuteResults(userResult, referenceResult)
	}

	eventID, err := domain.NewUUID()
	if err != nil {
		return domain.ExecuteResponse{}, fmt.Errorf("generate event id: %w", err)
	}

	createdAt := time.Now().UTC()
	var responseReferenceResult *domain.ExecuteResult
	if u.returnReferenceResult {
		responseReferenceResult = referenceResult
	}

	response := domain.ExecuteResponse{
		EventID:          eventID,
		UserID:           userID,
		TaskID:           req.TaskID,
		ExecutionSuccess: executionSuccess,
		IsCorrect:        isCorrect,
		UserResult:       userResult,
		ReferenceResult:  responseReferenceResult,
		CreatedAt:        createdAt,
	}

	event := domain.ExecutionEvent{
		EventID:          eventID,
		EventType:        "playground.execution.completed",
		EventVersion:     1,
		UserID:           userID,
		TaskID:           req.TaskID,
		UserQuery:        req.UserQuery,
		ReferenceQuery:   referenceQuery,
		ExecutionSuccess: executionSuccess,
		IsCorrect:        isCorrect,
		UserResult:       userResult,
		ReferenceResult:  referenceResult,
		CreatedAt:        createdAt,
	}
	if err := u.publisher.PublishExecution(ctx, event); err != nil {
		return domain.ExecuteResponse{}, err
	}

	return response, nil
}

func (u *PlaygroundUsecase) EnsureWorkspace(ctx context.Context, userID string) (domain.Workspace, error) {
	if strings.TrimSpace(userID) == "" {
		return domain.Workspace{}, domain.ErrUnauthorized
	}
	return u.workspaces.EnsureWorkspace(ctx, userID)
}

func (u *PlaygroundUsecase) ResetWorkspace(ctx context.Context, userID string) (domain.Workspace, error) {
	if strings.TrimSpace(userID) == "" {
		return domain.Workspace{}, domain.ErrUnauthorized
	}
	return u.workspaces.ResetWorkspace(ctx, userID)
}
