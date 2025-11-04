package image

import (
	"testing"

	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

func TestMain(m *testing.M) {
	// Initialize logger for tests
	logger, _ := zap.NewDevelopment()
	logging.Logger = logger
	defer logger.Sync()

	m.Run()
}

func TestImageExistenceChecker_CheckImageExists(t *testing.T) {
	checker := NewImageExistenceChecker()

	tests := []struct {
		name     string
		imageURL string
		wantErr  bool
	}{
		{
			name:     "alpine latest - should exist",
			imageURL: "alpine:latest",
			wantErr:  false,
		},
		{
			name:     "nonexistent image - should not exist",
			imageURL: "nonexistent-registry/nonexistent-image:nonexistent-tag",
			wantErr:  false, // Changed to false since the method returns metadata with Exists=false instead of error
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := checker.CheckImageExists(tt.imageURL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CheckImageExists() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("CheckImageExists() error = %v", err)
				return
			}

			// For nonexistent images, we expect Exists=false
			if tt.name == "nonexistent image - should not exist" {
				if metadata.Exists {
					t.Errorf("CheckImageExists() image should not exist but does")
				}
				return
			}

			if !metadata.Exists {
				t.Errorf("CheckImageExists() image should exist but doesn't")
				return
			}

			// Verify metadata fields are populated
			if metadata.Digest == "" {
				t.Errorf("CheckImageExists() digest should not be empty")
			}

			if len(metadata.Manifest) == 0 {
				t.Errorf("CheckImageExists() manifest should not be empty")
			}

			if metadata.ManifestType == "" {
				t.Errorf("CheckImageExists() manifest type should not be empty")
			}

			t.Logf("Image: %s", tt.imageURL)
			t.Logf("Digest: %s", metadata.Digest)
			t.Logf("Manifest Type: %s", metadata.ManifestType)
			t.Logf("Is Multi-Arch: %v", metadata.IsMultiArch)
			t.Logf("Architectures: %v", metadata.Architectures)
			t.Logf("Platform Digests: %v", metadata.PlatformDigests)
		})
	}
}

func TestImageExistenceChecker_CheckImageExistsForPlatform(t *testing.T) {
	checker := NewImageExistenceChecker()

	tests := []struct {
		name     string
		imageURL string
		os       string
		arch     string
		wantErr  bool
	}{
		{
			name:     "alpine latest for linux/amd64",
			imageURL: "alpine:latest",
			os:       "linux",
			arch:     "amd64",
			wantErr:  false,
		},
		{
			name:     "alpine latest for linux/arm64",
			imageURL: "alpine:latest",
			os:       "linux",
			arch:     "arm64",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			metadata, err := checker.CheckImageExistsForPlatform(tt.imageURL, tt.os, tt.arch)

			if tt.wantErr {
				if err == nil {
					t.Errorf("CheckImageExistsForPlatform() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("CheckImageExistsForPlatform() error = %v", err)
				return
			}

			if !metadata.Exists {
				t.Errorf("CheckImageExistsForPlatform() image should exist but doesn't")
				return
			}

			// Verify metadata fields are populated
			if metadata.Digest == "" {
				t.Errorf("CheckImageExistsForPlatform() digest should not be empty")
			}

			if len(metadata.Manifest) == 0 {
				t.Errorf("CheckImageExistsForPlatform() manifest should not be empty")
			}

			if metadata.ManifestType == "" {
				t.Errorf("CheckImageExistsForPlatform() manifest type should not be empty")
			}

			// Verify platform-specific fields
			expectedPlatform := tt.os + "/" + tt.arch
			if platformDigest, exists := metadata.PlatformDigests[expectedPlatform]; !exists {
				t.Errorf("CheckImageExistsForPlatform() platform digest for %s should exist", expectedPlatform)
			} else if platformDigest == "" {
				t.Errorf("CheckImageExistsForPlatform() platform digest for %s should not be empty", expectedPlatform)
			}

			t.Logf("Image: %s", tt.imageURL)
			t.Logf("Platform: %s/%s", tt.os, tt.arch)
			t.Logf("Digest: %s", metadata.Digest)
			t.Logf("Manifest Type: %s", metadata.ManifestType)
			t.Logf("Is Multi-Arch: %v", metadata.IsMultiArch)
			t.Logf("Architectures: %v", metadata.Architectures)
			t.Logf("Platform Digests: %v", metadata.PlatformDigests)
		})
	}
}

func TestImageExistenceChecker_GetDigestForPlatform(t *testing.T) {
	checker := NewImageExistenceChecker()

	tests := []struct {
		name     string
		imageURL string
		os       string
		arch     string
		wantErr  bool
	}{
		{
			name:     "nginx latest digest for linux/amd64",
			imageURL: "nginx:latest",
			os:       "linux",
			arch:     "amd64",
			wantErr:  false,
		},
		{
			name:     "nginx latest digest for linux/arm64",
			imageURL: "nginx:latest",
			os:       "linux",
			arch:     "arm64",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			digest, err := checker.GetDigestForPlatform(tt.imageURL, tt.os, tt.arch)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetDigestForPlatform() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetDigestForPlatform() error = %v", err)
				return
			}

			if digest == "" {
				t.Errorf("GetDigestForPlatform() digest should not be empty")
			}

			// Verify digest format (should start with sha256:)
			if len(digest) < 7 || digest[:7] != "sha256:" {
				t.Errorf("GetDigestForPlatform() digest should start with 'sha256:', got: %s", digest)
			}

			t.Logf("Image: %s", tt.imageURL)
			t.Logf("Platform: %s/%s", tt.os, tt.arch)
			t.Logf("Digest: %s", digest)
		})
	}
}

func TestImageExistenceChecker_GetAvailablePlatforms(t *testing.T) {
	checker := NewImageExistenceChecker()

	tests := []struct {
		name     string
		imageURL string
		wantErr  bool
	}{
		{
			name:     "nginx latest platforms",
			imageURL: "nginx:latest",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			platforms, err := checker.GetAvailablePlatforms(tt.imageURL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetAvailablePlatforms() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetAvailablePlatforms() error = %v", err)
				return
			}

			if len(platforms) == 0 {
				t.Errorf("GetAvailablePlatforms() should return at least one platform")
			}

			// Verify platform format (should be os/arch or os/arch/variant)
			for _, platform := range platforms {
				if platform == "" {
					t.Errorf("GetAvailablePlatforms() platform should not be empty")
				}
				// Platform should contain at least one slash
				if len(platform) < 3 || platform[0] == '/' || platform[len(platform)-1] == '/' {
					t.Errorf("GetAvailablePlatforms() platform format should be 'os/arch' or 'os/arch/variant', got: %s", platform)
				}
			}

			t.Logf("Image: %s", tt.imageURL)
			t.Logf("Available Platforms: %v", platforms)
		})
	}
}

func TestImageResolver_GetImageDigest(t *testing.T) {
	checker := NewImageExistenceChecker()
	resolver := NewImageResolver("", "", checker)

	tests := []struct {
		name     string
		imageURL string
		wantErr  bool
	}{
		{
			name:     "nginx latest digest",
			imageURL: "nginx:latest",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageWithDigest, err := resolver.GetImageDigest(tt.imageURL)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetImageDigest() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetImageDigest() error = %v", err)
				return
			}

			if imageWithDigest == "" {
				t.Errorf("GetImageDigest() result should not be empty")
			}

			// Verify format: should be image@sha256:digest
			if len(imageWithDigest) < len(tt.imageURL)+8 {
				t.Errorf("GetImageDigest() result should contain digest, got: %s", imageWithDigest)
			}

			t.Logf("Original Image: %s", tt.imageURL)
			t.Logf("Image with Digest: %s", imageWithDigest)
		})
	}
}

func TestImageResolver_GetImageDigestForPlatform(t *testing.T) {
	checker := NewImageExistenceChecker()
	resolver := NewImageResolver("", "", checker)

	tests := []struct {
		name     string
		imageURL string
		os       string
		arch     string
		wantErr  bool
	}{
		{
			name:     "nginx latest digest for linux/amd64",
			imageURL: "nginx:latest",
			os:       "linux",
			arch:     "amd64",
			wantErr:  false,
		},
		{
			name:     "nginx latest digest for linux/arm64",
			imageURL: "nginx:latest",
			os:       "linux",
			arch:     "arm64",
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			imageWithDigest, err := resolver.GetImageDigestForPlatform(tt.imageURL, tt.os, tt.arch)

			if tt.wantErr {
				if err == nil {
					t.Errorf("GetImageDigestForPlatform() expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("GetImageDigestForPlatform() error = %v", err)
				return
			}

			if imageWithDigest == "" {
				t.Errorf("GetImageDigestForPlatform() result should not be empty")
			}

			// Verify format: should be image@sha256:digest
			if len(imageWithDigest) < len(tt.imageURL)+8 {
				t.Errorf("GetImageDigestForPlatform() result should contain digest, got: %s", imageWithDigest)
			}

			t.Logf("Original Image: %s", tt.imageURL)
			t.Logf("Platform: %s/%s", tt.os, tt.arch)
			t.Logf("Image with Digest: %s", imageWithDigest)
		})
	}
}

// Benchmark tests to measure performance
func BenchmarkImageExistenceChecker_CheckImageExists(b *testing.B) {
	checker := NewImageExistenceChecker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = checker.CheckImageExists("nginx:latest")
	}
}

func BenchmarkImageExistenceChecker_CheckImageExistsForPlatform(b *testing.B) {
	checker := NewImageExistenceChecker()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = checker.CheckImageExistsForPlatform("nginx:latest", "linux", "amd64")
	}
}
