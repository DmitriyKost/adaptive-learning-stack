package service

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"task-progress-service/internal/domain"
	"task-progress-service/internal/repository"
)

type AnalyticsEventHandler struct {
	repo      *repository.Repository
	publisher EventPublisher
	log       *slog.Logger
}

func NewAnalyticsEventHandler(repo *repository.Repository, publisher EventPublisher, log *slog.Logger) *AnalyticsEventHandler {
	return &AnalyticsEventHandler{repo: repo, publisher: publisher, log: log}
}

func (h *AnalyticsEventHandler) HandleSkillAssessmentUpdated(ctx context.Context, event domain.AnalyticsSkillAssessmentUpdatedEvent) error {
	if event.EventID == "" || event.UserID == "" {
		return domain.ErrInvalidInput
	}
	if event.CreatedAt.IsZero() {
		event.CreatedAt = time.Now().UTC()
	}
	updates, err := h.repo.ApplyAnalyticsAssessment(ctx, event)
	if errors.Is(err, domain.ErrDuplicateEvent) {
		h.log.Info("analytics event already processed", "event_id", event.EventID)
		return nil
	}
	if err != nil {
		return err
	}

	for _, update := range updates {
		eventID, _ := domain.NewUUID()
		out := domain.LearnerModelUpdatedEvent{
			EventID:         eventID,
			EventVersion:    1,
			EventType:       domain.EventTypeLearnerModelUpdated,
			UserID:          event.UserID,
			SkillID:         update.SkillID,
			SkillCode:       update.SkillCode,
			OldMasteryScore: update.OldMastery,
			NewMasteryScore: update.NewMastery,
			OldConfidence:   update.OldConfidence,
			NewConfidence:   update.NewConfidence,
			Source:          "analytics-service",
			Reason:          event.RecommendationReason,
			Analysis:        event.Analysis,
			CreatedAt:       event.CreatedAt,
		}
		if err := h.publisher.PublishLearnerModelUpdated(ctx, out); err != nil {
			h.log.Error("publish learner model update from analytics failed", "error", err, "event_id", event.EventID)
		}
	}
	return nil
}
