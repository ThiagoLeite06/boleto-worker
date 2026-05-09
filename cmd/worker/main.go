package main

import (
	"fmt"

	"github.com/thiagoleite/boleto-worker/internal/config"
)

func main() {
	cfg := config.Load()

	fmt.Println("BoletoWorker starting...")
	fmt.Printf("  log level   : %s\n", cfg.LogLevel)
	fmt.Printf("  concurrency : %d workers\n", cfg.WorkerConcurrency)
}
