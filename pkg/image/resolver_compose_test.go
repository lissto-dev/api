package image_test

import (
	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/image"
)

var _ = Describe("ImageResolver - Registry Priority", func() {
	When("resolving registry with compose configuration", func() {
		It("should prioritize service label over compose and global", func() {
			resolver := image.NewImageResolver("global-registry.io", "", nil)
			service := types.ServiceConfig{
				Name: "test-service",
				Labels: map[string]string{
					"lissto.dev/registry": "service-registry.io",
				},
			}

			result := resolver.ResolveRegistryWithCompose(service, "compose-registry.io")

			Expect(result).To(Equal("service-registry.io"), "Service label should be highest priority")
		})

		It("should use compose registry when no service label", func() {
			resolver := image.NewImageResolver("global-registry.io", "", nil)
			service := types.ServiceConfig{
				Name:   "test-service",
				Labels: map[string]string{},
			}

			result := resolver.ResolveRegistryWithCompose(service, "compose-registry.io")

			Expect(result).To(Equal("compose-registry.io"), "Compose registry should be second priority")
		})

		It("should use global registry when no compose or service label", func() {
			resolver := image.NewImageResolver("global-registry.io", "", nil)
			service := types.ServiceConfig{
				Name:   "test-service",
				Labels: map[string]string{},
			}

			result := resolver.ResolveRegistryWithCompose(service, "")

			Expect(result).To(Equal("global-registry.io"), "Global registry should be used as fallback")
		})

		It("should return empty when no registry specified anywhere", func() {
			resolver := image.NewImageResolver("", "", nil)
			service := types.ServiceConfig{
				Name:   "test-service",
				Labels: map[string]string{},
			}

			result := resolver.ResolveRegistryWithCompose(service, "")

			Expect(result).To(BeEmpty(), "Should return empty when no registry is configured")
		})
	})
})

var _ = Describe("ImageResolver - Repository Priority", func() {
	When("resolving repository with compose configuration", func() {
		It("should prioritize service label repository", func() {
			resolver := image.NewImageResolver("", "global-prefix/", nil)
			service := types.ServiceConfig{
				Name: "web",
				Labels: map[string]string{
					"lissto.dev/repository": "custom/service-repo",
				},
			}

			result := resolver.ResolveImageNameWithCompose(service, "monorepo-image", "compose-prefix/")

			Expect(result).To(Equal("custom/service-repo"), "Service label repository should be highest priority")
		})

		It("should use compose repository for monorepo when no service label", func() {
			resolver := image.NewImageResolver("", "global-prefix/", nil)
			service := types.ServiceConfig{
				Name:   "web",
				Labels: map[string]string{},
			}

			result := resolver.ResolveImageNameWithCompose(service, "monorepo-image", "compose-prefix/")

			Expect(result).To(Equal("monorepo-image"), "Compose repository should be second priority (single image for all services)")
		})

		It("should use compose prefix when no repository or service label", func() {
			resolver := image.NewImageResolver("", "global-prefix/", nil)
			service := types.ServiceConfig{
				Name:   "web",
				Labels: map[string]string{},
			}

			result := resolver.ResolveImageNameWithCompose(service, "", "compose-prefix/")

			Expect(result).To(Equal("compose-prefix/web"), "Compose prefix should be third priority")
		})

		It("should use global prefix when no compose or service label", func() {
			resolver := image.NewImageResolver("", "global-prefix/", nil)
			service := types.ServiceConfig{
				Name:   "web",
				Labels: map[string]string{},
			}

			result := resolver.ResolveImageNameWithCompose(service, "", "")

			Expect(result).To(Equal("global-prefix/web"), "Global prefix should be fourth priority")
		})

		It("should use service name only when no prefix specified", func() {
			resolver := image.NewImageResolver("", "", nil)
			service := types.ServiceConfig{
				Name:   "web",
				Labels: map[string]string{},
			}

			result := resolver.ResolveImageNameWithCompose(service, "", "")

			Expect(result).To(Equal("web"), "Should use service name only when no prefix is configured")
		})
	})
})

var _ = Describe("ImageResolver - Integration", func() {
	var resolver *image.ImageResolver

	BeforeEach(func() {
		resolver = image.NewImageResolver("global.registry.io", "global-prefix/", nil)
	})

	Context("with compose prefix configuration", func() {
		It("should use compose registry and prefix", func() {
			service := types.ServiceConfig{
				Name:   "api",
				Labels: map[string]string{},
			}

			registry := resolver.ResolveRegistryWithCompose(service, "compose.registry.io")
			imageName := resolver.ResolveImageNameWithCompose(service, "", "compose-prefix/")

			Expect(registry).To(Equal("compose.registry.io"))
			Expect(imageName).To(Equal("compose-prefix/api"))
		})
	})

	Context("with compose repository (monorepo pattern)", func() {
		It("should use single image for all services", func() {
			service := types.ServiceConfig{
				Name:   "api",
				Labels: map[string]string{},
			}

			registry := resolver.ResolveRegistryWithCompose(service, "compose.registry.io")
			imageName := resolver.ResolveImageNameWithCompose(service, "my-monorepo-image", "compose-prefix/")

			Expect(registry).To(Equal("compose.registry.io"))
			Expect(imageName).To(Equal("my-monorepo-image"), "Should use repository over prefix")
		})
	})

	Context("with service label overrides", func() {
		It("should prioritize service labels over all other configs", func() {
			service := types.ServiceConfig{
				Name: "api",
				Labels: map[string]string{
					"lissto.dev/registry":   "service.registry.io",
					"lissto.dev/repository": "custom/api-service",
				},
			}

			registry := resolver.ResolveRegistryWithCompose(service, "compose.registry.io")
			imageName := resolver.ResolveImageNameWithCompose(service, "monorepo", "compose-prefix/")

			Expect(registry).To(Equal("service.registry.io"))
			Expect(imageName).To(Equal("custom/api-service"))
		})
	})

	Context("with no compose configuration", func() {
		It("should fall back to global configuration", func() {
			service := types.ServiceConfig{
				Name:   "api",
				Labels: map[string]string{},
			}

			registry := resolver.ResolveRegistryWithCompose(service, "")
			imageName := resolver.ResolveImageNameWithCompose(service, "", "")

			Expect(registry).To(Equal("global.registry.io"))
			Expect(imageName).To(Equal("global-prefix/api"))
		})
	})
})

var _ = Describe("ImageResolver - Image Override Label", func() {
	var (
		mockChecker *mockImageChecker
		resolver    *image.ImageResolver
	)

	BeforeEach(func() {
		mockChecker = &mockImageChecker{
			existingImages: make(map[string]bool),
		}
		resolver = image.NewImageResolver("global.registry.io", "global-prefix/", mockChecker)
	})

	Context("with lissto.dev/image label", func() {
		It("should override all other image resolution", func() {
			overrideImage := "123456789012.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/nginx:latest"
			mockChecker.existingImages[overrideImage] = true

			service := types.ServiceConfig{
				Name: "web",
				Labels: map[string]string{
					"lissto.dev/image":      overrideImage,
					"lissto.dev/registry":   "should-be-ignored.io",
					"lissto.dev/repository": "should-be-ignored",
					"lissto.dev/tag":        "should-be-ignored",
				},
			}

			config := image.ResolutionConfig{
				ComposeRegistry:   "compose.registry.io",
				ComposeRepository: "monorepo",
				ComposePrefix:     "compose-prefix/",
			}

			result, err := resolver.ResolveImage(service, config)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(overrideImage))
		})

		It("should work with ResolveImageWithCandidates", func() {
			overrideImage := "ghcr.io/lissto-dev/controller:v1.2.3"
			mockChecker.existingImages[overrideImage] = true

			service := types.ServiceConfig{
				Name: "controller",
				Labels: map[string]string{
					"lissto.dev/image": overrideImage,
				},
			}

			config := image.ResolutionConfig{
				Commit: "abc123",
				Branch: "main",
			}

			result, err := resolver.ResolveImageWithCandidates(service, config)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).ToNot(BeNil())
			// ResolveImageWithCandidates returns image with digest
			Expect(result.FinalImage).To(Equal("ghcr.io/lissto-dev/controller@sha256:mockdigest"))
			Expect(result.Method).To(Equal("override"))
			Expect(result.Selected).To(Equal(overrideImage))
		})

		It("should return error when override image doesn't exist", func() {
			overrideImage := "nonexistent.registry.io/image:tag"
			// Don't add to existingImages, so it doesn't exist

			service := types.ServiceConfig{
				Name: "web",
				Labels: map[string]string{
					"lissto.dev/image": overrideImage,
				},
			}

			config := image.ResolutionConfig{}

			result, err := resolver.ResolveImage(service, config)

			Expect(err).To(HaveOccurred())
			Expect(result).To(BeEmpty())
			Expect(err.Error()).To(ContainSubstring("image override"))
			Expect(err.Error()).To(ContainSubstring(overrideImage))
		})

		It("should prioritize override over all configs", func() {
			overrideImage := "override.io/custom/image:specific"
			mockChecker.existingImages[overrideImage] = true

			// Also add a competing image that would be found via normal resolution
			normalImage := "global.registry.io/global-prefix/web:latest"
			mockChecker.existingImages[normalImage] = true

			service := types.ServiceConfig{
				Name: "web",
				Labels: map[string]string{
					"lissto.dev/image": overrideImage,
				},
			}

			config := image.ResolutionConfig{}

			result, err := resolver.ResolveImage(service, config)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(overrideImage), "Should use override, not normal resolution")
		})

		It("should handle ECR pull-through cache pattern", func() {
			// Typical ECR pull-through cache URL for Docker Hub
			ecrPullThrough := "123456789012.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/postgres:15-alpine"
			mockChecker.existingImages[ecrPullThrough] = true

			service := types.ServiceConfig{
				Name:  "postgres",
				Image: "postgres:15-alpine",
				Labels: map[string]string{
					"lissto.dev/image": ecrPullThrough,
				},
			}

			config := image.ResolutionConfig{}

			result, err := resolver.ResolveImage(service, config)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(ecrPullThrough))
		})
	})

	Context("without lissto.dev/image label", func() {
		It("should use normal resolution path", func() {
			normalImage := "global.registry.io/global-prefix/web:latest"
			mockChecker.existingImages[normalImage] = true

			service := types.ServiceConfig{
				Name:   "web",
				Labels: map[string]string{
					// No override label
				},
			}

			config := image.ResolutionConfig{}

			result, err := resolver.ResolveImage(service, config)

			Expect(err).ToNot(HaveOccurred())
			Expect(result).To(Equal(normalImage))
		})
	})
})

// mockImageChecker is a test double for image existence checking
type mockImageChecker struct {
	existingImages map[string]bool
}

func (m *mockImageChecker) CheckImageExists(imageURL string) (*image.ImageMetadata, error) {
	if m.existingImages[imageURL] {
		return &image.ImageMetadata{
			Exists: true,
			Digest: "sha256:mockdigest",
		}, nil
	}
	return &image.ImageMetadata{
		Exists: false,
	}, nil
}

func (m *mockImageChecker) CheckImageExistsForPlatform(imageURL, os, arch string) (*image.ImageMetadata, error) {
	return m.CheckImageExists(imageURL)
}
