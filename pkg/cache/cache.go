package cache

import (
	"context"
	"time"
)

// Cache defines the interface for caching operations
type Cache interface {
	Set(ctx context.Context, key string, value interface{}, ttl time.Duration) error
	Get(ctx context.Context, key string, dest interface{}) error
}
