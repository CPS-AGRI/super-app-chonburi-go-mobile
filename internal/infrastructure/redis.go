package infrastructure

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	Client *redis.Client
}

func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     20,
		MinIdleConns: 5,
		DialTimeout:  5 * time.Second,
		ReadTimeout:  3 * time.Second,
		WriteTimeout: 3 * time.Second,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("failed to connect to redis: %w", err)
	}

	return &RedisClient{Client: client}, nil
}

// Distributed Lock helper using SETNX
func (r *RedisClient) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	return r.Client.SetNX(ctx, lockKey, "1", ttl).Result()
}

func (r *RedisClient) ReleaseLock(ctx context.Context, lockKey string) error {
	return r.Client.Del(ctx, lockKey).Err()
}
