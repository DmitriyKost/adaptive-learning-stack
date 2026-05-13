package kafka

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"

	"analytics-service/internal/service"

	"github.com/segmentio/kafka-go"
)

type Consumer struct {
	reader  *kafka.Reader
	handler *service.EventHandler
	log     *slog.Logger
}

func NewConsumer(brokers []string, groupID, clientID string, topics []string, handler *service.EventHandler, log *slog.Logger) *Consumer {
	return &Consumer{reader: kafka.NewReader(kafka.ReaderConfig{
		Brokers:     brokers,
		GroupID:     groupID,
		GroupTopics: topics,
		Dialer:      &kafka.Dialer{ClientID: clientID, Timeout: 5 * time.Second},
		MinBytes:    1,
		MaxBytes:    10e6,
		MaxWait:     1 * time.Second,
	}), handler: handler, log: log}
}

func (c *Consumer) Run(ctx context.Context) error {
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) {
				return nil
			}
			if strings.Contains(err.Error(), "use of closed network connection") {
				return nil
			}
			c.log.Error("fetch kafka message failed", "error", err)
			continue
		}
		err = c.handler.Handle(ctx, service.KafkaMeta{Topic: msg.Topic, Partition: msg.Partition, Offset: msg.Offset}, msg.Value)
		if err != nil {
			c.log.Error("handle kafka message failed", "error", err, "topic", msg.Topic, "partition", msg.Partition, "offset", msg.Offset)
			// Do not commit failed messages. Kafka/Redpanda will redeliver them.
			continue
		}
		if err := c.reader.CommitMessages(ctx, msg); err != nil {
			c.log.Error("commit kafka message failed", "error", err, "topic", msg.Topic, "offset", msg.Offset)
		}
	}
}

func (c *Consumer) Close() error { return c.reader.Close() }
