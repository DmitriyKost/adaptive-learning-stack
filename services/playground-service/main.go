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

	"playground-service/internal/config"
	playgroundkafka "playground-service/internal/kafka"
	"playground-service/internal/repository"
	"playground-service/internal/service"
	httptransport "playground-service/internal/transport/http"
	"playground-service/pkg/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config failed", "error", err)
		os.Exit(1)
	}

	log := logger.New(cfg.Env)
	if cfg.JWTSecret == "change-me-in-production" && !cfg.TrustedGatewayHeaders {
		log.Warn("JWT_SECRET uses default development value; change it outside local development")
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	db, err := repository.Connect(ctx, cfg.DatabaseURL)
	if err != nil {
		log.Error("database connection failed", "error", err)
		os.Exit(1)
	}
	defer db.Close()

	if cfg.KafkaAutoCreateTopics {
		ensureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		if err := playgroundkafka.EnsureTopic(ensureCtx, cfg.KafkaBrokers, cfg.KafkaExecutionTopic); err != nil {
			cancel()
			log.Error("ensure kafka topic failed", "error", err)
			os.Exit(1)
		}
		cancel()
	}

	producer := playgroundkafka.NewProducer(cfg.KafkaBrokers, cfg.KafkaExecutionTopic, cfg.KafkaClientID)
	defer func() {
		if err := producer.Close(); err != nil {
			log.Error("close kafka producer failed", "error", err)
		}
	}()

	workspaceRepo := repository.NewWorkspaceRepository(db, cfg.InternalSchema)
	executor := service.NewExecutor(db, service.ExecutorOptions{
		ExecutionTimeout: cfg.ExecutionTimeout,
		StatementTimeout: cfg.StatementTimeout,
		LockTimeout:      cfg.LockTimeout,
		MaxResultRows:    cfg.MaxResultRows,
		TaskSchema:       cfg.TaskSchema,
		ReadonlyRole:     cfg.ReadonlyRole,
	})
	tokenService := service.NewTokenService(cfg.JWTSecret)
	playgroundUsecase := service.NewPlaygroundUsecase(workspaceRepo, executor, producer)

	handler := httptransport.NewRouter(playgroundUsecase, tokenService, cfg.TrustedGatewayHeaders, log)

	server := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}

	go func() {
		log.Info("playground service started", "addr", cfg.HTTPAddr)
		if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			stop()
		}
	}()

	<-ctx.Done()
	log.Info("shutting down playground service")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Error("graceful shutdown failed", "error", err)
		os.Exit(1)
	}

	log.Info("playground service stopped")
}
