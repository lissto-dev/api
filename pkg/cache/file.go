package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// FileCache is a file-based implementation of the Cache interface with in-memory layer
// Used for development to persist cache across server restarts
type FileCache struct {
	filePath string
	data     map[string]*cacheEntry
	mu       sync.RWMutex
}

type fileCacheData struct {
	Entries map[string]*fileCacheEntry `json:"entries"`
	Version string                     `json:"version"`
}

type fileCacheEntry struct {
	Value     json.RawMessage `json:"value"`
	ExpiresAt time.Time       `json:"expires_at"`
}

// NewFileCache creates a new file-based cache with persistence
func NewFileCache(filePath string) (*FileCache, error) {
	fc := &FileCache{
		filePath: filePath,
		data:     make(map[string]*cacheEntry),
	}

	// Ensure directory exists
	dir := filepath.Dir(filePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("failed to create cache directory: %w", err)
	}

	// Load existing cache from file if it exists
	if err := fc.load(); err != nil {
		logging.Logger.Warn("Failed to load cache from file, starting with empty cache",
			zap.String("file", filePath),
			zap.Error(err))
	} else {
		logging.Logger.Info("Loaded cache from file",
			zap.String("file", filePath),
			zap.Int("entries", len(fc.data)))
	}

	// Start background cleanup and persistence goroutines
	go fc.cleanup()
	go fc.periodicSave()

	return fc, nil
}

// Set stores a value in the cache with the specified TTL
func (fc *FileCache) Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error {
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	fc.data[key] = &cacheEntry{
		value:     data,
		expiresAt: time.Now().Add(ttl),
	}

	return nil
}

// Get retrieves a value from the cache and unmarshals it into dest
func (fc *FileCache) Get(ctx context.Context, key string, dest interface{}) error {
	fc.mu.RLock()
	entry, exists := fc.data[key]
	fc.mu.RUnlock()

	if !exists {
		return ErrCacheNotFound
	}

	// Check if expired
	if time.Now().After(entry.expiresAt) {
		// Clean up expired entry
		fc.mu.Lock()
		delete(fc.data, key)
		fc.mu.Unlock()
		return ErrCacheExpired
	}

	// Unmarshal into destination
	return json.Unmarshal(entry.value, dest)
}

// cleanup runs periodically to remove expired entries
func (fc *FileCache) cleanup() {
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		now := time.Now()
		fc.mu.Lock()

		cleaned := 0
		for key, entry := range fc.data {
			if now.After(entry.expiresAt) {
				delete(fc.data, key)
				cleaned++
			}
		}

		fc.mu.Unlock()

		if cleaned > 0 {
			logging.Logger.Debug("Cleaned expired cache entries",
				zap.Int("count", cleaned))
		}
	}
}

// periodicSave saves the cache to disk every 30 seconds
func (fc *FileCache) periodicSave() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for range ticker.C {
		if err := fc.save(); err != nil {
			logging.Logger.Warn("Failed to save cache to file",
				zap.String("file", fc.filePath),
				zap.Error(err))
		}
	}
}

// load loads the cache from disk
func (fc *FileCache) load() error {
	data, err := os.ReadFile(fc.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil // File doesn't exist yet, that's ok
		}
		return err
	}

	var fileData fileCacheData
	if err := json.Unmarshal(data, &fileData); err != nil {
		return fmt.Errorf("failed to unmarshal cache file: %w", err)
	}

	fc.mu.Lock()
	defer fc.mu.Unlock()

	now := time.Now()
	loaded := 0
	expired := 0

	for key, entry := range fileData.Entries {
		// Skip expired entries during load
		if now.After(entry.ExpiresAt) {
			expired++
			continue
		}

		fc.data[key] = &cacheEntry{
			value:     entry.Value,
			expiresAt: entry.ExpiresAt,
		}
		loaded++
	}

	logging.Logger.Info("Cache loaded from disk",
		zap.Int("loaded", loaded),
		zap.Int("expired", expired))

	return nil
}

// save saves the cache to disk
func (fc *FileCache) save() error {
	fc.mu.RLock()
	defer fc.mu.RUnlock()

	// Convert to file format
	fileData := fileCacheData{
		Version: "1.0",
		Entries: make(map[string]*fileCacheEntry),
	}

	now := time.Now()
	saved := 0

	for key, entry := range fc.data {
		// Don't save expired entries
		if now.After(entry.expiresAt) {
			continue
		}

		fileData.Entries[key] = &fileCacheEntry{
			Value:     entry.value,
			ExpiresAt: entry.expiresAt,
		}
		saved++
	}

	data, err := json.MarshalIndent(fileData, "", "  ")
	if err != nil {
		return err
	}

	// Write to temp file first, then rename (atomic operation)
	tempFile := fc.filePath + ".tmp"
	if err := os.WriteFile(tempFile, data, 0644); err != nil {
		return err
	}

	if err := os.Rename(tempFile, fc.filePath); err != nil {
		return err
	}

	logging.Logger.Debug("Cache saved to disk",
		zap.String("file", fc.filePath),
		zap.Int("entries", saved))

	return nil
}

// Close saves the cache one final time before shutting down
func (fc *FileCache) Close() error {
	return fc.save()
}


