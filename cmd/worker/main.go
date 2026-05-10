package main

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/thiagoleite/boleto-worker/internal/config"
	"github.com/thiagoleite/boleto-worker/internal/consumer"
	"github.com/thiagoleite/boleto-worker/internal/idempotency"
	"github.com/thiagoleite/boleto-worker/internal/repository"
)

func main() {
	cfg := config.Load()

	fmt.Println("BoletoWorker starting...")
	fmt.Printf("  log level   : %s\n", cfg.LogLevel)
	fmt.Printf("  concurrency : %d workers\n", cfg.WorkerConcurrency)

	// PostgreSQL
	pool, err := pgxpool.New(context.Background(), cfg.PostgresURL)
	if err != nil {
		log.Fatalf("erro ao conectar no postgres: %v", err)
	}
	defer pool.Close()

	if err := pool.Ping(context.Background()); err != nil {
		log.Fatalf("postgres não respondeu: %v", err)
	}
	fmt.Println("  postgres    : conectado")

	repo := repository.NewPostgresChargeRepository(pool)
	_ = repo

	// Redis
	idemChecker, err := idempotency.NewRedisChecker(cfg.RedisURL)
	if err != nil {
		log.Fatalf("erro ao conectar no redis: %v", err)
	}
	defer idemChecker.Close()
	fmt.Println("  redis       : conectado")

	// RabbitMQ
	c, err := consumer.NewRabbitMQConsumer(cfg.RabbitMQURL)
	if err != nil {
		log.Fatalf("erro ao conectar no RabbitMQ: %v", err)
	}
	defer c.Close()

	if err := c.Setup(cfg.WorkerConcurrency); err != nil {
		log.Fatalf("erro ao configurar filas: %v", err)
	}
	fmt.Println("  rabbitmq    : conectado")

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
