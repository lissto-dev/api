package cache

import (
	"context"
	"encoding/json"
	"errors"
	"sync"
	"time"
)

var (
	ErrCacheNotFound = errors.New("cache entry not found")
	ErrCacheExpired  = errors.New("cache entry expired")
)

// cacheEntry represents a single cache entry with expiration
type cacheEntry struct {
	value     []byte // JSON-encoded value
	expiresAt time.Time
}

// MemoryCache is an in-memory implementation of the Cache interface
type MemoryCache struct {
	data map[string]*cacheEntry
	mu   sync.RWMutex
}

// NewMemoryCache creates a new in-memory cache with background cleanup
func NewMemoryCache() *MemoryCache {
	cache := &MemoryCache{
		data: make(map[string]*cacheEntry),
	}

	// Start background cleanup goroutine
	go cache.cleanup()

	return cache
}

// Set stores a value in the cache with the specified TTL
func (m *MemoryCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	m.data[key] = &cacheEntry{
		value:     data,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Get retrieves a value from the cache and unmarshals it into dest
func (m *MemoryCache) Get(ctx context.Context, key string, dest interface{}) error {
	m.mu.RLock()
	entry, exists := m.data[key]
	m.mu.RUnlock()

	if !exists {
		return ErrCacheNotFound
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Clean up expired entry
		m.mu.Lock()
		delete(m.data, key)
		m.mu.Unlock()
		return ErrCacheExpired
	}

	// Unmarshal into destination
	return json.Unmarshal(entry.value, dest)
}

// cleanup runs periodically to remove expired entries
func (m *MemoryCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		m.mu.Lock()

		for key, entry := range m.data {
			if now.After(entry.expiresAt) {
				delete(m.data, key)
			}
		}

		m.mu.Unlock()
	}
}
