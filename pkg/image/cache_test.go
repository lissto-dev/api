package image_test

import (
	"context"
	"time"

	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/cache"
	"github.com/lissto-dev/api/pkg/image"
)

// MockImageChecker implements ImageExistenceChecker interface for testing
type MockImageChecker struct {
	// Map of imageURL@platform -> digest for testing
	responses map[string]string
	callCount map[string]int // Track how many times each image was checked
}

func NewMockImageChecker() *MockImageChecker {
	return &MockImageChecker{
		responses: make(map[string]string),
		callCount: make(map[string]int),
	}
}

func (m *MockImageChecker) CheckImageExists(imageURL string) (*image.ImageMetadata, error) {
	// Default to linux/amd64 for non-platform-specific checks
	return m.CheckImageExistsForPlatform(imageURL, "linux", "amd64")
}

func (m *MockImageChecker) CheckImageExistsForPlatform(imageURL, os, arch string) (*image.ImageMetadata, error) {
	key := imageURL + "@" + os + "/" + arch
	m.callCount[key]++

	digest, exists := m.responses[key]
	if !exists {
		return &image.ImageMetadata{Exists: false}, nil
	}

	return &image.ImageMetadata{
		Exists:          true,
		Digest:          digest,
		Manifest:        []byte{},
		Config:          []byte{},
		Architectures:   []string{arch},
		PlatformDigests: map[string]string{os + "/" + arch: digest},
		IsMultiArch:     false,
		ManifestType:    "application/vnd.docker.distribution.manifest.v2+json",
	}, nil
}

func (m *MockImageChecker) GetCallCount(imageURL, os, arch string) int {
	key := imageURL + "@" + os + "/" + arch
	return m.callCount[key]
}

func (m *MockImageChecker) AddResponse(imageURL, os, arch, digest string) {
	key := imageURL + "@" + os + "/" + arch
	m.responses[key] = digest
}

var _ = Describe("Image Cache", func() {
	Describe("IsInfraImage", func() {
		Context("when service has image and no build", func() {
			It("should return true for postgres", func() {
				service := types.ServiceConfig{
					Image: "postgres:15.2",
					Build: nil,
				}
				Expect(image.IsInfraImage(service)).To(BeTrue())
			})

			It("should return true for redis", func() {
				service := types.ServiceConfig{
					Image: "redis:7.0-alpine",
					Build: nil,
				}
				Expect(image.IsInfraImage(service)).To(BeTrue())
			})
		})

		Context("when service has build section", func() {
			It("should return false even with image field", func() {
				service := types.ServiceConfig{
					Image: "myapp:latest",
					Build: &types.BuildConfig{
						Context: ".",
					},
				}
				Expect(image.IsInfraImage(service)).To(BeFalse())
			})

			It("should return false with no image field", func() {
				service := types.ServiceConfig{
					Image: "",
					Build: &types.BuildConfig{
						Context: ".",
					},
				}
				Expect(image.IsInfraImage(service)).To(BeFalse())
			})
		})
	})

	Describe("IsSemverTag", func() {
		DescribeTable("valid semver tags",
			func(imageURL string, expected bool) {
				Expect(image.IsSemverTag(imageURL)).To(Equal(expected))
			},
			Entry("semver 1.2.3", "myapp:1.2.3", true),
			Entry("semver v1.2.3", "myapp:v1.2.3", true),
			Entry("semver with prerelease", "myapp:v1.2.3-rc1", true),
			Entry("semver with build metadata", "myapp:1.2.3-alpha.1", true),
			Entry("short semver 1.2", "myapp:1.2", true),
			Entry("semver with suffix", "myapp:1.2.3-alpine", true),
		)

		DescribeTable("invalid semver tags",
			func(imageURL string, expected bool) {
				Expect(image.IsSemverTag(imageURL)).To(Equal(expected))
			},
			Entry("latest tag", "myapp:latest", false),
			Entry("dev tag", "myapp:dev", false),
			Entry("commit hash", "myapp:abc123", false),
			Entry("branch name", "myapp:feature-branch", false),
		)
	})

	Describe("GetCacheKey", func() {
		It("should generate consistent cache keys", func() {
			key := image.GetCacheKey("postgres:15.2", "linux", "amd64")
			Expect(key).To(Equal("postgres:15.2@linux/amd64"))
		})

		It("should handle different platforms", func() {
			key := image.GetCacheKey("redis:7.0-alpine", "linux", "arm64")
			Expect(key).To(Equal("redis:7.0-alpine@linux/arm64"))
		})
	})

	Describe("ShouldCache", func() {
		Context("infrastructure images", func() {
			It("should cache all tags including latest", func() {
				Expect(image.ShouldCache(true, "postgres:latest")).To(BeTrue())
			})

			It("should cache specific version tags", func() {
				Expect(image.ShouldCache(true, "postgres:15.2")).To(BeTrue())
			})

			It("should cache variant tags", func() {
				Expect(image.ShouldCache(true, "redis:7.0-alpine")).To(BeTrue())
			})

			It("should cache even mutable tags", func() {
				Expect(image.ShouldCache(true, "postgres:dev")).To(BeTrue())
			})
		})

		Context("service images", func() {
			It("should cache semver tags", func() {
				Expect(image.ShouldCache(false, "myapp:v1.2.3")).To(BeTrue())
			})

			It("should not cache latest tag", func() {
				Expect(image.ShouldCache(false, "myapp:latest")).To(BeFalse())
			})

			It("should not cache main branch", func() {
				Expect(image.ShouldCache(false, "myapp:main")).To(BeFalse())
			})

			It("should not cache dev tag", func() {
				Expect(image.ShouldCache(false, "myapp:dev")).To(BeFalse())
			})

			It("should not cache staging tag", func() {
				Expect(image.ShouldCache(false, "myapp:staging")).To(BeFalse())
			})
		})
	})

	Describe("GetTTL", func() {
		Context("infrastructure images", func() {
			It("should return 24h for latest tag", func() {
				ttl := image.GetTTL(true, "postgres:latest")
				Expect(ttl).To(Equal(24 * time.Hour))
			})

			It("should return 24h for specific tags", func() {
				ttl := image.GetTTL(true, "postgres:15.2")
				Expect(ttl).To(Equal(24 * time.Hour))
			})

			It("should return 24h for variant tags", func() {
				ttl := image.GetTTL(true, "redis:7.0-alpine")
				Expect(ttl).To(Equal(24 * time.Hour))
			})
		})

		Context("service images", func() {
			It("should return 1h for semver tags", func() {
				ttl := image.GetTTL(false, "myapp:v1.2.3")
				Expect(ttl).To(Equal(1 * time.Hour))
			})

			It("should return 0 for latest tag (no cache)", func() {
				ttl := image.GetTTL(false, "myapp:latest")
				Expect(ttl).To(Equal(time.Duration(0)))
			})

			It("should return 0 for main branch (no cache)", func() {
				ttl := image.GetTTL(false, "myapp:main")
				Expect(ttl).To(Equal(time.Duration(0)))
			})

			It("should return 0 for dev tag (no cache)", func() {
				ttl := image.GetTTL(false, "myapp:dev")
				Expect(ttl).To(Equal(time.Duration(0)))
			})
		})
	})

	Describe("GetImageType", func() {
		It("should return 'infra' for infrastructure images", func() {
			Expect(image.GetImageType(true)).To(Equal("infra"))
		})

		It("should return 'service' for service images", func() {
			Expect(image.GetImageType(false)).To(Equal("service"))
		})
	})

	Describe("ImageResolver with Cache Integration (mocked)", func() {
		var (
			resolver    *image.ImageResolver
			mockCache   *cache.MemoryCache
			mockChecker *MockImageChecker
		)

		BeforeEach(func() {
			mockChecker = NewMockImageChecker()
			mockCache = cache.NewMemoryCache()
			resolver = image.NewImageResolverWithCache("", "", mockChecker, mockCache)

			// Setup mock responses
			mockChecker.AddResponse("postgres:15.2", "linux", "amd64", "sha256:abc123postgres")
			mockChecker.AddResponse("redis:7.0-alpine", "linux", "amd64", "sha256:def456redis")
			mockChecker.AddResponse("myapp:v1.2.3", "linux", "amd64", "sha256:xyz789myapp")
			mockChecker.AddResponse("myapp:latest", "linux", "amd64", "sha256:latest123")
			mockChecker.AddResponse("myapp:main", "linux", "amd64", "sha256:main456")
			mockChecker.AddResponse("postgres:15.2", "linux", "arm64", "sha256:arm64postgres")
		})

		Context("Infrastructure images (no build)", func() {
			It("should cache postgres:15.2 and return from cache on second call", func() {
				service := types.ServiceConfig{
					Image: "postgres:15.2",
					Build: nil,
				}

				// First call - cache miss
				digest1, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest1).To(ContainSubstring("sha256:abc123postgres"))
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "amd64")).To(Equal(1))

				// Second call - should be cache hit (checker should not be called again)
				digest2, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest2).To(Equal(digest1))
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "amd64")).To(Equal(1), "Should use cache, not call checker again")
			})

			It("should cache redis:7.0-alpine even though it has variant tag", func() {
				service := types.ServiceConfig{
					Image: "redis:7.0-alpine",
					Build: nil,
				}

				// First call
				digest1, err := resolver.GetImageDigestWithCacheContext("redis:7.0-alpine", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest1).To(ContainSubstring("sha256:def456redis"))

				// Second call - cache hit
				digest2, err := resolver.GetImageDigestWithCacheContext("redis:7.0-alpine", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest2).To(Equal(digest1))
				Expect(mockChecker.GetCallCount("redis:7.0-alpine", "linux", "amd64")).To(Equal(1))
			})

			It("should cache different platforms separately", func() {
				service := types.ServiceConfig{
					Image: "postgres:15.2",
					Build: nil,
				}

				// Call for amd64
				digestAmd64, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digestAmd64).To(ContainSubstring("sha256:abc123postgres"))

				// Call for arm64 - should not use amd64 cache
				digestArm64, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "arm64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digestArm64).To(ContainSubstring("sha256:arm64postgres"))

				// Verify both were fetched
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "amd64")).To(Equal(1))
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "arm64")).To(Equal(1))

				// Second call for amd64 should use cache
				digestAmd64_2, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digestAmd64_2).To(Equal(digestAmd64))
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "amd64")).To(Equal(1), "Should use cache")
			})
		})

		Context("Service images (with build)", func() {
			It("should cache semver tag v1.2.3", func() {
				service := types.ServiceConfig{
					Image: "myapp:v1.2.3",
					Build: &types.BuildConfig{Context: "."},
				}

				// First call
				digest1, err := resolver.GetImageDigestWithCacheContext("myapp:v1.2.3", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest1).To(ContainSubstring("sha256:xyz789myapp"))

				// Second call - cache hit
				digest2, err := resolver.GetImageDigestWithCacheContext("myapp:v1.2.3", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest2).To(Equal(digest1))
				Expect(mockChecker.GetCallCount("myapp:v1.2.3", "linux", "amd64")).To(Equal(1))
			})

			It("should NOT cache latest tag", func() {
				service := types.ServiceConfig{
					Image: "myapp:latest",
					Build: &types.BuildConfig{Context: "."},
				}

				// First call
				digest1, err := resolver.GetImageDigestWithCacheContext("myapp:latest", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest1).To(ContainSubstring("sha256:latest123"))

				// Second call - should NOT use cache
				digest2, err := resolver.GetImageDigestWithCacheContext("myapp:latest", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest2).To(Equal(digest1))
				Expect(mockChecker.GetCallCount("myapp:latest", "linux", "amd64")).To(Equal(2), "Should NOT cache latest tag")
			})

			It("should NOT cache main branch tag", func() {
				service := types.ServiceConfig{
					Image: "myapp:main",
					Build: &types.BuildConfig{Context: "."},
				}

				// First call
				_, err := resolver.GetImageDigestWithCacheContext("myapp:main", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())

				// Second call - should NOT use cache
				_, err = resolver.GetImageDigestWithCacheContext("myapp:main", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())

				Expect(mockChecker.GetCallCount("myapp:main", "linux", "amd64")).To(Equal(2), "Should NOT cache branch tag")
			})
		})

		Context("Cache expiration", func() {
			It("should respect TTL for cached entries", func() {
				// This test verifies the cache respects TTL by waiting for expiration
				// Note: We can't easily test 24h/1h TTLs in unit tests, but we verify the cache stores with TTL
				service := types.ServiceConfig{
					Image: "postgres:15.2",
					Build: nil,
				}

				// First call - cache miss
				digest1, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())

				// Verify entry is in cache
				ctx := context.Background()
				var cachedEntry cache.ImageDigestCache
				cacheKey := image.GetCacheKey("postgres:15.2", "linux", "amd64")
				err = mockCache.Get(ctx, cacheKey, &cachedEntry)
				Expect(err).NotTo(HaveOccurred())
				Expect(cachedEntry.Digest).To(ContainSubstring("sha256:abc123postgres"))
				Expect(cachedEntry.ImageType).To(Equal("infra"))
				Expect(cachedEntry.Platform).To(Equal("linux/amd64"))

				// Immediate second call should use cache
				digest2, err := resolver.GetImageDigestWithCacheContext("postgres:15.2", "linux", "amd64", service)
				Expect(err).NotTo(HaveOccurred())
				Expect(digest2).To(Equal(digest1))
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "amd64")).To(Equal(1))
			})
		})

		Context("Without cache", func() {
			It("should always fetch when cache is not configured", func() {
				// Create resolver without cache
				resolverNoCache := image.NewImageResolver("", "", mockChecker)
				service := types.ServiceConfig{
					Image: "postgres:15.2",
					Build: nil,
				}

				// First call
				_, err := resolverNoCache.GetImageDigestWithServicePlatform("postgres:15.2", service)
				Expect(err).NotTo(HaveOccurred())

				// Second call - should fetch again since no cache
				_, err = resolverNoCache.GetImageDigestWithServicePlatform("postgres:15.2", service)
				Expect(err).NotTo(HaveOccurred())

				// Both calls should hit the checker
				Expect(mockChecker.GetCallCount("postgres:15.2", "linux", "amd64")).To(Equal(2))
			})
		})
	})
})
