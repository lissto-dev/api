package image_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/pkg/image"
)

var _ = Describe("ImageExistenceChecker (mocked)", func() {
	var (
		mockChecker *MockImageChecker
	)

	BeforeEach(func() {
		mockChecker = NewMockImageChecker()

		// Setup common mock responses
		mockChecker.AddResponse("alpine:latest", "linux", "amd64", "sha256:alpine-amd64-digest")
		mockChecker.AddResponse("alpine:latest", "linux", "arm64", "sha256:alpine-arm64-digest")
		mockChecker.AddResponse("nginx:latest", "linux", "amd64", "sha256:nginx-amd64-digest")
		mockChecker.AddResponse("nginx:latest", "linux", "arm64", "sha256:nginx-arm64-digest")
	})

	Describe("CheckImageExists", func() {
		Context("when image exists", func() {
			It("should return metadata with Exists=true for alpine:latest", func() {
				metadata, err := mockChecker.CheckImageExists("alpine:latest")
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Exists).To(BeTrue())
				Expect(metadata.Digest).To(Equal("sha256:alpine-amd64-digest"))
				Expect(metadata.ManifestType).NotTo(BeEmpty())
			})
		})

		Context("when image does not exist", func() {
			It("should return metadata with Exists=false", func() {
				metadata, err := mockChecker.CheckImageExists("nonexistent-registry/nonexistent-image:nonexistent-tag")
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Exists).To(BeFalse())
			})
		})
	})

	Describe("CheckImageExistsForPlatform", func() {
		Context("for linux/amd64", func() {
			It("should return correct digest for alpine:latest", func() {
				metadata, err := mockChecker.CheckImageExistsForPlatform("alpine:latest", "linux", "amd64")
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Exists).To(BeTrue())
				Expect(metadata.Digest).To(Equal("sha256:alpine-amd64-digest"))
				Expect(metadata.Architectures).To(ContainElement("amd64"))
				Expect(metadata.PlatformDigests).To(HaveKey("linux/amd64"))
			})
		})

		Context("for linux/arm64", func() {
			It("should return correct digest for alpine:latest", func() {
				metadata, err := mockChecker.CheckImageExistsForPlatform("alpine:latest", "linux", "arm64")
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Exists).To(BeTrue())
				Expect(metadata.Digest).To(Equal("sha256:alpine-arm64-digest"))
				Expect(metadata.Architectures).To(ContainElement("arm64"))
				Expect(metadata.PlatformDigests).To(HaveKey("linux/arm64"))
			})
		})
	})

	Describe("GetDigestForPlatform", func() {
		Context("for nginx:latest", func() {
			It("should return digest for linux/amd64", func() {
				metadata, err := mockChecker.CheckImageExistsForPlatform("nginx:latest", "linux", "amd64")
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Digest).To(Equal("sha256:nginx-amd64-digest"))
			})

			It("should return digest for linux/arm64", func() {
				metadata, err := mockChecker.CheckImageExistsForPlatform("nginx:latest", "linux", "arm64")
				Expect(err).NotTo(HaveOccurred())
				Expect(metadata.Digest).To(Equal("sha256:nginx-arm64-digest"))
			})
		})
	})
})

var _ = Describe("ImageResolver (mocked)", func() {
	var (
		resolver    *image.ImageResolver
		mockChecker *MockImageChecker
	)

	BeforeEach(func() {
		mockChecker = NewMockImageChecker()
		resolver = image.NewImageResolver("", "", mockChecker)

		// Setup mock responses
		mockChecker.AddResponse("nginx:latest", "linux", "amd64", "sha256:nginx-test-digest")
		mockChecker.AddResponse("nginx:latest", "linux", "arm64", "sha256:nginx-arm64-test-digest")
	})

	Describe("GetImageDigest", func() {
		It("should return image with digest for nginx:latest", func() {
			imageWithDigest, err := resolver.GetImageDigest("nginx:latest")
			Expect(err).NotTo(HaveOccurred())
			Expect(imageWithDigest).To(ContainSubstring("nginx@sha256:nginx-test-digest"))
		})
	})

	Describe("GetImageDigestForPlatform", func() {
		It("should return image with digest for linux/amd64", func() {
			imageWithDigest, err := resolver.GetImageDigestForPlatform("nginx:latest", "linux", "amd64")
			Expect(err).NotTo(HaveOccurred())
			Expect(imageWithDigest).To(ContainSubstring("nginx@sha256:nginx-test-digest"))
		})

		It("should return image with digest for linux/arm64", func() {
			imageWithDigest, err := resolver.GetImageDigestForPlatform("nginx:latest", "linux", "arm64")
			Expect(err).NotTo(HaveOccurred())
			Expect(imageWithDigest).To(ContainSubstring("nginx@sha256:nginx-arm64-test-digest"))
		})
	})
})
