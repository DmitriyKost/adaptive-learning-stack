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

	"api-gateway/internal/config"
	httptransport "api-gateway/internal/transport/http"
	"api-gateway/pkg/logger"
)

func main() {
	cfg := config.Load()
	log := logger.New(cfg.Env)

	gateway, err := httptransport.NewServer(cfg, log)
	if err != nil {
		log.Error("create gateway server failed", "error", err)
		os.Exit(1)
	}

	srv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           gateway.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       cfg.RequestTimeout + 5*time.Second,
		WriteTimeout:      cfg.RequestTimeout + 5*time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		log.Info("api gateway started",
			slog.String("addr", cfg.HTTPAddr),
			slog.String("env", cfg.Env),
			slog.String("auth_service_url", cfg.AuthServiceURL),
			slog.String("task_progress_service_url", cfg.TaskProgressServiceURL),
			slog.String("playground_service_url", cfg.PlaygroundServiceURL),
			slog.String("analytics_service_url", cfg.AnalyticsServiceURL),
		)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Error("http server failed", "error", err)
			os.Exit(1)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	log.Info("api gateway shutting down")
	if err := srv.Shutdown(ctx); err != nil {
		log.Error("shutdown failed", "error", err)
		os.Exit(1)
	}
	log.Info("api gateway stopped")
}
