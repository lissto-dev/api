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
