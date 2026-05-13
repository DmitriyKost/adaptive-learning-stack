package kafkatransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"task-progress-service/internal/domain"
	"task-progress-service/internal/service"

	"github.com/segmentio/kafka-go"
)

type AnalyticsConsumer struct {
	reader  *kafka.Reader
	handler *service.AnalyticsEventHandler
	log     *slog.Logger
}

func NewAnalyticsSkillAssessmentConsumer(brokers []string, topic, groupID, clientID string, handler *service.AnalyticsEventHandler, log *slog.Logger) *AnalyticsConsumer {
	reader := newReader(brokers, topic, groupID, clientID, log)
	return &AnalyticsConsumer{reader: reader, handler: handler, log: log}
}

func (c *AnalyticsConsumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Error("fetch analytics kafka message failed", "error", err)
			continue
		}

		var event domain.AnalyticsSkillAssessmentUpdatedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			c.log.Error("decode analytics skill assessment event failed", "error", err, "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset)
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}
		if event.EventType == "" {
			event.EventType = domain.EventTypeAnalyticsSkillAssessmentUpdated
		}
		if err := c.handler.HandleSkillAssessmentUpdated(ctx, event); err != nil {
			c.log.Error("handle analytics skill assessment event failed", "error", err, "event_id", event.EventID, "user_id", event.UserID)
			continue
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.log.Error("commit analytics kafka message failed", "error", err, "event_id", event.EventID)
		}
	}
}

func (c *AnalyticsConsumer) Close() error { return c.reader.Close() }

type PlaygroundExecutionConsumer struct {
	reader  *kafka.Reader
	handler *service.PlaygroundEventHandler
	log     *slog.Logger
}

func NewPlaygroundExecutionCompletedConsumer(brokers []string, topic, groupID, clientID string, handler *service.PlaygroundEventHandler, log *slog.Logger) *PlaygroundExecutionConsumer {
	reader := newReader(brokers, topic, groupID, clientID, log)
	return &PlaygroundExecutionConsumer{reader: reader, handler: handler, log: log}
}

func (c *PlaygroundExecutionConsumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			c.log.Error("fetch playground kafka message failed", "error", err)
			continue
		}

		var event domain.PlaygroundExecutionCompletedEvent
		if err := json.Unmarshal(msg.Value, &event); err != nil {
			c.log.Error("decode playground execution event failed", "error", err, "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset)
			_ = c.reader.CommitMessages(ctx, msg)
			continue
		}
		if event.EventType == "" {
			event.EventType = domain.EventTypePlaygroundExecutionCompleted
		}
		if err := c.handler.HandleExecutionCompleted(ctx, event); err != nil {
			c.log.Error("handle playground execution event failed", "error", err, "event_id", event.EventID, "user_id", event.UserID, "task_id", event.TaskID)
			continue
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.log.Error("commit playground kafka message failed", "error", err, "event_id", event.EventID)
		}
	}
}

func (c *PlaygroundExecutionConsumer) Close() error { return c.reader.Close() }

func newReader(brokers []string, topic, groupID, clientID string, log *slog.Logger) *kafka.Reader {
	return kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokers,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10e6,
		CommitInterval: time.Second,
		StartOffset:    kafka.FirstOffset,
		Logger:         kafka.LoggerFunc(func(msg string, args ...any) { log.Debug(msg, args...) }),
		ErrorLogger:    kafka.LoggerFunc(func(msg string, args ...any) { log.Error(msg, args...) }),
		Dialer:         &kafka.Dialer{ClientID: clientID, Timeout: 5 * time.Second},
	})
}
