package messaging

import (
	"encoding/json"
	"fmt"
	"time"

	"github.com/rabbitmq/amqp091-go"
)

type Event struct {
	EventID   string          `json:"event_id"`
	Type      string          `json:"type"`
	Source    string          `json:"source"`
	Version   string          `json:"version"`
	Timestamp time.Time       `json:"timestamp"`
	Payload   json.RawMessage `json:"payload"`
}

type Consumer struct {
	conn    *amqp091.Connection
	channel *amqp091.Channel
	queue   string
}

func NewConsumer(amqpURL, queueName string) (*Consumer, error) {
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

	q, err := ch.QueueDeclare(
		queueName,
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

	for _, key := range []string{"project.*", "task.*"} {
		if err := ch.QueueBind(q.Name, key, "doowork.events", false, nil); err != nil {
			ch.Close()
			conn.Close()
			return nil, err
		}
	}

	return &Consumer{conn: conn, channel: ch, queue: q.Name}, nil
}

func (c *Consumer) Consume() (<-chan amqp091.Delivery, error) {
	return c.channel.Consume(
		c.queue,
		"",
		false,
		false,
		false,
		false,
		nil,
	)
}

func (c *Consumer) Close() error {
	if c == nil {
		return nil
	}

	var chErr error
	if c.channel != nil {
		chErr = c.channel.Close()
	}

	var connErr error
	if c.conn != nil {
		connErr = c.conn.Close()
	}

	if chErr != nil {
		return chErr
	}
	if connErr != nil {
		return connErr
	}

	return nil
}

func ConnectWithRetry(amqpURL, queueName string, maxRetry int, delay time.Duration) (*Consumer, error) {
	var lastErr error
	for i := 0; i < maxRetry; i++ {
		consumer, err := NewConsumer(amqpURL, queueName)
		if err == nil {
			return consumer, nil
		}

		lastErr = err
		time.Sleep(delay)
	}

	return nil, fmt.Errorf("failed to connect to rabbitmq after %d retries: %w", maxRetry, lastErr)
}
