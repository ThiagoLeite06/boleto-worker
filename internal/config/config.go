package config

import (
	"os"
	"strconv"
)

type Config struct {
	LogLevel          string
	WorkerConcurrency int
	RabbitMQURL       string
	PostgresURL       string
	RedisURL          string
}

func Load() Config {
	return Config{
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		WorkerConcurrency: getEnvInt("WORKER_CONCURRENCY", 5),
		RabbitMQURL:       getEnv("RABBITMQ_URL", "amqp://guest:guest@localhost:5672/"),
		PostgresURL:       getEnv("POSTGRES_URL", "postgres://bw_user:bw_pass@localhost:5432/boletoworker?sslmode=disable"),
		RedisURL:          getEnv("REDIS_URL", "redis://localhost:6379/0"),
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
