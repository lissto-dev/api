package image

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/api/internal/api/common"
	pkgcache "github.com/lissto-dev/api/pkg/cache"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// TagCandidate represents a potential image tag with its source
type TagCandidate struct {
	Tag    string
	Source string // "original", "label", "commit", "branch", "latest"
}

// ResolutionConfig contains configuration for image resolution
type ResolutionConfig struct {
	Commit            string // Git commit hash for commit-based tags
	Branch            string // Git branch name for branch-based tags
	ComposeRegistry   string // Registry from x-lissto.registry
	ComposeRepository string // Single repository from x-lissto.repository (for monorepo)
	ComposePrefix     string // Repository prefix from x-lissto.repositoryPrefix
}

// ImageResolver handles image resolution with registry/repository/tag priority
type ImageResolver struct {
	globalRegistry string
	globalPrefix   string
	imageChecker   ImageChecker
	defaultOS      string
	defaultArch    string
	cache          pkgcache.Cache // Optional cache for image digest lookups
}

// NewImageResolver creates a new image resolver
func NewImageResolver(globalRegistry, globalPrefix string, imageChecker ImageChecker) *ImageResolver {
	return &ImageResolver{
		globalRegistry: globalRegistry,
		globalPrefix:   globalPrefix,
		imageChecker:   imageChecker,
		defaultOS:      "linux",
		defaultArch:    "amd64",
		cache:          nil, // No cache by default
	}
}

// NewImageResolverWithPlatform creates a new image resolver with custom platform defaults
func NewImageResolverWithPlatform(globalRegistry, globalPrefix string, imageChecker ImageChecker, defaultOS, defaultArch string) *ImageResolver {
	return &ImageResolver{
		globalRegistry: globalRegistry,
		globalPrefix:   globalPrefix,
		imageChecker:   imageChecker,
		defaultOS:      defaultOS,
		defaultArch:    defaultArch,
		cache:          nil, // No cache by default
	}
}

// NewImageResolverWithCache creates a new image resolver with caching enabled
func NewImageResolverWithCache(globalRegistry, globalPrefix string, imageChecker ImageChecker, cache pkgcache.Cache) *ImageResolver {
	return &ImageResolver{
		globalRegistry: globalRegistry,
		globalPrefix:   globalPrefix,
		imageChecker:   imageChecker,
		defaultOS:      "linux",
		defaultArch:    "amd64",
		cache:          cache,
	}
}

// ResolveImage determines the final container image URL for a service
// Priority: lissto.dev/image (complete override) → registry + repository + tag resolution
func (ir *ImageResolver) ResolveImage(service types.ServiceConfig, config ResolutionConfig) (string, error) {
	// Step 0: Check for complete image override label (highest priority)
	if imageOverride := ir.getLabelValue(service.Labels, "lissto.dev/image", ""); imageOverride != "" {
		logging.Logger.Info("Using image override from label",
			zap.String("service", service.Name),
			zap.String("override_image", imageOverride))

		// Still validate that the image exists
		metadata, err := ir.imageChecker.CheckImageExists(imageOverride)
		if err == nil && metadata.Exists {
			logging.Logger.Info("Image override validated successfully",
				zap.String("image", imageOverride),
				zap.String("service", service.Name))
			return imageOverride, nil
		}

		logging.Logger.Warn("Image override label specified but image not found",
			zap.String("image", imageOverride),
			zap.String("service", service.Name),
			zap.Error(err))
		return "", fmt.Errorf("image override '%s' for service %s not found: %w", imageOverride, service.Name, err)
	}

	// Step 1: Resolve registry
	registry := ir.ResolveRegistryWithCompose(service, config.ComposeRegistry)

	// Step 2: Resolve image name
	imageName := ir.ResolveImageNameWithCompose(service, config.ComposeRepository, config.ComposePrefix)

	// Step 3: Resolve tag candidates
	tagCandidates := ir.resolveTag(service, config.Commit, config.Branch)

	// Step 4: Check existence for each candidate
	for _, candidate := range tagCandidates {
		var imageURL string
		if registry != "" {
			imageURL = fmt.Sprintf("%s/%s:%s", registry, imageName, candidate.Tag)
		} else {
			imageURL = fmt.Sprintf("%s:%s", imageName, candidate.Tag)
		}

		// Check if image exists
		metadata, err := ir.imageChecker.CheckImageExists(imageURL)
		if err == nil && metadata.Exists {
			logging.Logger.Info("Found existing image",
				zap.String("image", imageURL),
				zap.String("tag_source", candidate.Source),
				zap.String("service", service.Name))
			return imageURL, nil
		}

		logging.Logger.Debug("Image not found, trying next candidate",
			zap.String("image", imageURL),
			zap.String("tag_source", candidate.Source),
			zap.String("service", service.Name))
	}

	return "", fmt.Errorf("no existing image found for service %s", service.Name)
}

// ResolveRegistryWithCompose determines the registry for a service with compose-level config
// Priority: Service label → Compose registry (x-lissto) → Global registry → No registry
func (ir *ImageResolver) ResolveRegistryWithCompose(service types.ServiceConfig, composeRegistry string) string {
	// Service-specific label always takes precedence
	if registry := ir.getLabelValue(service.Labels, "lissto.dev/registry", ""); registry != "" {
		return registry
	}
	// Check compose-level registry from x-lissto
	if composeRegistry != "" {
		return composeRegistry
	}
	// Fall back to global config
	if ir.globalRegistry != "" {
		return ir.globalRegistry
	}
	return ""
}

// ResolveImageNameWithCompose determines the image name for a service with compose-level config
// Priority: Service label → Compose repository (x-lissto.repository) → Compose prefix (x-lissto.repositoryPrefix) + service name → Global prefix + service name → Service name
func (ir *ImageResolver) ResolveImageNameWithCompose(service types.ServiceConfig, composeRepository, composePrefix string) string {
	// Service-specific label always takes precedence
	if repo := ir.getLabelValue(service.Labels, "lissto.dev/repository", ""); repo != "" {
		return repo
	}
	// Check compose-level repository (single image for all services)
	if composeRepository != "" {
		return composeRepository
	}
	// Check compose-level prefix from x-lissto
	if composePrefix != "" {
		return composePrefix + service.Name
	}
	// Fall back to global prefix + service name
	if ir.globalPrefix != "" {
		return ir.globalPrefix + service.Name
	}
	// Final fallback: just service name
	return service.Name
}

// resolveTag determines tag candidates in priority order
// Priority: Original → Labels → commit → branch → latest
func (ir *ImageResolver) resolveTag(service types.ServiceConfig, commit, branch string) []TagCandidate {
	candidates := make([]TagCandidate, 0)

	// Priority 0: Original tag from docker-compose image field
	// Extract tag from service.Image (e.g., "nginx:alpine" -> "alpine")
	if originalTag := ir.extractOriginalTag(service.Image); originalTag != "" {
		candidates = append(candidates, TagCandidate{Tag: originalTag, Source: "original"})
	}

	// Priority 1: Custom tag from label
	if tag := ir.getLabelValue(service.Labels, "lissto.dev/tag", ""); tag != "" {
		candidates = append(candidates, TagCandidate{Tag: tag, Source: "label"})
	}

	// Priority 2: Commit-based tag
	if commit != "" {
		candidates = append(candidates, TagCandidate{Tag: commit, Source: "commit"})
	}

	// Priority 3: Branch-based tag
	if branch != "" {
		candidates = append(candidates, TagCandidate{Tag: branch, Source: "branch"})
	}

	// Priority 4: Latest
	candidates = append(candidates, TagCandidate{Tag: "latest", Source: "latest"})

	return candidates
}

// extractOriginalTag extracts the tag from the original docker-compose image field
// Examples:
//   - "nginx:alpine" -> "alpine"
//   - "registry.io/org/repo:v1.2.3" -> "v1.2.3"
//   - "nginx@sha256:abc123" -> "" (digest, no tag)
//   - "nginx" -> "" (no tag specified)
func (ir *ImageResolver) extractOriginalTag(image string) string {
	if image == "" {
		return ""
	}

	// If image has a digest (@sha256:...), there's no tag to extract
	if strings.Contains(image, "@") {
		return ""
	}

	// Find the last colon (separates tag from repository)
	lastColonIndex := strings.LastIndex(image, ":")

	// If no colon found, no tag specified
	if lastColonIndex == -1 {
		return ""
	}

	// Extract the tag part after the colon
	tag := image[lastColonIndex+1:]

	// Validate that the tag doesn't look like a port number (for registry URLs)
	// Port numbers are typically followed by a slash (e.g., "registry:5000/repo")
	if strings.Contains(tag, "/") {
		// This is likely a port number, not a tag
		return ""
	}

	// Return the extracted tag
	return tag
}

// getLabelValue safely extracts a label value from service labels
func (ir *ImageResolver) getLabelValue(labels map[string]string, key, defaultValue string) string {
	if labels == nil {
		return defaultValue
	}
	if value, exists := labels[key]; exists {
		return value
	}
	return defaultValue
}

// ImageResolutionResult contains minimal resolution info
type ImageResolutionResult struct {
	FinalImage string // Image with digest
	Method     string // How it was resolved
	Selected   string // Which candidate worked (empty if first try)
}

// DetailedImageResolutionResult contains detailed resolution info with all candidates
type DetailedImageResolutionResult struct {
	FinalImage string                  // Image with digest
	Method     string                  // How it was resolved
	Selected   string                  // Which candidate worked (empty if first try)
	Registry   string                  // Registry used
	ImageName  string                  // Image name resolved
	Candidates []common.ImageCandidate // All candidates that were tried
}

// ResolveImageWithCandidates tries multiple candidates, returns which worked
// Priority: lissto.dev/image (complete override) → registry + repository + tag resolution
func (ir *ImageResolver) ResolveImageWithCandidates(
	service types.ServiceConfig,
	config ResolutionConfig,
) (*ImageResolutionResult, error) {
	// Step 0: Check for complete image override label (highest priority)
	if imageOverride := ir.getLabelValue(service.Labels, "lissto.dev/image", ""); imageOverride != "" {
		logging.Logger.Info("Using image override from label",
			zap.String("service", service.Name),
			zap.String("override_image", imageOverride))

		// Try to get image with digest using service-specific platform
		imageWithDigest, err := ir.GetImageDigestWithServicePlatform(imageOverride, service)
		if err == nil {
			logging.Logger.Info("Image override resolved successfully with digest",
				zap.String("image", imageWithDigest),
				zap.String("service", service.Name))

			return &ImageResolutionResult{
				FinalImage: imageWithDigest,
				Method:     "override",
				Selected:   imageOverride,
			}, nil
		}

		logging.Logger.Warn("Image override label specified but image not found",
			zap.String("image", imageOverride),
			zap.String("service", service.Name),
			zap.Error(err))
		return nil, fmt.Errorf("image override '%s' for service %s not found: %w", imageOverride, service.Name, err)
	}

	// Step 1: Resolve registry
	registry := ir.ResolveRegistryWithCompose(service, config.ComposeRegistry)

	// Step 2: Resolve image name
	imageName := ir.ResolveImageNameWithCompose(service, config.ComposeRepository, config.ComposePrefix)

	// Step 3: Resolve tag candidates
	tagCandidates := ir.resolveTag(service, config.Commit, config.Branch)

	logging.Logger.Info("Resolving image with candidates",
		zap.String("service", service.Name),
		zap.String("registry", registry),
		zap.String("image_name", imageName),
		zap.String("commit", config.Commit),
		zap.String("branch", config.Branch),
		zap.Int("candidates_count", len(tagCandidates)))

	// Log all candidates that will be tried
	for i, candidate := range tagCandidates {
		var imageURL string
		if registry != "" {
			imageURL = fmt.Sprintf("%s/%s:%s", registry, imageName, candidate.Tag)
		} else {
			imageURL = fmt.Sprintf("%s:%s", imageName, candidate.Tag)
		}
		logging.Logger.Info("Image candidate",
			zap.String("service", service.Name),
			zap.Int("candidate_index", i),
			zap.String("tag", candidate.Tag),
			zap.String("source", candidate.Source),
			zap.String("full_image_url", imageURL))
	}

	// Step 4: Check existence for each candidate
	for _, candidate := range tagCandidates {
		var imageURL string
		if registry != "" {
			imageURL = fmt.Sprintf("%s/%s:%s", registry, imageName, candidate.Tag)
		} else {
			imageURL = fmt.Sprintf("%s:%s", imageName, candidate.Tag)
		}

		// Try to get image with digest using service-specific platform
		logging.Logger.Info("Trying image candidate",
			zap.String("service", service.Name),
			zap.String("candidate_url", imageURL),
			zap.String("tag_source", candidate.Source))

		imageWithDigest, err := ir.GetImageDigestWithServicePlatform(imageURL, service)
		if err == nil {
			logging.Logger.Info("Found existing image",
				zap.String("image", imageWithDigest),
				zap.String("tag_source", candidate.Source),
				zap.String("service", service.Name))

			return &ImageResolutionResult{
				FinalImage: imageWithDigest,
				Method:     candidate.Source,
				Selected:   imageURL,
			}, nil
		}

		logging.Logger.Info("Image not found, trying next candidate",
			zap.String("image", imageURL),
			zap.String("tag_source", candidate.Source),
			zap.String("service", service.Name),
			zap.Error(err))
	}

	return nil, fmt.Errorf("no existing image found for service %s", service.Name)
}

// ResolveImageDetailed tries multiple candidates and returns detailed info about all attempts
func (ir *ImageResolver) ResolveImageDetailed(
	service types.ServiceConfig,
	config ResolutionConfig,
) (*DetailedImageResolutionResult, error) {
	// Step 1: Resolve registry
	registry := ir.ResolveRegistryWithCompose(service, config.ComposeRegistry)

	// Step 2: Resolve image name
	imageName := ir.ResolveImageNameWithCompose(service, config.ComposeRepository, config.ComposePrefix)

	// Step 3: Resolve tag candidates
	tagCandidates := ir.resolveTag(service, config.Commit, config.Branch)

	logging.Logger.Info("Resolving image with detailed candidates",
		zap.String("service", service.Name),
		zap.String("registry", registry),
		zap.String("image_name", imageName),
		zap.String("commit", config.Commit),
		zap.String("branch", config.Branch),
		zap.Int("candidates_count", len(tagCandidates)))

	// Track all candidates
	candidates := make([]common.ImageCandidate, 0, len(tagCandidates))
	var finalImage, method, selected string

	// Step 4: Check existence for each candidate
	for _, candidate := range tagCandidates {
		var imageURL string
		if registry != "" {
			imageURL = fmt.Sprintf("%s/%s:%s", registry, imageName, candidate.Tag)
		} else {
			imageURL = fmt.Sprintf("%s:%s", imageName, candidate.Tag)
		}

		logging.Logger.Info("Trying image candidate",
			zap.String("service", service.Name),
			zap.String("candidate_url", imageURL),
			zap.String("tag_source", candidate.Source))

		// Try to get image with digest using service-specific platform
		imageWithDigest, err := ir.GetImageDigestWithServicePlatform(imageURL, service)

		candidateResult := common.ImageCandidate{
			ImageURL: imageURL,
			Tag:      candidate.Tag,
			Source:   candidate.Source,
			Success:  err == nil,
		}

		if err == nil {
			candidateResult.Digest = imageWithDigest
			finalImage = imageWithDigest
			method = candidate.Source
			selected = imageURL

			logging.Logger.Info("Found existing image",
				zap.String("image", imageWithDigest),
				zap.String("tag_source", candidate.Source),
				zap.String("service", service.Name))
		} else {
			candidateResult.Error = err.Error()
			logging.Logger.Info("Image not found, trying next candidate",
				zap.String("image", imageURL),
				zap.String("tag_source", candidate.Source),
				zap.String("service", service.Name),
				zap.Error(err))
		}

		candidates = append(candidates, candidateResult)

		// If we found a working image, we can stop here
		if err == nil {
			break
		}
	}

	if finalImage == "" {
		return &DetailedImageResolutionResult{
			FinalImage: "",
			Method:     "",
			Selected:   "",
			Registry:   registry,
			ImageName:  imageName,
			Candidates: candidates,
		}, fmt.Errorf("no existing image found for service %s", service.Name)
	}

	return &DetailedImageResolutionResult{
		FinalImage: finalImage,
		Method:     method,
		Selected:   selected,
		Registry:   registry,
		ImageName:  imageName,
		Candidates: candidates,
	}, nil
}

// GetImageDigest resolves an image URL to its digest
func (ir *ImageResolver) GetImageDigest(imageURL string) (string, error) {
	// Use default platform for backward compatibility
	return ir.GetImageDigestForPlatform(imageURL, ir.defaultOS, ir.defaultArch)
}

// GetImageDigestForPlatform resolves an image URL to its digest for a specific platform
func (ir *ImageResolver) GetImageDigestForPlatform(imageURL, os, arch string) (string, error) {
	metadata, err := ir.imageChecker.CheckImageExistsForPlatform(imageURL, os, arch)
	if err != nil || !metadata.Exists {
		return "", fmt.Errorf("image not found: %s", imageURL)
	}

	// Check if we have a digest
	if metadata.Digest == "" {
		logging.Logger.Warn("Image exists but digest unavailable",
			zap.String("image", imageURL),
			zap.String("platform", os+"/"+arch))
		// Return the image without digest - this is acceptable for some use cases
		return imageURL, nil
	}

	// Return image with digest-only format (strip tag)
	return ir.formatImageWithDigest(imageURL, metadata.Digest), nil
}

// GetImageDigestWithCacheContext resolves an image URL to its digest with caching support
// Uses service context to determine if it's an infra or service image for cache TTL decisions
func (ir *ImageResolver) GetImageDigestWithCacheContext(imageURL, os, arch string, service types.ServiceConfig) (string, error) {
	// If no cache is configured, fall back to non-cached behavior
	if ir.cache == nil {
		return ir.GetImageDigestForPlatform(imageURL, os, arch)
	}

	ctx := context.Background()
	isInfra := IsInfraImage(service)
	imageType := GetImageType(isInfra)

	// Check if this image should be cached
	if !ShouldCache(isInfra, imageURL) {
		logging.Logger.Debug("Image not cacheable, skipping cache",
			zap.String("image", imageURL),
			zap.String("image_type", imageType),
			zap.String("platform", os+"/"+arch))
		return ir.GetImageDigestForPlatform(imageURL, os, arch)
	}

	// Check cache first
	cacheKey := GetCacheKey(imageURL, os, arch)
	var cachedEntry pkgcache.ImageDigestCache

	err := ir.cache.Get(ctx, cacheKey, &cachedEntry)
	if err == nil {
		// Cache hit!
		logging.Logger.Info("Image digest cache HIT",
			zap.String("image", imageURL),
			zap.String("image_type", imageType),
			zap.String("platform", os+"/"+arch),
			zap.String("digest", cachedEntry.Digest),
			zap.Time("cached_at", cachedEntry.CachedAt))
		return cachedEntry.Digest, nil
	}

	// Cache miss - log it
	logging.Logger.Debug("Image digest cache MISS",
		zap.String("image", imageURL),
		zap.String("image_type", imageType),
		zap.String("platform", os+"/"+arch))

	// Fetch from registry
	digest, err := ir.GetImageDigestForPlatform(imageURL, os, arch)
	if err != nil {
		return "", err
	}

	// Store in cache with appropriate TTL
	ttl := GetTTL(isInfra, imageURL)
	if ttl > 0 {
		cacheEntry := pkgcache.ImageDigestCache{
			ImageURL:  imageURL,
			Digest:    digest,
			Platform:  fmt.Sprintf("%s/%s", os, arch),
			ImageType: imageType,
			CachedAt:  time.Now(),
		}

		if err := ir.cache.Set(ctx, cacheKey, cacheEntry, ttl); err != nil {
			// Log error but don't fail - cache is optional
			logging.Logger.Warn("Failed to cache image digest",
				zap.String("image", imageURL),
				zap.Error(err))
		} else {
			logging.Logger.Info("Cached image digest",
				zap.String("image", imageURL),
				zap.String("image_type", imageType),
				zap.String("platform", os+"/"+arch),
				zap.Duration("ttl", ttl))
		}
	}

	return digest, nil
}

// GetImageDigestWithServicePlatform resolves an image URL to its digest using service-specific platform configuration
func (ir *ImageResolver) GetImageDigestWithServicePlatform(imageURL string, service types.ServiceConfig) (string, error) {
	os, arch := ir.getPlatformFromService(service)

	// If cache is available, use the cache-aware method
	if ir.cache != nil {
		return ir.GetImageDigestWithCacheContext(imageURL, os, arch, service)
	}

	// Otherwise use the standard method
	return ir.GetImageDigestForPlatform(imageURL, os, arch)
}

// getPlatformFromService extracts platform configuration from service labels or uses defaults
func (ir *ImageResolver) getPlatformFromService(service types.ServiceConfig) (string, string) {
	os := ir.getLabelValue(service.Labels, "lissto.dev/platform-os", ir.defaultOS)
	arch := ir.getLabelValue(service.Labels, "lissto.dev/platform-arch", ir.defaultArch)
	return os, arch
}

// formatImageWithDigest formats an image URL with digest, removing any existing tag
// Converts "nginx:latest" + "sha256:abc123" to "nginx@sha256:abc123"
func (ir *ImageResolver) formatImageWithDigest(imageURL, digest string) string {
	// Split the image URL to separate registry/repository from tag
	// Handle formats like:
	// - "nginx:latest" -> "nginx@sha256:abc123"
	// - "registry.com/nginx:latest" -> "registry.com/nginx@sha256:abc123"
	// - "registry.com/namespace/nginx:latest" -> "registry.com/namespace/nginx@sha256:abc123"

	// First check if the image already has a digest (contains @)
	if atIndex := strings.LastIndex(imageURL, "@"); atIndex != -1 {
		// Image already has a digest, replace it
		imageWithoutDigest := imageURL[:atIndex]
		return fmt.Sprintf("%s@%s", imageWithoutDigest, digest)
	}

	// Find the last colon that separates the tag from the repository
	// We need to be careful about port numbers in registry URLs
	lastColonIndex := -1

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
				lastColonIndex = i
				break
			}
		}
	}

	if lastColonIndex == -1 {
		// No tag found, just append digest
		return fmt.Sprintf("%s@%s", imageURL, digest)
	}

	// Remove the tag and append digest
	imageWithoutTag := imageURL[:lastColonIndex]
	return fmt.Sprintf("%s@%s", imageWithoutTag, digest)
}
