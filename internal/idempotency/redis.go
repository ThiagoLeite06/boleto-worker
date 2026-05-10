package idempotency

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

const (
	ttlProcessing = 1 * time.Hour
	ttlCompleted  = 24 * time.Hour

	stateProcessing = "processing"
	stateCompleted  = "completed"
)

type Checker interface {
	// TryAcquire tenta reservar a chave. Retorna true se for a primeira vez.
	TryAcquire(ctx context.Context, key string) (acquired bool, err error)
	// MarkCompleted marca a chave como concluída com TTL maior.
	MarkCompleted(ctx context.Context, key string) error
	// Release remove a chave (usado em caso de erro para permitir retentativa).
	Release(ctx context.Context, key string) error
}

type RedisChecker struct {
	client *redis.Client
}

func NewRedisChecker(redisURL string) (*RedisChecker, error) {
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, fmt.Errorf("url redis inválida: %w", err)
	}
	return &RedisChecker{client: redis.NewClient(opts)}, nil
}

func (r *RedisChecker) TryAcquire(ctx context.Context, key string) (bool, error) {
	// SET key "processing" NX EX ttl
	// NX = só seta se não existir (operação atômica — sem race condition)
	ok, err := r.client.SetNX(ctx, key, stateProcessing, ttlProcessing).Result()
	if err != nil {
		// fail-open: se Redis estiver fora do ar, deixa passar
		// o Postgres ainda protege com o UNIQUE constraint
		log.Printf("WARN redis indisponível, ignorando idempotência: %v", err)
		return true, nil
	}
	return ok, nil
}

func (r *RedisChecker) MarkCompleted(ctx context.Context, key string) error {
	err := r.client.Set(ctx, key, stateCompleted, ttlCompleted).Err()
	if err != nil {
		log.Printf("WARN falha ao marcar completed no redis key=%s: %v", key, err)
	}
	return nil
}

func (r *RedisChecker) Release(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisChecker) Close() error {
	return r.client.Close()
}
