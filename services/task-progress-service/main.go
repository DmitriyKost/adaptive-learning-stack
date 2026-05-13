package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task-progress-service/internal/config"
	"task-progress-service/internal/repository"
	"task-progress-service/internal/service"
	httptransport "task-progress-service/internal/transport/http"
	kafkatransport "task-progress-service/internal/transport/kafka"
	"task-progress-service/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}
	log := logger.New(cfg.Env)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := repository.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	repo := repository.New(db)
	ebb := service.NewEbbinghaus(service.EbbinghausConfig{MinHalfLifeDays: cfg.MinHalfLifeDays, MaxHalfLifeDays: cfg.MaxHalfLifeDays})
	plannerCfg := service.PlannerConfig{DefaultGraphCode: cfg.DefaultGraphCode, MasteryThreshold: cfg.MasteryThreshold, PrerequisiteThreshold: cfg.PrerequisiteThreshold, RecallThreshold: cfg.RecallThreshold, RecommendationWaitTimeout: cfg.RecommendationWaitTimeout}

	var publisher service.EventPublisher = service.NoopPublisher{}
	if cfg.KafkaEnabled {
		publisher = kafkatransport.NewPublisher(cfg.KafkaBrokers, cfg.KafkaClientID, cfg.TopicTaskChecked, cfg.TopicTaskCompleted, cfg.TopicTaskRecommended, cfg.TopicLearnerModelUpdated, log)
	}
	defer func() { _ = publisher.Close() }()

	hintClient := service.NewHTTPHintGeneratorClient(cfg.AnalyticsInternalURL, cfg.HintGenerationTimeout)
	progress := service.NewProgressUsecase(repo, publisher, hintClient, ebb, plannerCfg, log)
	planner := service.NewPlanner(repo, ebb, plannerCfg, publisher, log)
	tokens := httptransport.NewTokenValidator(cfg.JWTSecret)
	server := httptransport.NewServer(progress, planner, repo, tokens, cfg.AllowGatewayHeaders, cfg.DefaultGraphCode, log)

	if cfg.KafkaEnabled {
		analyticsHandler := service.NewAnalyticsEventHandler(repo, publisher, log)
		analyticsConsumer := kafkatransport.NewAnalyticsSkillAssessmentConsumer(cfg.KafkaBrokers, cfg.TopicAnalyticsSkillAssessmentUpdated, cfg.KafkaGroupID+"-analytics", cfg.KafkaClientID+"-analytics", analyticsHandler, log)
		defer func() { _ = analyticsConsumer.Close() }()
		go func() {
			if err := analyticsConsumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("analytics kafka consumer stopped", "error", err)
			}
		}()

		playgroundHandler := service.NewPlaygroundEventHandler(progress, log)
		playgroundConsumer := kafkatransport.NewPlaygroundExecutionCompletedConsumer(cfg.KafkaBrokers, cfg.TopicPlaygroundExecutionCompleted, cfg.KafkaGroupID+"-playground", cfg.KafkaClientID+"-playground", playgroundHandler, log)
		defer func() { _ = playgroundConsumer.Close() }()
		go func() {
			if err := playgroundConsumer.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				log.Error("playground kafka consumer stopped", "error", err)
			}
		}()
	}

	httpServer := &http.Server{Addr: cfg.HTTPAddr, Handler: server.Handler(), ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Info("http server started", "addr", cfg.HTTPAddr)
		if err := httpServer.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := httpServer.Shutdown(shutdownCtx); err != nil {
		log.Error("http shutdown failed", "error", err)
	}
	log.Info("service stopped")
}
