package cache

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
