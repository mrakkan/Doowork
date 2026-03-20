package messaging

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"project-service/models"

	"gorm.io/gorm"
)

type OutboxProcessor struct {
	db        *gorm.DB
	publisher *Publisher
	source    string
}

func NewOutboxProcessor(db *gorm.DB, publisher *Publisher, source string) *OutboxProcessor {
	return &OutboxProcessor{db: db, publisher: publisher, source: source}
}

func (p *OutboxProcessor) Start(ctx context.Context, interval time.Duration) {
	if p == nil || p.publisher == nil {
		return
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := p.ProcessPending(50); err != nil {
				log.Printf("outbox processor error: %v", err)
			}
		}
	}
}

func (p *OutboxProcessor) ProcessPending(limit int) error {
	var events []models.OutboxEvent
	if err := p.db.Where("status = ?", "pending").Order("id asc").Limit(limit).Find(&events).Error; err != nil {
		return err
	}

	for _, item := range events {
		payload := json.RawMessage(item.Payload)
		event := Event{
			EventID:   item.EventID,
			Type:      item.EventType,
			Source:    p.source,
			Version:   "v1",
			Timestamp: time.Now().UTC(),
			Payload:   payload,
		}

		if err := p.publisher.Publish(item.RoutingKey, event); err != nil {
			_ = p.db.Model(&models.OutboxEvent{}).
				Where("id = ?", item.ID).
				Updates(map[string]interface{}{
					"retry_count": item.RetryCount + 1,
					"last_error":  err.Error(),
				}).Error
			continue
		}

		now := time.Now().UTC()
		_ = p.db.Model(&models.OutboxEvent{}).
			Where("id = ?", item.ID).
			Updates(map[string]interface{}{
				"status":       "published",
				"published_at": &now,
				"last_error":   "",
			}).Error
	}

	return nil
}
