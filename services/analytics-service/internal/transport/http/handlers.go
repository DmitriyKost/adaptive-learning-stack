package httptransport

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"analytics-service/internal/domain"
	"analytics-service/internal/repository"
	"analytics-service/internal/service"
)

type Handler struct {
	repo         *repository.Repository
	intelligence service.IntelligenceClient
	publisher    service.EventPublisher
	hintTimeout  time.Duration
	log          *slog.Logger
}

func NewHandler(repo *repository.Repository, intelligence service.IntelligenceClient, publisher service.EventPublisher, hintTimeout time.Duration, log *slog.Logger) *Handler {
	return &Handler{repo: repo, intelligence: intelligence, publisher: publisher, hintTimeout: hintTimeout, log: log}
}

func (h *Handler) Router() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", h.health)
	mux.HandleFunc("GET /ready", h.ready)
	mux.HandleFunc("POST /internal/hints/generate", h.generateHint)
	return loggingMiddleware(mux)
}

func (h *Handler) health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
func (h *Handler) ready(w http.ResponseWriter, r *http.Request) {
	if err := h.repo.Ping(r.Context()); err != nil {
		writeError(w, http.StatusServiceUnavailable, "clickhouse_unavailable")
		return
	}
	if err := h.intelligence.Ready(r.Context()); err != nil {
		h.log.Warn("intelligence readiness check failed", "error", err)
		writeError(w, http.StatusServiceUnavailable, "intelligence_unavailable")
		return
	}
	writeJSON(w, http.StatusOK, map[string]string{"status": "ready"})
}

func (h *Handler) generateHint(w http.ResponseWriter, r *http.Request) {
	var req domain.HintGenerateRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	if req.UserID == "" || req.TaskID == "" || req.CurrentAttemptNumber < 2 {
		writeError(w, http.StatusBadRequest, "invalid_input")
		return
	}
	ctx := r.Context()
	if h.hintTimeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, h.hintTimeout)
		defer cancel()
	}
	hintCtx, err := h.repo.LoadHintContext(ctx, req)
	if err != nil {
		writeError(w, http.StatusNotFound, "hint_context_not_found")
		return
	}
	started := time.Now().UTC()
	resp, requestJSON, responseJSON, err := h.intelligence.GenerateHint(ctx, hintCtx)
	completed := time.Now().UTC()
	if err != nil {
		status := http.StatusGatewayTimeout
		if !errors.Is(err, context.DeadlineExceeded) && !errors.Is(ctx.Err(), context.DeadlineExceeded) {
			status = http.StatusBadGateway
		}
		// Failed hint generations are intentionally not written to hint_events and are
		// not counted by task-progress: фактически подсказка не была выдана.
		h.log.Warn("hint generation failed", "error", err, "user_id", req.UserID, "task_id", req.TaskID, "duration", completed.Sub(started))
		writeError(w, status, "hint_generation_failed")
		return
	}
	if resp.UserID == "" {
		resp.UserID = req.UserID
	}
	if resp.TaskID == "" {
		resp.TaskID = req.TaskID
	}
	if resp.AttemptNumber == 0 {
		resp.AttemptNumber = req.CurrentAttemptNumber
	}
	if resp.CreatedAt.IsZero() {
		resp.CreatedAt = completed
	}
	eventID, _ := domain.NewUUID()
	if err := h.repo.StoreHintGenerated(ctx, eventID, resp, requestJSON, responseJSON); err != nil {
		writeError(w, http.StatusInternalServerError, "hint_log_failed")
		return
	}
	event := domain.AnalyticsHintGeneratedEvent{EventID: eventID, EventVersion: 1, EventType: domain.EventTypeAnalyticsHintGenerated, UserID: resp.UserID, TaskID: resp.TaskID, HintID: resp.HintID, HintType: resp.HintType, AttemptNumber: resp.AttemptNumber, RelatedSkills: resp.RelatedSkills, CreatedAt: resp.CreatedAt}
	if err := h.publisher.PublishHintGenerated(ctx, event); err != nil {
		h.log.Error("publish hint generated failed", "error", err, "hint_id", resp.HintID)
	}
	writeJSON(w, http.StatusOK, resp)
}
