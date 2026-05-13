package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"time"

	"analytics-service/internal/domain"

	"github.com/segmentio/kafka-go"
)

type Publisher struct {
	writer               *kafka.Writer
	topicSkillAssessment string
	topicCareer          string
	topicHint            string
	log                  *slog.Logger
}

func NewPublisher(brokers []string, clientID, topicSkillAssessment, topicCareer, topicHint string, log *slog.Logger) *Publisher {
	return &Publisher{writer: &kafka.Writer{Addr: kafka.TCP(brokers...), Balancer: &kafka.Hash{}, RequiredAcks: kafka.RequireAll, Async: false, WriteTimeout: 10 * time.Second, Completion: nil}, topicSkillAssessment: topicSkillAssessment, topicCareer: topicCareer, topicHint: topicHint, log: log}
}

func (p *Publisher) PublishSkillAssessmentUpdated(ctx context.Context, event domain.AnalyticsSkillAssessmentUpdatedEvent) error {
	return p.publish(ctx, p.topicSkillAssessment, event.UserID, event.EventType, event)
}
func (p *Publisher) PublishCareerRecommendationCreated(ctx context.Context, event domain.CareerRecommendationCreatedEvent) error {
	return p.publish(ctx, p.topicCareer, event.UserID, event.EventType, event)
}
func (p *Publisher) PublishHintGenerated(ctx context.Context, event domain.AnalyticsHintGeneratedEvent) error {
	return p.publish(ctx, p.topicHint, event.UserID, event.EventType, event)
}
func (p *Publisher) publish(ctx context.Context, topic, key, eventType string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	err = p.writer.WriteMessages(ctx, kafka.Message{Topic: topic, Key: []byte(key), Value: b, Time: time.Now().UTC(), Headers: []kafka.Header{{Key: "event_type", Value: []byte(eventType)}}})
	if err == nil {
		p.log.Debug("published kafka event", "topic", topic, "event_type", eventType, "key", key)
	}
	return err
}
func (p *Publisher) Close() error { return p.writer.Close() }
