package infrastructure_test

import (
	"context"
	"testing"
	"time"

	"super-app-chonburi-go-mobile/internal/infrastructure"

	"github.com/stretchr/testify/assert"
)

func TestRedisClient_NilSafety(t *testing.T) {
	var r *infrastructure.RedisClient
	ctx := context.Background()

	// 1. Nil safety on lock acquisition
	acquired, err := r.AcquireLock(ctx, "test_lock", 5*time.Second)
	assert.NoError(t, err)
	assert.True(t, acquired)

	acquiredWithValue, err := r.AcquireLockWithValue(ctx, "test_lock", "token-123", 5*time.Second)
	assert.NoError(t, err)
	assert.True(t, acquiredWithValue)

	// 2. Nil safety on lock release
	err = r.ReleaseLock(ctx, "test_lock")
	assert.NoError(t, err)

	err = r.ReleaseLockWithValue(ctx, "test_lock", "token-123")
	assert.NoError(t, err)

	// 3. Nil safety on caching
	type SampleData struct {
		Name string `json:"name"`
	}
	var out SampleData
	err = r.GetJSON(ctx, "sample_key", &out)
	assert.Error(t, err)
	assert.Equal(t, "redis client not initialized", err.Error())

	err = r.SetJSON(ctx, "sample_key", SampleData{Name: "Chonburi"}, 5*time.Minute)
	assert.Error(t, err)
	assert.Equal(t, "redis client not initialized", err.Error())

	err = r.Delete(ctx, "sample_key")
	assert.NoError(t, err)

	assert.Nil(t, r.Raw())
	assert.NoError(t, r.Close())
}

func TestNewRedisClient_EmptyAddr(t *testing.T) {
	client, err := infrastructure.NewRedisClient("", "", 0)
	assert.NoError(t, err)
	assert.Nil(t, client)
}
