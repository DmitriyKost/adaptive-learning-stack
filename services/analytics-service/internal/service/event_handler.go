package service

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"analytics-service/internal/domain"
	"analytics-service/internal/repository"
)

type EventHandler struct {
	repo         *repository.Repository
	intelligence IntelligenceClient
	publisher    EventPublisher
	log          *slog.Logger
}

func NewEventHandler(repo *repository.Repository, intelligence IntelligenceClient, publisher EventPublisher, log *slog.Logger) *EventHandler {
	return &EventHandler{repo: repo, intelligence: intelligence, publisher: publisher, log: log}
}

type KafkaMeta struct {
	Topic     string
	Partition int
	Offset    int64
}

func (h *EventHandler) Handle(ctx context.Context, meta KafkaMeta, payload []byte) error {
	var env domain.EventEnvelope
	if err := json.Unmarshal(payload, &env); err != nil {
		return fmt.Errorf("decode envelope: %w", err)
	}
	if env.EventID == "" {
		return fmt.Errorf("event_id is required")
	}
	if env.CreatedAt.IsZero() {
		env.CreatedAt = time.Now().UTC()
	}
	processed, err := h.repo.IsEventProcessed(ctx, env.EventID)
	if err != nil {
		return err
	}
	if processed {
		h.log.Info("skip duplicate event", "event_id", env.EventID, "event_type", env.EventType)
		return nil
	}
	if err := h.repo.LogRawEvent(ctx, domain.RawEventLog{EventID: env.EventID, EventType: env.EventType, UserID: env.UserID, TaskID: env.TaskID, AttemptID: env.AttemptID, SourceTopic: meta.Topic, KafkaPartition: meta.Partition, KafkaOffset: meta.Offset, EventTime: env.CreatedAt, Payload: string(payload)}); err != nil {
		return err
	}

	switch env.EventType {
	case domain.EventTypeTaskChecked:
		return h.handleTaskChecked(ctx, payload)
	case domain.EventTypeTaskCompleted:
		return h.handleTaskCompleted(ctx, payload)
	case domain.EventTypeTaskRecommended:
		return h.handleTaskRecommended(ctx, payload)
	case domain.EventTypeLearnerModelUpdated:
		return h.handleLearnerModelUpdated(ctx, payload)
	default:
		h.log.Debug("raw event stored without structured handler", "event_type", env.EventType, "event_id", env.EventID)
		return nil
	}
}

func (h *EventHandler) handleTaskChecked(ctx context.Context, payload []byte) error {
	var event domain.TaskCheckedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	return h.repo.StoreTaskAttempt(ctx, event)
}

func (h *EventHandler) handleTaskRecommended(ctx context.Context, payload []byte) error {
	var event domain.TaskRecommendedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	return h.repo.StoreTaskRecommendation(ctx, event, string(payload))
}

func (h *EventHandler) handleLearnerModelUpdated(ctx context.Context, payload []byte) error {
	var event domain.LearnerModelUpdatedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	return h.repo.StoreLearnerModelUpdate(ctx, event)
}

func (h *EventHandler) handleTaskCompleted(ctx context.Context, payload []byte) error {
	var event domain.TaskCompletedEvent
	if err := json.Unmarshal(payload, &event); err != nil {
		return err
	}
	if err := h.repo.StoreTaskCompletion(ctx, event); err != nil {
		return err
	}
	llmCtx, err := h.repo.LoadLLMContext(ctx, event)
	if err != nil {
		return err
	}
	start := time.Now().UTC()
	result, err := h.intelligence.EvaluateTaskCompletion(ctx, llmCtx)
	completed := time.Now().UTC()
	if err != nil {
		runID, _ := domain.NewUUID()
		_ = h.repo.SaveAnalysisRun(ctx, domain.AnalysisRunLog{RunID: runID, UserID: event.UserID, SourceTaskID: event.TaskID, SourceAttemptID: event.AttemptID, Status: "failed", ErrorMessage: err.Error(), StartedAt: start, CompletedAt: completed})
		return err
	}
	runID := ""
	modelVersion := ""
	promptVersion := ""
	if result.Assessment.Analysis != nil {
		runID = result.Assessment.Analysis.AnalysisRunID
		modelVersion = result.Assessment.Analysis.ModelVersion
		promptVersion = result.Assessment.Analysis.PromptVersion
	}
	if err := h.repo.SaveAnalysisRun(ctx, domain.AnalysisRunLog{RunID: runID, UserID: event.UserID, SourceTaskID: event.TaskID, SourceAttemptID: event.AttemptID, Status: "completed", ModelVersion: modelVersion, PromptVersion: promptVersion, RequestContext: result.RequestContextJSON, ResponsePayload: result.ResponseJSON, StartedAt: start, CompletedAt: completed}); err != nil {
		return err
	}
	if err := h.repo.SaveSkillAssessments(ctx, runID, event.UserID, result.Assessment.SkillScores, result.Assessment.CreatedAt); err != nil {
		return err
	}
	if err := h.repo.SaveSkillRecommendations(ctx, runID, event.UserID, result.Assessment.RecommendedSkills, result.Assessment.CreatedAt); err != nil {
		return err
	}
	if result.Career != nil {
		if err := h.repo.SaveCareerRecommendation(ctx, *result.Career); err != nil {
			return err
		}
		if err := h.publisher.PublishCareerRecommendationCreated(ctx, *result.Career); err != nil {
			h.log.Error("publish career recommendation failed", "error", err, "run_id", runID)
		}
	}
	if err := h.publisher.PublishSkillAssessmentUpdated(ctx, result.Assessment); err != nil {
		return err
	}
	h.log.Info("analysis completed", "run_id", runID, "user_id", event.UserID, "task_id", event.TaskID, "skills", len(result.Assessment.SkillScores), "recommended_skills", len(result.Assessment.RecommendedSkills))
	return nil
}
