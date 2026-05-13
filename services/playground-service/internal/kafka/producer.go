package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"playground-service/internal/domain"

	kafkago "github.com/segmentio/kafka-go"
)

type Producer struct {
	writer *kafkago.Writer
	topic  string
}

func NewProducer(brokers []string, topic string, clientID string) *Producer {
	return &Producer{
		topic: topic,
		writer: &kafkago.Writer{
			Addr:                   kafkago.TCP(brokers...),
			Topic:                  topic,
			Balancer:               &kafkago.Hash{},
			RequiredAcks:           kafkago.RequireAll,
			AllowAutoTopicCreation: false,
			BatchTimeout:           20 * time.Millisecond,
			WriteTimeout:           5 * time.Second,
			ReadTimeout:            5 * time.Second,
			Transport: &kafkago.Transport{
				ClientID: clientID,
			},
		},
	}
}

func (p *Producer) PublishExecution(ctx context.Context, event domain.ExecutionEvent) error {
	payload, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("marshal execution event: %w", err)
	}

	message := kafkago.Message{
		Key:   []byte(event.UserID),
		Value: payload,
		Time:  event.CreatedAt,
		Headers: []kafkago.Header{
			{Key: "event_type", Value: []byte(event.EventType)},
			{Key: "event_version", Value: []byte(fmt.Sprintf("%d", event.EventVersion))},
		},
	}

	if err := p.writer.WriteMessages(ctx, message); err != nil {
		return fmt.Errorf("write kafka message: %w", err)
	}
	return nil
}

func (p *Producer) Close() error {
	if p == nil || p.writer == nil {
		return nil
	}
	return p.writer.Close()
}

func EnsureTopic(ctx context.Context, brokers []string, topic string) error {
	if len(brokers) == 0 {
		return fmt.Errorf("empty kafka broker list")
	}

	conn, err := kafkago.DialContext(ctx, "tcp", brokers[0])
	if err != nil {
		return fmt.Errorf("dial kafka broker: %w", err)
	}
	defer conn.Close()

	controller, err := conn.Controller()
	if err != nil {
		return fmt.Errorf("get kafka controller: %w", err)
	}

	controllerConn, err := kafkago.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", controller.Host, controller.Port))
	if err != nil {
		return fmt.Errorf("dial kafka controller: %w", err)
	}
	defer controllerConn.Close()

	err = controllerConn.CreateTopics(kafkago.TopicConfig{
		Topic:             topic,
		NumPartitions:     3,
		ReplicationFactor: 1,
	})
	if err != nil {
		// Redpanda/Kafka returns an error if the topic already exists. The service can continue.
		return nil
	}
	return nil
}
