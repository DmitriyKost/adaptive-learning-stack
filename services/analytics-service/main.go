package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"analytics-service/internal/config"
	"analytics-service/internal/repository"
	"analytics-service/internal/service"
	httptransport "analytics-service/internal/transport/http"
	kafkatransport "analytics-service/internal/transport/kafka"
	"analytics-service/pkg/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.Env)
	slog.SetDefault(log)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	repo, err := repository.Connect(ctx, cfg, log)
	if err != nil {
		log.Error("connect clickhouse failed", "error", err)
		os.Exit(1)
	}
	defer repo.Close()
	if cfg.ClickHouseAutoMigrate {
		if err := repo.AutoMigrate(ctx); err != nil {
			log.Error("clickhouse automigrate failed", "error", err)
			os.Exit(1)
		}
	}

	var publisher service.EventPublisher = service.NoopPublisher{}
	if cfg.KafkaEnabled {
		publisher = kafkatransport.NewPublisher(cfg.KafkaBrokers, cfg.KafkaClientID, cfg.TopicAnalyticsSkillAssessmentUpdated, cfg.TopicCareerRecommendationCreated, cfg.TopicHintGenerated, log)
	}
	defer publisher.Close()

	if strings.TrimSpace(cfg.IntelligenceBaseURL) == "" {
		log.Error("INTELLIGENCE_BASE_URL is required", "example", "http://intelligence:8080")
		os.Exit(1)
	}
	intelligence := service.NewHTTPIntelligenceClient(cfg)
	log.Info("intelligence: HTTP client", "base_url", cfg.IntelligenceBaseURL, "ready_path", cfg.IntelligenceReadyPath, "evaluate_path", cfg.IntelligenceEvaluatePath, "hint_path", cfg.IntelligenceHintPath, "include_career", cfg.IntelligenceIncludeCareer)
	eventHandler := service.NewEventHandler(repo, intelligence, publisher, log)

	var consumer *kafkatransport.Consumer
	if cfg.KafkaEnabled {
		topics := []string{cfg.TopicTaskChecked, cfg.TopicTaskCompleted, cfg.TopicTaskRecommended, cfg.TopicLearnerModelUpdated}
		consumer = kafkatransport.NewConsumer(cfg.KafkaBrokers, cfg.KafkaGroupID, cfg.KafkaClientID, topics, eventHandler, log)
		go func() {
			if err := consumer.Run(ctx); err != nil {
				log.Error("kafka consumer stopped", "error", err)
			}
		}()
		defer consumer.Close()
	}

	h := httptransport.NewHandler(repo, intelligence, publisher, cfg.HintGenerationTimeout, log)
	srv := &http.Server{Addr: cfg.HTTPAddr, Handler: h.Router(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("analytics-service started", "addr", cfg.HTTPAddr, "kafka_enabled", cfg.KafkaEnabled)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
	log.Info("analytics-service stopped")
}
