package main

import (
	"fmt"

	"github.com/thiagoleite/boleto-worker/internal/config"
)

func main() {
	cfg := config.Load()

	fmt.Println("BoletoWorker starting...")
	fmt.Printf("  concurrency : %d workers\n", cfg.WorkerConcurrency)
	fmt.Printf("  shutdown    : %s\n", cfg.ShutdownTimeout)
	fmt.Printf("  log level   : %s\n", cfg.LogLevel)
	fmt.Printf("  rabbitmq    : %s\n", cfg.RabbitMQURL)
	fmt.Printf("  postgres    : %s\n", cfg.PostgresURL)
	fmt.Printf("  redis       : %s\n", cfg.RedisURL)
	fmt.Printf("  boleto api  : %s\n", cfg.BoletoAPIURL)
}
