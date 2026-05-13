package service

import (
	"analytics-service/internal/domain"
	"context"
)

type EventPublisher interface {
	PublishSkillAssessmentUpdated(ctx context.Context, event domain.AnalyticsSkillAssessmentUpdatedEvent) error
	PublishCareerRecommendationCreated(ctx context.Context, event domain.CareerRecommendationCreatedEvent) error
	PublishHintGenerated(ctx context.Context, event domain.AnalyticsHintGeneratedEvent) error
	Close() error
}

type NoopPublisher struct{}

func (NoopPublisher) PublishSkillAssessmentUpdated(ctx context.Context, event domain.AnalyticsSkillAssessmentUpdatedEvent) error {
	return nil
}
func (NoopPublisher) PublishCareerRecommendationCreated(ctx context.Context, event domain.CareerRecommendationCreatedEvent) error {
	return nil
}
func (NoopPublisher) PublishHintGenerated(ctx context.Context, event domain.AnalyticsHintGeneratedEvent) error {
	return nil
}
func (NoopPublisher) Close() error { return nil }
