package kafkatransport

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"task-progress-service/internal/domain"

	"github.com/segmentio/kafka-go"
)

type Publisher struct {
	writer                   *kafka.Writer
	topicTaskChecked         string
	topicTaskCompleted       string
	topicTaskRecommended     string
	topicLearnerModelUpdated string
	log                      *slog.Logger
}

func NewPublisher(brokers []string, clientID, topicTaskChecked, topicTaskCompleted, topicTaskRecommended, topicLearnerModelUpdated string, log *slog.Logger) *Publisher {
	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokers...),
		Balancer:               &kafka.Hash{},
		RequiredAcks:           kafka.RequireOne,
		AllowAutoTopicCreation: true,
		BatchTimeout:           10 * time.Millisecond,
		WriteTimeout:           5 * time.Second,
		ReadTimeout:            5 * time.Second,
		Transport:              &kafka.Transport{ClientID: clientID},
	}
	return &Publisher{writer: writer, topicTaskChecked: topicTaskChecked, topicTaskCompleted: topicTaskCompleted, topicTaskRecommended: topicTaskRecommended, topicLearnerModelUpdated: topicLearnerModelUpdated, log: log}
}

func (p *Publisher) PublishTaskChecked(ctx context.Context, event domain.TaskCheckedEvent) error {
	return p.publish(ctx, p.topicTaskChecked, event.UserID, event.EventType, event)
}

func (p *Publisher) PublishTaskCompleted(ctx context.Context, event domain.TaskCompletedEvent) error {
	return p.publish(ctx, p.topicTaskCompleted, event.UserID, event.EventType, event)
}

func (p *Publisher) PublishTaskRecommended(ctx context.Context, event domain.TaskRecommendedEvent) error {
	return p.publish(ctx, p.topicTaskRecommended, event.UserID, event.EventType, event)
}

func (p *Publisher) PublishLearnerModelUpdated(ctx context.Context, event domain.LearnerModelUpdatedEvent) error {
	return p.publish(ctx, p.topicLearnerModelUpdated, event.UserID, event.EventType, event)
}

func (p *Publisher) publish(ctx context.Context, topic, key, eventType string, payload any) error {
	data, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	return p.writer.WriteMessages(ctx, kafka.Message{
		Topic: topic,
		Key:   []byte(key),
		Value: data,
		Headers: []kafka.Header{
			{Key: "event_type", Value: []byte(eventType)},
			{Key: "event_version", Value: []byte("1")},
			{Key: "producer", Value: []byte("task-progress-service")},
		},
		Time: time.Now().UTC(),
	})
}

func (p *Publisher) Close() error { return p.writer.Close() }
