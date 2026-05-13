package logger

import (
	"log/slog"
	"os"
)

func New(env string) *slog.Logger {
	level := slog.LevelInfo
	if env == "local" || env == "dev" {
		level = slog.LevelDebug
	}

	var handler slog.Handler = slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	if env == "local" {
		handler = slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})
	}

	log := slog.New(handler)
	slog.SetDefault(log)
	return log
}
