package config

import (
	"os"
	"strconv"
)

type Config struct {
	LogLevel          string
	WorkerConcurrency int
}

func Load() Config {
	return Config{
		LogLevel:          getEnv("LOG_LEVEL", "info"),
		WorkerConcurrency: getEnvInt("WORKER_CONCURRENCY", 5),
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
