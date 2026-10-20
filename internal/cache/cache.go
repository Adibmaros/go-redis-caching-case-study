package cache

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"math/rand"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/sync/singleflight"
)

type CacheStatus string

const (
	CacheHit    CacheStatus = "HIT"
	CacheMiss   CacheStatus = "MISS"
	CacheBypass CacheStatus = "BYPASS"
)

type Manager struct {
	client  *redis.Client
	enabled bool
	sfGroup singleflight.Group
}

func NewManager(client *redis.Client, enabled bool) *Manager {
	return &Manager{
		client:  client,
		enabled: enabled,
	}
}

func (m *Manager) IsEnabled() bool {
	return m.enabled && m.client != nil
}

func (m *Manager) Client() *redis.Client {
	return m.client
}

// GetJSON retrieves a JSON-marshaled item from Redis.
// Returns (value, true, nil) on cache hit.
// Returns (zero, false, nil) on cache miss or when Redis fails (graceful degradation).
func GetJSON[T any](ctx context.Context, m *Manager, key string) (T, bool, error) {
	var zero T
	if !m.IsEnabled() {
		return zero, false, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	val, err := m.client.Get(timeoutCtx, key).Result()
	if err != nil {
		if errors.Is(err, redis.Nil) {
			return zero, false, nil
		}
		// Failure tolerance: log warning and fallback to MySQL
		slog.Warn("redis error on GetJSON, falling back to database", "key", key, "error", err)
		return zero, false, nil
	}

	var data T
	if err := json.Unmarshal([]byte(val), &data); err != nil {
		slog.Warn("redis unmarshal error, falling back to database", "key", key, "error", err)
		return zero, false, nil
	}

	return data, true, nil
}

// SetJSON stores a value as a JSON string with TTL and jitter (+/- 10%).
func SetJSON(ctx context.Context, m *Manager, key string, value any, ttl time.Duration) error {
	if !m.IsEnabled() {
		return nil
	}

	bytes, err := json.Marshal(value)
	if err != nil {
		return fmt.Errorf("failed to marshal cache value: %w", err)
	}

	jitteredTTL := applyJitter(ttl, 0.10)

	timeoutCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	if err := m.client.Set(timeoutCtx, key, bytes, jitteredTTL).Err(); err != nil {
		slog.Warn("redis error on SetJSON, ignoring cache write failure", "key", key, "error", err)
		return nil
	}

	return nil
}

// Delete removes one or more keys from Redis.
func (m *Manager) Delete(ctx context.Context, keys ...string) error {
	if !m.IsEnabled() || len(keys) == 0 {
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	if err := m.client.Del(timeoutCtx, keys...).Err(); err != nil {
		slog.Warn("redis error on Delete", "keys", keys, "error", err)
		return nil
	}

	return nil
}

// IncrVersion increments a version counter in Redis.
func (m *Manager) IncrVersion(ctx context.Context, key string) (int64, error) {
	if !m.IsEnabled() {
		return 1, nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	val, err := m.client.Incr(timeoutCtx, key).Result()
	if err != nil {
		slog.Warn("redis error on IncrVersion", "key", key, "error", err)
		return 1, nil
	}

	return val, nil
}

// GetVersion gets the current version counter or returns 1 on miss/error.
func (m *Manager) GetVersion(ctx context.Context, key string) int64 {
	if !m.IsEnabled() {
		return 1
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()

	val, err := m.client.Get(timeoutCtx, key).Int64()
	if err != nil {
		return 1
	}

	return val
}

// FlushDB flushes the currently selected Redis database (used for testing/benchmarks).
func (m *Manager) FlushDB(ctx context.Context) error {
	if !m.IsEnabled() {
		return nil
	}

	timeoutCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	return m.client.FlushDB(timeoutCtx).Err()
}

// SingleFlight prevents cache stampede when multiple requests miss the same key simultaneously.
func (m *Manager) SingleFlight(key string, fn func() (any, error)) (any, error) {
	val, err, _ := m.sfGroup.Do(key, fn)
	return val, err
}

// applyJitter adds/subtracts up to factor percent of the original duration to avoid thundering herd.
func applyJitter(duration time.Duration, factor float64) time.Duration {
	if duration <= 0 || factor <= 0 {
		return duration
	}
	delta := float64(duration) * factor
	minDuration := float64(duration) - delta
	spread := 2 * delta
	randomOffset := rand.Float64() * spread
	return time.Duration(minDuration + randomOffset)
}
