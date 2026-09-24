package infrastructure

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"time"

	"github.com/redis/go-redis/v9"
)

// Lua script for atomic unlock only if the lock owner token matches (Redlock pattern)
var releaseLockLuaScript = redis.NewScript(`
	if redis.call("get", KEYS[1]) == ARGV[1] then
		return redis.call("del", KEYS[1])
	else
		return 0
	end
`)

type RedisClient struct {
	Client *redis.Client
}

// NewRedisClient initializes a high-throughput Redis client connection pool.
// If addr is empty or connection fails, it logs gracefully without crashing the service.
func NewRedisClient(addr, password string, db int) (*RedisClient, error) {
	if addr == "" {
		log.Println("[Redis] Redis address is empty, distributed lock & L2 cache disabled")
		return nil, nil
	}

	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		Password:     password,
		DB:           db,
		PoolSize:     200,             // High throughput connection pool (matches MB v2)
		MinIdleConns: 30,              // Pre-warmed idle connections for instant response
		DialTimeout:  3 * time.Second,  // Fast fail-over dial
		ReadTimeout:  2 * time.Second,  // Strict read latency
		WriteTimeout: 2 * time.Second,  // Strict write latency
		PoolTimeout:  4 * time.Second,  // Max wait for available connection in pool
	})

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		log.Printf("⚠️ [Redis Warning] Redis ping failed during startup (%s): %v. Continuing with fallback mode.", addr, err)
		return &RedisClient{Client: client}, err
	}

	log.Printf("✅ [Redis] Connected to Redis cluster successfully (%s, DB %d, Pool 200)", addr, db)
	return &RedisClient{Client: client}, nil
}

// AcquireLock attempts to acquire a distributed lock using SETNX with standard token.
func (r *RedisClient) AcquireLock(ctx context.Context, lockKey string, ttl time.Duration) (bool, error) {
	if r == nil || r.Client == nil {
		return true, nil // Fallback gracefully if Redis is not configured
	}
	return r.Client.SetNX(ctx, lockKey, "1", ttl).Result()
}

// AcquireLockWithValue attempts to acquire a distributed lock with a specific owner token.
func (r *RedisClient) AcquireLockWithValue(ctx context.Context, lockKey, ownerToken string, ttl time.Duration) (bool, error) {
	if r == nil || r.Client == nil {
		return true, nil
	}
	return r.Client.SetNX(ctx, lockKey, ownerToken, ttl).Result()
}

// ReleaseLock releases a distributed lock by key.
func (r *RedisClient) ReleaseLock(ctx context.Context, lockKey string) error {
	if r == nil || r.Client == nil {
		return nil
	}
	return r.Client.Del(ctx, lockKey).Err()
}

// ReleaseLockWithValue releases a distributed lock safely using Lua script (verifies owner token).
func (r *RedisClient) ReleaseLockWithValue(ctx context.Context, lockKey, ownerToken string) error {
	if r == nil || r.Client == nil {
		return nil
	}
	return releaseLockLuaScript.Run(ctx, r.Client, []string{lockKey}, ownerToken).Err()
}

// GetJSON retrieves a JSON-serialized object from Redis L2 cache and unmarshals it into dest.
func (r *RedisClient) GetJSON(ctx context.Context, key string, dest interface{}) error {
	if r == nil || r.Client == nil {
		return errors.New("redis client not initialized")
	}

	val, err := r.Client.Get(ctx, key).Bytes()
	if err != nil {
		return err
	}

	return json.Unmarshal(val, dest)
}

// SetJSON marshals obj to JSON and stores it in Redis with the given TTL.
func (r *RedisClient) SetJSON(ctx context.Context, key string, obj interface{}, ttl time.Duration) error {
	if r == nil || r.Client == nil {
		return errors.New("redis client not initialized")
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return fmt.Errorf("failed to marshal cache object: %w", err)
	}

	return r.Client.Set(ctx, key, data, ttl).Err()
}

// Delete removes one or more keys from Redis.
func (r *RedisClient) Delete(ctx context.Context, keys ...string) error {
	if r == nil || r.Client == nil || len(keys) == 0 {
		return nil
	}
	return r.Client.Del(ctx, keys...).Err()
}

// Raw returns the underlying *redis.Client for custom pipelines or pub/sub.
func (r *RedisClient) Raw() *redis.Client {
	if r == nil {
		return nil
	}
	return r.Client
}

// Close gracefully closes the Redis client connection pool.
func (r *RedisClient) Close() error {
	if r == nil || r.Client == nil {
		return nil
	}
	return r.Client.Close()
}
