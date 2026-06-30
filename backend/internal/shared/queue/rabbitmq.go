package queue

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// RabbitMQPublisher implements EventPublisher using amqp091 connection.
type RabbitMQPublisher struct {
	url      string
	conn     *amqp.Connection
	ch       *amqp.Channel
	mu       sync.RWMutex
	closed   bool
	notifyClose chan *amqp.Error
}

// NewRabbitMQPublisher connects to RabbitMQ and starts reconnection monitor.
func NewRabbitMQPublisher(host string, port int, user, password string) (*RabbitMQPublisher, error) {
	url := fmt.Sprintf("amqp://%s:%s@%s:%d/", user, password, host, port)
	p := &RabbitMQPublisher{url: url}

	if err := p.connect(); err != nil {
		return nil, err
	}

	go p.handleReconnect()

	return p, nil
}

// connect establishes connection and channel, declaring exchange.
func (p *RabbitMQPublisher) connect() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	conn, err := amqp.Dial(p.url)
	if err != nil {
		return err
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return err
	}

	// Enable publish confirmations
	if err := ch.Confirm(false); err != nil {
		ch.Close()
		conn.Close()
		return err
	}

	p.conn = conn
	p.ch = ch
	p.notifyClose = make(chan *amqp.Error)
	p.ch.NotifyClose(p.notifyClose)

	return nil
}

// handleReconnect runs in background to reconnect on connection loss.
func (p *RabbitMQPublisher) handleReconnect() {
	for {
		p.mu.RLock()
		isClosed := p.closed
		p.mu.RUnlock()

		if isClosed {
			return
		}

		err := <-p.notifyClose
		if err != nil {
			fmt.Printf("[RabbitMQ] Connection closed, reconnecting: %v\n", err)
			backoff := 1 * time.Second
			for {
				time.Sleep(backoff)
				if reconnectErr := p.connect(); reconnectErr == nil {
					fmt.Println("[RabbitMQ] Reconnected successfully")
					break
				}
				if backoff < 30*time.Second {
					backoff *= 2
				}
			}
		}
	}
}

// Publish serializes event and publishes it to the specified exchange.
func (p *RabbitMQPublisher) Publish(ctx context.Context, exchange string, routingKey string, event interface{}) error {
	p.mu.RLock()
	ch := p.ch
	p.mu.RUnlock()

	if ch == nil {
		return fmt.Errorf("rabbitmq channel is not initialized")
	}

	body, err := json.Marshal(event)
	if err != nil {
		return fmt.Errorf("failed to marshal event payload: %w", err)
	}

	// Declare exchange dynamically if needed (topic, durable, auto-delete=false)
	err = ch.ExchangeDeclare(
		exchange,
		"topic",
		true,
		false,
		false,
		false,
		nil,
	)
	if err != nil {
		return fmt.Errorf("failed to declare exchange %s: %w", exchange, err)
	}

	// Publish message with confirmation
	conf, err := ch.PublishWithDeferredConfirmWithContext(
		ctx,
		exchange,
		routingKey,
		false,
		false,
		amqp.Publishing{
			ContentType: "application/json",
			Body:        body,
			Timestamp:   time.Now(),
		},
	)
	if err != nil {
		return fmt.Errorf("failed to publish event: %w", err)
	}

	// Wait for server acknowledgement
	ok := conf.Wait()
	if !ok {
		return fmt.Errorf("failed to receive publish confirmation from rabbitmq")
	}

	return nil
}

// Close gracefully releases connections.
func (p *RabbitMQPublisher) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.closed = true

	if p.ch != nil {
		p.ch.Close()
	}
	if p.conn != nil {
		return p.conn.Close()
	}
	return nil
}
