package image

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

var _ = Describe("ImageResolver - formatImageWithDigest", func() {
	var resolver *ImageResolver

	BeforeEach(func() {
		resolver = &ImageResolver{}
	})

	Describe("formatImageWithDigest", func() {
		Context("with standard image formats", func() {
			DescribeTable("should format image with digest correctly",
				func(imageURL, digest, expected string) {
					result := resolver.formatImageWithDigest(imageURL, digest)
					Expect(result).To(Equal(expected))
				},
				Entry("nginx:latest with digest",
					"nginx:latest",
					"sha256:029d4461bd98f124e531380505ceea2072418fdf28752aa73b7b273ba3048903",
					"nginx@sha256:029d4461bd98f124e531380505ceea2072418fdf28752aa73b7b273ba3048903"),
				Entry("nginx:1.25-alpine with digest",
					"nginx:1.25-alpine",
					"sha256:516475cc129da42866742567714ddc681e5eed7b9ee0b9e9c015e464b4221a00",
					"nginx@sha256:516475cc129da42866742567714ddc681e5eed7b9ee0b9e9c015e464b4221a00"),
				Entry("registry.com/nginx:latest with digest",
					"registry.com/nginx:latest",
					"sha256:abc123",
					"registry.com/nginx@sha256:abc123"),
				Entry("registry with port and namespace",
					"registry.com:5000/namespace/nginx:latest",
					"sha256:abc123",
					"registry.com:5000/namespace/nginx@sha256:abc123"),
				Entry("image without tag",
					"nginx",
					"sha256:abc123",
					"nginx@sha256:abc123"),
				Entry("image already with digest (replace)",
					"nginx@sha256:old-digest",
					"sha256:new-digest",
					"nginx@sha256:new-digest"),
				Entry("complex registry path",
					"myregistry.com:8080/myorg/myproject/myapp:v1.2.3",
					"sha256:def456",
					"myregistry.com:8080/myorg/myproject/myapp@sha256:def456"),
				Entry("image with numeric tag",
					"nginx:1.25",
					"sha256:ghi789",
					"nginx@sha256:ghi789"),
			)
		})

		Context("with edge cases", func() {
			DescribeTable("should handle edge cases correctly",
				func(imageURL, digest, expected string) {
					result := resolver.formatImageWithDigest(imageURL, digest)
					Expect(result).To(Equal(expected))
				},
				Entry("empty image URL",
					"",
					"sha256:abc123",
					"@sha256:abc123"),
				Entry("image URL with multiple colons (port)",
					"localhost:5000/nginx:latest",
					"sha256:abc123",
					"localhost:5000/nginx@sha256:abc123"),
				Entry("image URL with IPv6-like format",
					"registry:5000/nginx:latest",
					"sha256:abc123",
					"registry:5000/nginx@sha256:abc123"),
			)
		})
	})
})
