package cache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/saivedant169/AegisFlow/pkg/types"
	"github.com/redis/go-redis/v9"
)

type CacheStats struct {
	Hits      int64 `json:"hits"`
	Misses    int64 `json:"misses"`
	Size      int   `json:"size"`
	MaxSize   int   `json:"max_size"`
	Evictions int64 `json:"evictions"`
}

type Cache interface {
	Get(key string) (*types.ChatCompletionResponse, bool)
	Set(key string, resp *types.ChatCompletionResponse)
	Stats() CacheStats
}

// BuildKey creates a deterministic cache key from tenant + model + messages.
func BuildKey(tenantID string, model string, messages []types.Message) string {
	h := sha256.New()
	h.Write([]byte(tenantID))
	h.Write([]byte{0}) // separator to prevent collisions
	h.Write([]byte(model))
	for _, m := range messages {
		h.Write([]byte(m.Role))
		h.Write([]byte(m.Content))
	}
	return fmt.Sprintf("aegis:cache:%x", h.Sum(nil))
}

// MemoryCache is an in-memory LRU-style cache with TTL.
type MemoryCache struct {
	mu        sync.RWMutex
	entries   map[string]*cacheEntry
	ttl       time.Duration
	maxSize   int
	hits      int64
	misses    int64
	evictions int64
}

type cacheEntry struct {
	resp      *types.ChatCompletionResponse
	expiresAt time.Time
}

func NewMemoryCache(ttl time.Duration, maxSize int) *MemoryCache {
	return &MemoryCache{
		entries: make(map[string]*cacheEntry),
		ttl:     ttl,
		maxSize: maxSize,
	}
}

func (c *MemoryCache) Get(key string) (*types.ChatCompletionResponse, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()

	entry, ok := c.entries[key]
	if !ok {
		c.misses++
		return nil, false
	}
	if time.Now().After(entry.expiresAt) {
		c.misses++
		return nil, false
	}
	c.hits++
	return entry.resp, true
}

func (c *MemoryCache) Set(key string, resp *types.ChatCompletionResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Evict expired entries if at capacity
	if len(c.entries) >= c.maxSize {
		now := time.Now()
		for k, v := range c.entries {
			if now.After(v.expiresAt) {
				delete(c.entries, k)
				c.evictions++
			}
		}
	}

	// If still at capacity, evict oldest
	if len(c.entries) >= c.maxSize {
		var oldestKey string
		var oldestTime time.Time
		for k, v := range c.entries {
			if oldestKey == "" || v.expiresAt.Before(oldestTime) {
				oldestKey = k
				oldestTime = v.expiresAt
			}
		}
		delete(c.entries, oldestKey)
		c.evictions++
	}

	c.entries[key] = &cacheEntry{
		resp:      resp,
		expiresAt: time.Now().Add(c.ttl),
	}
}

func (c *MemoryCache) Stats() CacheStats {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return CacheStats{
		Hits:      c.hits,
		Misses:    c.misses,
		Size:      len(c.entries),
		MaxSize:   c.maxSize,
		Evictions: c.evictions,
	}
}

// RedisCache uses Redis as the cache backend.
type RedisCache struct {
	client *redis.Client
	ttl    time.Duration
}

func NewRedisCache(addr, password string, db int, ttl time.Duration) (*RedisCache, error) {
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := client.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis connection failed: %w", err)
	}

	return &RedisCache{client: client, ttl: ttl}, nil
}

func (c *RedisCache) Get(key string) (*types.ChatCompletionResponse, bool) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := c.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, false
	}

	var resp types.ChatCompletionResponse
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, false
	}
	return &resp, true
}

func (c *RedisCache) Set(key string, resp *types.ChatCompletionResponse) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	data, err := json.Marshal(resp)
	if err != nil {
		return
	}
	c.client.Set(ctx, key, data, c.ttl)
}

func (c *RedisCache) Stats() CacheStats {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	size, _ := c.client.DBSize(ctx).Result()
	return CacheStats{Size: int(size)}
}
