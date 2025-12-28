package cache

import (
	"os"

	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// NewImageCache creates the appropriate cache for image digests
// If IMAGE_CACHE_FILE_PATH env var is set, uses file-based cache (for dev)
// Otherwise uses in-memory cache (for production)
func NewImageCache() Cache {
	cacheFilePath := os.Getenv("IMAGE_CACHE_FILE_PATH")
	if cacheFilePath != "" {
		fileCache, err := NewFileCache(cacheFilePath)
		if err != nil {
			logging.Logger.Warn("Failed to create file-based image cache, falling back to memory cache",
				zap.String("path", cacheFilePath),
				zap.Error(err))
			return NewMemoryCache()
		}
		logging.Logger.Info("Initialized file-based image cache for development",
			zap.String("path", cacheFilePath))
		return fileCache
	}

	logging.Logger.Info("Initialized in-memory image cache")
	return NewMemoryCache()
}
