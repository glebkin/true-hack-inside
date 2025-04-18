package chain

import (
	"strings"
	"sync"
	"time"
)

type CacheKey struct {
	Question  string
	StartTime time.Time
	EndTime   time.Time
	Metrics   []string
}

func (k CacheKey) String() string {
	var b strings.Builder
	b.WriteString(k.Question)
	b.WriteString(k.StartTime.String())
	b.WriteString(k.EndTime.String())
	for _, m := range k.Metrics {
		b.WriteString(m)
	}
	return b.String()
}

func (k CacheKey) Equal(other CacheKey) bool {
	if k.Question != other.Question {
		return false
	}
	if !k.StartTime.Equal(other.StartTime) || !k.EndTime.Equal(other.EndTime) {
		return false
	}
	if len(k.Metrics) != len(other.Metrics) {
		return false
	}
	for i := range k.Metrics {
		if k.Metrics[i] != other.Metrics[i] {
			return false
		}
	}
	return true
}

type CacheEntry struct {
	Value     *LLMResponse
	ExpiresAt time.Time
}

type Cache struct {
	entries map[string]CacheEntry
	ttl     time.Duration
	mu      sync.RWMutex
}

func NewCache(ttl time.Duration) *Cache {
	return &Cache{
		entries: make(map[string]CacheEntry),
		ttl:     ttl,
	}
}

func (c *Cache) Get(key CacheKey) (*LLMResponse, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, ok := c.entries[key.String()]
	if !ok {
		return nil, false
	}

	if time.Now().After(entry.ExpiresAt) {
		c.mu.RUnlock()
		c.mu.Lock()
		delete(c.entries, key.String())
		c.mu.Unlock()
		c.mu.RLock()
		return nil, false
	}

	return entry.Value, true
}

func (c *Cache) Set(key CacheKey, value *LLMResponse) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.entries[key.String()] = CacheEntry{
		Value:     value,
		ExpiresAt: time.Now().Add(c.ttl),
	}
}

func (c *Cache) Cleanup() {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	for key, entry := range c.entries {
		if now.After(entry.ExpiresAt) {
			delete(c.entries, key)
		}
	}
}
