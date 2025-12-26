package compose_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/compose"
	"github.com/lissto-dev/controller/pkg/config"
)

var _ = Describe("Parser", func() {
	Describe("ParseBlueprintMetadata", func() {
		Context("with x-lissto title", func() {
			It("should extract title from x-lissto extension", func() {
				composeContent := `
version: "3.8"
x-lissto:
  title: "My Application"

services:
  app:
    image: myapp:latest
`

				metadata, err := compose.ParseBlueprintMetadata(composeContent, config.RepoConfig{})
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata.Title).To(Equal("My Application"))
			})
		})

		Context("with title priority", func() {
			It("should prioritize x-lissto.title over repo.Name", func() {
				composeContent := `
version: "3.8"
x-lissto:
  title: "Override Title"

services:
  app:
    image: myapp:latest
`

				repoConfig := config.RepoConfig{
					Name: "Repo Name",
					URL:  "git@github.com:user/repo.git",
				}

				metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata.Title).To(Equal("Override Title"))
			})

			It("should use repo.Name when x-lissto.title is absent", func() {
				composeContent := `
version: "3.8"
services:
  app:
    image: myapp:latest
`

				repoConfig := config.RepoConfig{
					Name: "My Service",
					URL:  "git@github.com:user/repo.git",
				}

				metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata.Title).To(Equal("My Service"))
			})

			It("should fall back to normalized repo.URL", func() {
				composeContent := `
version: "3.8"
services:
  app:
    image: myapp:latest
`

				repoConfig := config.RepoConfig{
					URL: "git@github.com:user/repo.git",
				}

				metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
				Expect(err).ToNot(HaveOccurred())
				Expect(metadata.Title).To(Equal("github.com/user/repo"))
			})
		})

		Context("with invalid YAML", func() {
			It("should return an error", func() {
				invalidCompose := `
this is not valid YAML
  - broken
    indentation
`

				metadata, err := compose.ParseBlueprintMetadata(invalidCompose, config.RepoConfig{})
				Expect(err).To(HaveOccurred())
				Expect(metadata).To(BeNil())
			})
		})
	})

	Describe("ServiceMetadataToJSON", func() {
		It("should convert ServiceMetadata to JSON", func() {
			metadata := compose.ServiceMetadata{
				Services: []string{"api", "worker"},
				Infra:    []string{"postgres", "redis"},
			}

			jsonStr, err := compose.ServiceMetadataToJSON(metadata)
			Expect(err).ToNot(HaveOccurred())
			Expect(jsonStr).To(ContainSubstring("api"))
			Expect(jsonStr).To(ContainSubstring("postgres"))
		})
	})

	Describe("ServiceMetadataFromJSON", func() {
		It("should parse JSON to ServiceMetadata", func() {
			jsonStr := `{"services":["api","worker"],"infra":["postgres","redis"]}`

			metadata, err := compose.ServiceMetadataFromJSON(jsonStr)
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata.Services).To(Equal([]string{"api", "worker"}))
			Expect(metadata.Infra).To(Equal([]string{"postgres", "redis"}))
		})

		It("should handle empty JSON string", func() {
			metadata, err := compose.ServiceMetadataFromJSON("")
			Expect(err).ToNot(HaveOccurred())
			Expect(metadata.Services).To(BeEmpty())
			Expect(metadata.Infra).To(BeEmpty())
		})
	})
})
