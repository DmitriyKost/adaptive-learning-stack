package service

import (
	"context"

	"task-progress-service/internal/domain"
)

type EventPublisher interface {
	PublishTaskChecked(ctx context.Context, event domain.TaskCheckedEvent) error
	PublishTaskCompleted(ctx context.Context, event domain.TaskCompletedEvent) error
	PublishTaskRecommended(ctx context.Context, event domain.TaskRecommendedEvent) error
	PublishLearnerModelUpdated(ctx context.Context, event domain.LearnerModelUpdatedEvent) error
	Close() error
}

type NoopPublisher struct{}

func (p NoopPublisher) PublishTaskChecked(ctx context.Context, event domain.TaskCheckedEvent) error {
	return nil
}
func (p NoopPublisher) PublishTaskCompleted(ctx context.Context, event domain.TaskCompletedEvent) error {
	return nil
}
func (p NoopPublisher) PublishTaskRecommended(ctx context.Context, event domain.TaskRecommendedEvent) error {
	return nil
}
func (p NoopPublisher) PublishLearnerModelUpdated(ctx context.Context, event domain.LearnerModelUpdatedEvent) error {
	return nil
}
func (p NoopPublisher) Close() error { return nil }
