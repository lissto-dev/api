package image

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// Semantic versioning pattern (e.g., 1.2.3, v1.2.3, 2.0.1-alpha, etc.)
var semverPattern = regexp.MustCompile(`^v?\d+\.\d+(\.\d+)?(-[a-zA-Z0-9.-]+)?$`)

// IsInfraImage determines if a service is an infrastructure image (has image but no build)
func IsInfraImage(service types.ServiceConfig) bool {
	// Infrastructure images have explicit image field and NO build section
	return service.Image != "" && service.Build == nil
}

// IsSemverTag checks if a tag matches semantic versioning pattern
func IsSemverTag(imageURL string) bool {
	tag := extractTag(imageURL)
	if tag == "" {
		return false
	}

	// Extract just the version part if tag has additional suffix like -alpine
	// e.g., "1.2.3-alpine" -> check "1.2.3"
	parts := strings.Split(tag, "-")
	if len(parts) > 0 && semverPattern.MatchString(parts[0]) {
		return true
	}

	// Also check the full tag (e.g., "v1.2.3-rc1")
	return semverPattern.MatchString(tag)
}

// extractTag extracts the tag portion from an image URL
// Examples:
//   - "postgres:15.2" -> "15.2"
//   - "redis:7.0-alpine" -> "7.0-alpine"
//   - "nginx" -> ""
//   - "registry.com:5000/nginx:latest" -> "latest"
func extractTag(imageURL string) string {
	// Remove digest if present (e.g., nginx@sha256:abc -> nginx)
	if idx := strings.LastIndex(imageURL, "@"); idx != -1 {
		imageURL = imageURL[:idx]
	}

	// Find the last colon that's not part of a port number
	// Start from the end and work backwards
	for i := len(imageURL) - 1; i >= 0; i-- {
		if imageURL[i] == ':' {
			// Check if this colon is part of a port number
			// Port numbers are typically after a slash and before another slash or end
			isPort := false

			// Look backwards for a slash to see if this colon is after a registry host
			for j := i - 1; j >= 0; j-- {
				if imageURL[j] == '/' {
					// Found a slash before the colon, check if next chars are digits
					if i+1 < len(imageURL) {
						nextChar := imageURL[i+1]
						if nextChar >= '0' && nextChar <= '9' {
							isPort = true
						}
					}
					break
				}
			}

			if !isPort {
				// This colon is likely separating tag from repository
				return imageURL[i+1:]
			}
		}
	}

	return ""
}

// GetCacheKey generates a consistent cache key for an image+platform combination
func GetCacheKey(imageURL, os, arch string) string {
	return fmt.Sprintf("%s@%s/%s", imageURL, os, arch)
}

// ShouldCache determines if an image should be cached based on its type and tag
// Returns true if the image should be cached
func ShouldCache(isInfraImage bool, imageURL string) bool {
	if isInfraImage {
		// Infrastructure images: cache all tags for 24h
		return true
	}

	// Service images: only cache semver tags
	return IsSemverTag(imageURL)
}

// GetTTL returns the appropriate TTL for an image based on its type and tag
// Returns 0 if the image should not be cached
func GetTTL(isInfraImage bool, imageURL string) time.Duration {
	if !ShouldCache(isInfraImage, imageURL) {
		return 0
	}

	if isInfraImage {
		// Infrastructure images: 24h for all tags
		logging.Logger.Debug("Cache TTL for infrastructure image",
			zap.String("image", imageURL),
			zap.Duration("ttl", 24*time.Hour))
		return 24 * time.Hour
	}

	// Service images with semver: 1h
	if IsSemverTag(imageURL) {
		logging.Logger.Debug("Cache TTL for service semver image",
			zap.String("image", imageURL),
			zap.Duration("ttl", 1*time.Hour))
		return 1 * time.Hour
	}

	return 0
}

// GetImageType returns a string representation of the image type for logging/caching
func GetImageType(isInfraImage bool) string {
	if isInfraImage {
		return "infra"
	}
	return "service"
}
