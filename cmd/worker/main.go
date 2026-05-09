package main

import (
	"context"
	"fmt"
	"log"

	"github.com/thiagoleite/boleto-worker/internal/config"
	"github.com/thiagoleite/boleto-worker/internal/consumer"
)

func main() {
	cfg := config.Load()

	fmt.Println("BoletoWorker starting...")
	fmt.Printf("  log level   : %s\n", cfg.LogLevel)
	fmt.Printf("  concurrency : %d workers\n", cfg.WorkerConcurrency)

	c, err := consumer.NewRabbitMQConsumer(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("erro ao conectar no RabbitMQ: %v", err)
	}
	defer c.Close()

	if err := c.Setup(cfg.WorkerConcurrency); err != nil {
		log.Fatalf("erro ao configurar filas: %v", err)
	}

	fmt.Println("aguardando mensagens. pressione CTRL+C para sair.")

	deliveries, err := c.Consume(context.Background())
	if err != nil {
		log.Fatalf("erro ao iniciar consumer: %v", err)
	}

	for msg := range deliveries {
		fmt.Printf("mensagem recebida: %s\n", string(msg.Body))
		msg.Ack(false)
	}
}
