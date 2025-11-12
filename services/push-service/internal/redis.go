package internal

import (
	"context"
	"fmt"
	"time"

	redis "github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
)

type RedisStorage struct {
	client *redis.Client
	logger *zerolog.Logger
}

func NewRedisStorage(cfg *RedisConfig, log *zerolog.Logger) (*RedisStorage, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     cfg.URL,
		Password: cfg.Password,
		DB:       cfg.DB,
	})

	// Test connection
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to Redis: %w", err)
	}

	log.Info().Msg(fmt.Sprintf("Successfully connected to Redis, addr: %s", cfg.URL))

	return &RedisStorage{
		client: client,
		logger: log,
	}, nil
}

func (r *RedisStorage) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return r.client.Set(ctx, key, value, ttl).Err()
}

func (r *RedisStorage) Get(ctx context.Context, key string) ([]byte, error) {
	val, err := r.client.Get(ctx, key).Bytes()
	if err == redis.Nil {
		return nil, fmt.Errorf("key not found: %s", key)
	}
	return val, err
}

func (r *RedisStorage) Delete(ctx context.Context, key string) error {
	return r.client.Del(ctx, key).Err()
}

func (r *RedisStorage) Exists(ctx context.Context, key string) (bool, error) {
	count, err := r.client.Exists(ctx, key).Result()
	return count > 0, err
}

func (r *RedisStorage) SetWithExpiry(ctx context.Context, key string, value []byte, expiry time.Duration) error {
	return r.client.Set(ctx, key, value, expiry).Err()
}

func (r *RedisStorage) Increment(ctx context.Context, key string) (int64, error) {
	return r.client.Incr(ctx, key).Result()
}

func (r *RedisStorage) Close() error {
	r.logger.Info().Msg("Closing Redis connection...")
	return r.client.Close()
}
