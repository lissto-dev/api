package compose_test

import (
	"context"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/compose"
	"github.com/lissto-dev/controller/pkg/config"
)

var _ = Describe("ExtractLisstoConfig", func() {
	var parseCompose = func(content string) *types.Project {
		project, err := loader.LoadWithContext(
			context.Background(),
			types.ConfigDetails{
				ConfigFiles: []types.ConfigFile{
					{
						Filename: "docker-compose.yml",
						Content:  []byte(content),
					},
				},
				WorkingDir: "/tmp",
			},
			loader.WithSkipValidation,
		)
		Expect(err).NotTo(HaveOccurred(), "Failed to parse compose file")
		return project
	}

	Context("with registry and repositoryPrefix", func() {
		It("should extract both values correctly", func() {
			composeContent := `
services:
  web:
    image: nginx:latest
  api:
    build: .

x-lissto:
  title: My App
  registry: 123.ecr.aws.amazon.com
  repositoryPrefix: lissto-org/
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(Equal("123.ecr.aws.amazon.com"))
			Expect(config.Repository).To(BeEmpty())
			Expect(config.RepositoryPrefix).To(Equal("lissto-org/"))
		})
	})

	Context("with registry and repository (single image for all services)", func() {
		It("should extract both values for monorepo pattern", func() {
			composeContent := `
services:
  web:
    build: .
  api:
    build: .
  worker:
    build: .

x-lissto:
  title: Monorepo App
  registry: 123.ecr.aws.amazon.com
  repository: my-monorepo-image
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(Equal("123.ecr.aws.amazon.com"))
			Expect(config.Repository).To(Equal("my-monorepo-image"))
			Expect(config.RepositoryPrefix).To(BeEmpty())
		})
	})

	Context("with only registry", func() {
		It("should extract only registry value", func() {
			composeContent := `
services:
  web:
    image: nginx:latest

x-lissto:
  registry: my-registry.io
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(Equal("my-registry.io"))
			Expect(config.Repository).To(BeEmpty())
			Expect(config.RepositoryPrefix).To(BeEmpty())
		})
	})

	Context("with only repository", func() {
		It("should extract only repository value", func() {
			composeContent := `
services:
  web:
    build: .

x-lissto:
  repository: shared-monorepo
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(BeEmpty())
			Expect(config.Repository).To(Equal("shared-monorepo"))
			Expect(config.RepositoryPrefix).To(BeEmpty())
		})
	})

	Context("with only repositoryPrefix", func() {
		It("should extract only repositoryPrefix value", func() {
			composeContent := `
services:
  web:
    image: nginx:latest

x-lissto:
  repositoryPrefix: my-prefix-
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(BeEmpty())
			Expect(config.Repository).To(BeEmpty())
			Expect(config.RepositoryPrefix).To(Equal("my-prefix-"))
		})
	})

	Context("with no x-lissto section", func() {
		It("should return empty config", func() {
			composeContent := `
services:
  web:
    image: nginx:latest
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(BeEmpty())
			Expect(config.Repository).To(BeEmpty())
			Expect(config.RepositoryPrefix).To(BeEmpty())
		})
	})

	Context("with empty x-lissto section", func() {
		It("should return empty config", func() {
			composeContent := `
services:
  web:
    image: nginx:latest

x-lissto: {}
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config.Registry).To(BeEmpty())
			Expect(config.Repository).To(BeEmpty())
			Expect(config.RepositoryPrefix).To(BeEmpty())
		})
	})

	Describe("real-world example", func() {
		It("should extract values from actual docker-compose-build.yaml format", func() {
			composeContent := `
services:
  postgres:
    image: postgres:15-alpine
    labels:
      lissto.dev/service: postgres

  operator:
    build:
      context: ./operator
      dockerfile: operator/Dockerfile
    labels:
      lissto.dev/expose: true

volumes:
  postgres_data:

x-lissto:
  title: lissto.dev/playground-go
  registry: 123.ecr.aws.amazon.com
  repositoryPrefix: lissto-org/
  hostSuffix: .dev.lissto.dev
  ingressClass: haproxy
`
			project := parseCompose(composeContent)
			config := compose.ExtractLisstoConfig(project)

			Expect(config).To(SatisfyAll(
				HaveField("Registry", "123.ecr.aws.amazon.com"),
				HaveField("RepositoryPrefix", "lissto-org/"),
			))
		})
	})
})

var _ = Describe("ParseBlueprintMetadata - Title Priority", func() {
	Context("Title extraction priority", func() {
		It("should prioritize x-lissto.title over repo.Name and repo.URL", func() {
			composeContent := `
services:
  web:
    build: .

x-lissto:
  title: "Explicit Title from Compose"
`
			repoConfig := config.RepoConfig{
				URL:      "https://github.com/lissto/lissto",
				Name:     "Lissto Repository",
				Branches: []string{"main"},
			}

			metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.Title).To(Equal("Explicit Title from Compose"))
		})

		It("should use repo.Name when x-lissto.title is not provided", func() {
			composeContent := `
services:
  web:
    build: .
`
			repoConfig := config.RepoConfig{
				URL:      "https://github.com/lissto/lissto",
				Name:     "Lissto Repository",
				Branches: []string{"main"},
			}

			metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.Title).To(Equal("Lissto Repository"))
		})

		It("should use normalized repo.URL when repo.Name is empty", func() {
			composeContent := `
services:
  web:
    build: .
`
			repoConfig := config.RepoConfig{
				URL:      "https://github.com/lissto/lissto.git",
				Name:     "", // No name configured
				Branches: []string{"main"},
			}

			metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.Title).To(Equal("github.com/lissto/lissto"))
		})

		It("should use normalized repo.URL for git@github.com format", func() {
			composeContent := `
services:
  web:
    build: .
`
			repoConfig := config.RepoConfig{
				URL:      "git@github.com:lissto/lissto.git",
				Name:     "", // No name configured
				Branches: []string{"main"},
			}

			metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.Title).To(Equal("github.com/lissto/lissto"))
		})

		It("should ignore empty x-lissto.title and fallback to repo.Name", func() {
			composeContent := `
services:
  web:
    build: .

x-lissto:
  title: ""
  registry: 123.ecr.aws.amazon.com
`
			repoConfig := config.RepoConfig{
				URL:      "https://github.com/lissto/lissto",
				Name:     "Lissto",
				Branches: []string{"main"},
			}

			metadata, err := compose.ParseBlueprintMetadata(composeContent, repoConfig)
			Expect(err).NotTo(HaveOccurred())
			Expect(metadata.Title).To(Equal("Lissto"))
		})
	})
})
