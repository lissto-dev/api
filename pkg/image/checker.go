package image

import (
	"context"
	"fmt"
	"runtime"
	"strings"

	"github.com/containers/image/v5/docker"
	"github.com/containers/image/v5/image"
	"github.com/containers/image/v5/manifest"
	"github.com/containers/image/v5/types"
	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// ImageMetadata contains information about an image
type ImageMetadata struct {
	Exists          bool
	Digest          string
	Manifest        []byte
	Config          []byte
	Architectures   []string          // List of available architectures
	PlatformDigests map[string]string // Digest per platform (e.g., "linux/amd64": "sha256:...")
	IsMultiArch     bool              // Flag indicating manifest list vs single manifest
	ManifestType    string            // Type of manifest retrieved
}

// ImageExistenceChecker checks if container images exist in registries
type ImageExistenceChecker struct {
	systemContext *types.SystemContext
	keychain      authn.Keychain // Optional K8s keychain for authenticated access
}

// NewImageExistenceChecker creates a new image existence checker with anonymous access
func NewImageExistenceChecker() *ImageExistenceChecker {
	return &ImageExistenceChecker{
		systemContext: &types.SystemContext{
			// Use default authentication and TLS settings
			DockerInsecureSkipTLSVerify: types.OptionalBoolFalse,
		},
		keychain: nil, // No authentication
	}
}

// NewImageExistenceCheckerWithK8sAuth creates a new image existence checker with K8s authentication
// This uses k8schain to automatically discover credentials from:
// - Image pull secrets attached to the pod's service account
// - Node IAM credentials (AWS ECR, GCP Workload Identity, etc.)
// - Docker config files and credential helpers
// Falls back to anonymous access if K8s authentication initialization fails
func NewImageExistenceCheckerWithK8sAuth(ctx context.Context) *ImageExistenceChecker {
	keychain, err := GetK8sKeychain(ctx)
	if err != nil {
		logging.Logger.Warn("K8s authentication not available, using anonymous access",
			zap.Error(err))
		return NewImageExistenceChecker()
	}

	logging.Logger.Info("Image checker initialized with K8s authentication")

	return &ImageExistenceChecker{
		systemContext: &types.SystemContext{
			DockerInsecureSkipTLSVerify: types.OptionalBoolFalse,
		},
		keychain: keychain,
	}
}

// CheckImageExists verifies if an image exists in the registry
// Uses a more robust approach that handles architecture mismatches gracefully
// Maintains backward compatibility while supporting multi-arch images
// If keychain is available, uses authenticated access via go-containerregistry
func (iec *ImageExistenceChecker) CheckImageExists(imageURL string) (*ImageMetadata, error) {
	ctx := context.Background()

	logging.Logger.Debug("Checking image existence",
		zap.String("image", imageURL),
		zap.String("host_arch", runtime.GOARCH),
		zap.Bool("authenticated", iec.keychain != nil))

	// Try authenticated access first if keychain is available
	if iec.keychain != nil {
		metadata, err := iec.checkImageWithAuth(ctx, imageURL, runtime.GOOS, runtime.GOARCH)
		if err == nil {
			return metadata, nil
		}
		logging.Logger.Debug("Authenticated image check failed, falling back to anonymous",
			zap.String("image", imageURL),
			zap.Error(err))
	}

	// Fallback to existing containers/image implementation
	return iec.checkImageWithContainersImage(ctx, imageURL, runtime.GOOS, runtime.GOARCH)
}

// checkImageWithContainersImage uses the existing containers/image library implementation
func (iec *ImageExistenceChecker) checkImageWithContainersImage(ctx context.Context, imageURL, targetOS, targetArch string) (*ImageMetadata, error) {
	// Parse the image reference
	ref, err := docker.ParseReference("//" + imageURL)
	if err != nil {
		logging.Logger.Error("Failed to parse image reference",
			zap.String("image", imageURL),
			zap.Error(err))
		return &ImageMetadata{Exists: false}, fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Create platform-specific system context
	systemContext := &types.SystemContext{
		DockerInsecureSkipTLSVerify: types.OptionalBoolFalse,
		OSChoice:                    targetOS,
		ArchitectureChoice:          targetArch,
	}

	// Create a source for the image
	source, err := ref.NewImageSource(ctx, systemContext)
	if err != nil {
		logging.Logger.Debug("Image source creation failed (image likely doesn't exist)",
			zap.String("image", imageURL),
			zap.Error(err))
		return &ImageMetadata{Exists: false}, nil
	}
	defer source.Close()

	// Get the image manifest
	manifestBytes, manifestType, err := source.GetManifest(ctx, nil)
	if err != nil {
		logging.Logger.Debug("Failed to get manifest (image likely doesn't exist)",
			zap.String("image", imageURL),
			zap.Error(err))
		return &ImageMetadata{Exists: false}, nil
	}

	// Check if it's a manifest list (multi-arch)
	if manifest.MIMETypeIsMultiImage(manifestType) {
		list, err := manifest.ListFromBlob(manifestBytes, manifestType)
		if err != nil {
			logging.Logger.Debug("Failed to parse manifest list",
				zap.String("image", imageURL),
				zap.Error(err))
			return &ImageMetadata{Exists: false}, nil
		}
		return iec.handleManifestList(ctx, source, list, imageURL, targetOS, targetArch, manifestBytes, manifestType)
	}

	// Single manifest - try to create image
	img, err := image.FromSource(ctx, systemContext, source)
	if err != nil {
		logging.Logger.Debug("Failed to create image from source",
			zap.String("image", imageURL),
			zap.Error(err))

		// Check if this is an architecture mismatch
		if strings.Contains(err.Error(), "no image found in image index for architecture") {
			logging.Logger.Debug("Architecture mismatch detected - image exists but not compatible with host",
				zap.String("image", imageURL),
				zap.String("host_arch", runtime.GOARCH))

			// For architecture mismatches, we'll return that the image exists but without digest
			// This allows the system to proceed while acknowledging the limitation
			return &ImageMetadata{
				Exists:          true,
				Digest:          "", // No digest available due to architecture mismatch
				Manifest:        manifestBytes,
				Config:          nil,
				Architectures:   []string{runtime.GOARCH},
				PlatformDigests: map[string]string{},
				IsMultiArch:     false,
				ManifestType:    manifestType,
			}, nil
		}

		// For other errors, treat as image not found
		return &ImageMetadata{Exists: false}, nil
	}
	defer img.Close()

	// Success! Get the config blob and digest
	configBlob, err := img.ConfigBlob(ctx)
	if err != nil {
		logging.Logger.Debug("Failed to get config blob",
			zap.String("image", imageURL),
			zap.Error(err))
		return &ImageMetadata{Exists: false}, nil
	}

	// Get the digest
	digest := img.ConfigInfo().Digest.String()

	logging.Logger.Debug("Image exists and metadata retrieved",
		zap.String("image", imageURL),
		zap.String("digest", digest),
		zap.String("manifest_type", manifestType))

	return &ImageMetadata{
		Exists:          true,
		Digest:          digest,
		Manifest:        manifestBytes,
		Config:          configBlob,
		Architectures:   []string{runtime.GOARCH},
		PlatformDigests: map[string]string{runtime.GOOS + "/" + runtime.GOARCH: digest},
		IsMultiArch:     false,
		ManifestType:    manifestType,
	}, nil
}

// checkImageWithAuth uses go-containerregistry with k8schain authentication
func (iec *ImageExistenceChecker) checkImageWithAuth(ctx context.Context, imageURL, targetOS, targetArch string) (*ImageMetadata, error) {
	logging.Logger.Debug("Checking image with authentication",
		zap.String("image", imageURL),
		zap.String("platform", targetOS+"/"+targetArch))

	// Parse image reference
	ref, err := name.ParseReference(imageURL)
	if err != nil {
		logging.Logger.Debug("Failed to parse image reference with go-containerregistry",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Set platform options
	platform := v1.Platform{
		OS:           targetOS,
		Architecture: targetArch,
	}

	// Fetch image descriptor with authentication
	desc, err := remote.Get(ref, remote.WithAuthFromKeychain(iec.keychain), remote.WithPlatform(platform))
	if err != nil {
		logging.Logger.Debug("Failed to fetch image descriptor",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, err
	}

	// Get the image
	img, err := desc.Image()
	if err != nil {
		logging.Logger.Debug("Failed to get image from descriptor",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, err
	}

	// Get manifest
	rawManifest, err := img.RawManifest()
	if err != nil {
		logging.Logger.Debug("Failed to get raw manifest",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, err
	}

	// Get config
	configFile, err := img.ConfigFile()
	if err != nil {
		logging.Logger.Debug("Failed to get config file",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, err
	}

	// Get digest
	digest, err := img.Digest()
	if err != nil {
		logging.Logger.Debug("Failed to get image digest",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, err
	}

	// Get manifest type
	mediaType, err := img.MediaType()
	if err != nil {
		logging.Logger.Debug("Failed to get media type",
			zap.String("image", imageURL),
			zap.Error(err))
		return nil, err
	}

	logging.Logger.Debug("Image exists and metadata retrieved with authentication",
		zap.String("image", imageURL),
		zap.String("digest", digest.String()),
		zap.String("media_type", string(mediaType)))

	// Convert config to JSON bytes
	configBytes, err := img.RawConfigFile()
	if err != nil {
		logging.Logger.Debug("Failed to get raw config",
			zap.String("image", imageURL),
			zap.Error(err))
		// Continue without config
		configBytes = []byte{}
	}

	return &ImageMetadata{
		Exists:          true,
		Digest:          digest.String(),
		Manifest:        rawManifest,
		Config:          configBytes,
		Architectures:   []string{configFile.Architecture},
		PlatformDigests: map[string]string{configFile.OS + "/" + configFile.Architecture: digest.String()},
		IsMultiArch:     false,
		ManifestType:    string(mediaType),
	}, nil
}

// CheckImageExistsForPlatform checks if an image exists for a specific platform
func (iec *ImageExistenceChecker) CheckImageExistsForPlatform(imageURL, os, arch string) (*ImageMetadata, error) {
	ctx := context.Background()

	logging.Logger.Debug("Checking image existence for platform",
		zap.String("image", imageURL),
		zap.String("os", os),
		zap.String("arch", arch),
		zap.Bool("authenticated", iec.keychain != nil))

	// Try authenticated access first if keychain is available
	if iec.keychain != nil {
		metadata, err := iec.checkImageWithAuth(ctx, imageURL, os, arch)
		if err == nil {
			return metadata, nil
		}
		logging.Logger.Debug("Authenticated image check failed, falling back to anonymous",
			zap.String("image", imageURL),
			zap.String("platform", os+"/"+arch),
			zap.Error(err))
	}

	// Fallback to existing containers/image implementation
	return iec.checkImageWithContainersImage(ctx, imageURL, os, arch)
}

// handleManifestList processes a manifest list and extracts platform-specific information
func (iec *ImageExistenceChecker) handleManifestList(ctx context.Context, source types.ImageSource, list manifest.List, imageURL, targetOS, targetArch string, manifestBytes []byte, manifestType string) (*ImageMetadata, error) {
	// For manifest lists, we'll use the containers/image library's built-in platform selection
	// Create a platform-specific system context
	systemContext := &types.SystemContext{
		DockerInsecureSkipTLSVerify: types.OptionalBoolFalse,
		OSChoice:                    targetOS,
		ArchitectureChoice:          targetArch,
	}

	// Try to create an image from the source with the target platform
	img, err := image.FromSource(ctx, systemContext, source)
	if err != nil {
		logging.Logger.Debug("Failed to create image from manifest list for target platform",
			zap.String("image", imageURL),
			zap.String("platform", targetOS+"/"+targetArch),
			zap.Error(err))
		return &ImageMetadata{Exists: false}, nil
	}
	defer img.Close()

	// Get config blob and digest
	configBlob, err := img.ConfigBlob(ctx)
	if err != nil {
		logging.Logger.Debug("Failed to get config blob from manifest list",
			zap.String("image", imageURL),
			zap.String("platform", targetOS+"/"+targetArch),
			zap.Error(err))
		return &ImageMetadata{Exists: false}, nil
	}

	digest := img.ConfigInfo().Digest.String()
	platform := targetOS + "/" + targetArch

	logging.Logger.Debug("Manifest list processed successfully",
		zap.String("image", imageURL),
		zap.String("platform", platform),
		zap.String("digest", digest))

	return &ImageMetadata{
		Exists:          true,
		Digest:          digest,
		Manifest:        manifestBytes,
		Config:          configBlob,
		Architectures:   []string{targetArch},
		PlatformDigests: map[string]string{platform: digest},
		IsMultiArch:     true,
		ManifestType:    manifestType,
	}, nil
}

// GetAvailablePlatforms returns all available platforms for an image
// Note: This is a simplified implementation that returns common platforms for multi-arch images
func (iec *ImageExistenceChecker) GetAvailablePlatforms(imageURL string) ([]string, error) {
	ctx := context.Background()

	// Parse the image reference
	ref, err := docker.ParseReference("//" + imageURL)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference: %w", err)
	}

	// Create a source for the image
	source, err := ref.NewImageSource(ctx, iec.systemContext)
	if err != nil {
		return nil, fmt.Errorf("failed to create image source: %w", err)
	}
	defer source.Close()

	// Get the image manifest
	_, manifestType, err := source.GetManifest(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to get manifest: %w", err)
	}

	// Check if it's a manifest list
	if manifest.MIMETypeIsMultiImage(manifestType) {
		// For now, return common platforms that are typically available
		// In a more sophisticated implementation, you would parse the manifest list
		return []string{"linux/amd64", "linux/arm64", "linux/arm/v7"}, nil
	}

	// Single manifest - return host platform
	return []string{runtime.GOOS + "/" + runtime.GOARCH}, nil
}

// GetDigestForPlatform returns the digest for a specific platform
func (iec *ImageExistenceChecker) GetDigestForPlatform(imageURL, os, arch string) (string, error) {
	metadata, err := iec.CheckImageExistsForPlatform(imageURL, os, arch)
	if err != nil {
		return "", err
	}
	if !metadata.Exists {
		return "", fmt.Errorf("image not found for platform %s/%s", os, arch)
	}
	return metadata.Digest, nil
}

// Helper function to get map keys
func getKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
