package messaging

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type Event struct {
	EventID   string      `json:"event_id"`
	Type      string      `json:"type"`
	Source    string      `json:"source"`
	Version   string      `json:"version"`
	Timestamp time.Time   `json:"timestamp"`
	Payload   interface{} `json:"payload"`
}

type Publisher struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
}

func NewPublisher(amqpURL string) (*Publisher, error) {
	conn, err := amqp091.Dial(amqpURL)
	if err != nil {
		return nil, err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, err
	}

	err = ch.ExchangeDeclare(
		"doowork.events",
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		ch.Close()
		conn.Close()
		return nil, err
	}

	return &Publisher{conn: conn, channel: ch}, nil
}

func (p *Publisher) Publish(routingKey string, event Event) error {
	body, err := json.Marshal(event)
	if err != nil {
		return err
	}

	return p.channel.Publish(
		"doowork.events",
		routingKey,
		false,
		false,
		amqp091.Publishing{
			ContentType: "application/json",
			Body:        body,
		},
	)
}

func (p *Publisher) Close() error {
	if p == nil {
		return nil
	}

	var chErr error
	if p.channel != nil {
		chErr = p.channel.Close()
	}

	var connErr error
	if p.conn != nil {
		connErr = p.conn.Close()
	}

	if chErr != nil {
		return chErr
	}
	if connErr != nil {
		return connErr
	}

	return nil
}

func ConnectWithRetry(amqpURL string, maxRetry int, delay time.Duration) (*Publisher, error) {
	var lastErr error
	for i := 0; i < maxRetry; i++ {
		publisher, err := NewPublisher(amqpURL)
		if err == nil {
			return publisher, nil
		}

		lastErr = err
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("failed to connect to rabbitmq after %d retries: %w", maxRetry, lastErr)
}
