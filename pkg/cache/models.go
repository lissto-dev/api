package cache

import "time"

// PrepareResultCache stores the result of a prepare operation
type PrepareResultCache struct {
	Namespace string                    `json:"namespace"` // For ownership verification
	Images    map[string]ImageInfoCache `json:"images"`
}

// ImageInfoCache contains the cached information about a resolved image
type ImageInfoCache struct {
	Digest string `json:"digest"`
	Image  string `json:"image"`
	URL    string `json:"url,omitempty"`
}

// ImageDigestCache stores the digest for a specific image+tag+platform combination
type ImageDigestCache struct {
	ImageURL  string    `json:"image_url"`  // Original image:tag (e.g., postgres:15.2)
	Digest    string    `json:"digest"`     // Full digest (e.g., sha256:abc123...)
	Platform  string    `json:"platform"`   // Platform (e.g., linux/amd64)
	ImageType string    `json:"image_type"` // "infra" or "service"
	CachedAt  time.Time `json:"cached_at"`  // When this was cached (for debugging)
}
