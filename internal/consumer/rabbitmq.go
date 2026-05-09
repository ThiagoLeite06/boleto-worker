package consumer

import (
	"context"
	"fmt"

	amqp "github.com/rabbitmq/amqp091-go"
)

const (
	mainQueue = "charges.process"
	dlqQueue  = "charges.process.dlq"
)

type RabbitMQConsumer struct {
	conn    *amqp.Connection
	channel *amqp.Channel
}

func NewRabbitMQConsumer(url string) (*RabbitMQConsumer, error) {
	conn, err := amqp.Dial(url)
	if err != nil {
		return nil, fmt.Errorf("falha ao conectar no RabbitMQ: %w", err)
	}

	ch, err := conn.Channel()
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("falha ao abrir channel: %w", err)
	}

	return &RabbitMQConsumer{conn: conn, channel: ch}, nil
}

// Setup declara as filas. Pode ser chamado quantas vezes quiser — é idempotente.
func (c *RabbitMQConsumer) Setup(prefetch int) error {
	// DLQ primeiro — a fila principal referencia ela
	_, err := c.channel.QueueDeclare(dlqQueue, true, false, false, false, nil)
	if err != nil {
		return fmt.Errorf("falha ao declarar DLQ: %w", err)
	}

	args := amqp.Table{
		"x-dead-letter-exchange":    "",
		"x-dead-letter-routing-key": dlqQueue,
	}
	_, err = c.channel.QueueDeclare(mainQueue, true, false, false, false, args)
	if err != nil {
		return fmt.Errorf("falha ao declarar fila principal: %w", err)
	}

	// Prefetch: quantas mensagens cada consumer pode segurar de uma vez
	return c.channel.Qos(prefetch, 0, false)
}

// Consume retorna um channel de mensagens. O caller lê desse channel no seu próprio ritmo.
func (c *RabbitMQConsumer) Consume(ctx context.Context) (<-chan amqp.Delivery, error) {
	deliveries, err := c.channel.ConsumeWithContext(
		ctx,
		mainQueue,
		"",    // consumer tag gerada automaticamente
		false, // auto-ack desligado — confirmamos manualmente
		false, false, false,
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("falha ao iniciar consumer: %w", err)
	}

	return deliveries, nil
}

func (c *RabbitMQConsumer) Close() {
	if c.channel != nil {
		c.channel.Close()
	}
	if c.conn != nil {
		c.conn.Close()
	}
}
