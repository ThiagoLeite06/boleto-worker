package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	RabbitMQURL         string
	PostgresURL         string
	RedisURL            string
	WorkerConcurrency   int
	ShutdownTimeout     time.Duration
	LogLevel            string
	BoletoAPIURL        string
	WebhookURL          string
	WebhookSecret       string
}

func Load() Config {
	return Config{
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		PostgresURL:       getEnv("POSTGRES_URL", "postgres://bw_user:bw_pass@localhost:5432/boletoworker?sslmode=disable"),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379/0"),
		WorkerConcurrency: getEnvInt("WORKER_CONCURRENCY", 5),
		ShutdownTimeout:   getEnvDuration("SHUTDOWN_TIMEOUT", 30*time.Second),
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		BoletoAPIURL:      getEnv("BOLETO_API_URL", "http://localhost:8081"),
		WebhookURL:        getEnv("WEBHOOK_URL", "http://localhost:9000/webhook"),
		WebhookSecret:     getEnv("WEBHOOK_SECRET", "secret"),
	}
}

func getEnv(key, defaultValue string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultValue
}

func getEnvInt(key string, defaultValue int) int {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return defaultValue
	}
	return n
}

func getEnvDuration(key string, defaultValue time.Duration) time.Duration {
	v := os.Getenv(key)
	if v == "" {
		return defaultValue
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return defaultValue
	}
	return d
}
