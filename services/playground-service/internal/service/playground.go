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
	workspaces WorkspaceStore
	executor   *Executor
	publisher  ExecutionPublisher
}

func NewPlaygroundUsecase(workspaces WorkspaceStore, executor *Executor, publisher ExecutionPublisher) *PlaygroundUsecase {
	return &PlaygroundUsecase{workspaces: workspaces, executor: executor, publisher: publisher}
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

	var referenceResult *domain.ExecuteResult
	if strings.TrimSpace(req.ReferenceQuery) != "" && userResult.Error == nil {
		referenceResult, err = u.executor.ExecuteReference(ctx, req.ReferenceQuery)
		if err != nil {
			return domain.ExecuteResponse{}, err
		}
	}

	eventID, err := domain.NewUUID()
	if err != nil {
		return domain.ExecuteResponse{}, fmt.Errorf("generate event id: %w", err)
	}

	createdAt := time.Now().UTC()
	response := domain.ExecuteResponse{
		EventID:         eventID,
		UserID:          userID,
		TaskID:          req.TaskID,
		UserResult:      userResult,
		ReferenceResult: referenceResult,
		CreatedAt:       createdAt,
	}

	event := domain.ExecutionEvent{
		EventID:         eventID,
		EventType:       "playground.execution.completed",
		EventVersion:    1,
		UserID:          userID,
		TaskID:          req.TaskID,
		UserQuery:       req.UserQuery,
		ReferenceQuery:  req.ReferenceQuery,
		UserResult:      userResult,
		ReferenceResult: referenceResult,
		CreatedAt:       createdAt,
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
