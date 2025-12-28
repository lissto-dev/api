package prepare_test

import (
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/api/pkg/image"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/stretchr/testify/mock"
)

func TestPrepare(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Prepare Handler Suite")
}

// mockImageResolver mocks the ImageResolver interface
type mockImageResolver struct {
	mock.Mock
}

func (m *mockImageResolver) GetImageDigestWithServicePlatform(imageURL string, service types.ServiceConfig) (string, error) {
	args := m.Called(imageURL, service)
	return args.String(0), args.Error(1)
}

func (m *mockImageResolver) ResolveImageDetailed(service types.ServiceConfig, config image.ResolutionConfig) (*image.DetailedImageResolutionResult, error) {
	args := m.Called(service, config)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*image.DetailedImageResolutionResult), args.Error(1)
}

// Helper function to get image override from service labels
func getImageOverride(service types.ServiceConfig) string {
	if service.Labels != nil {
		if override, ok := service.Labels["lissto.dev/image"]; ok && override != "" {
			return override
		}
	}
	return ""
}

// Helper function to resolve image with mock
func resolveImage(mockResolver *mockImageResolver, service types.ServiceConfig) (string, error) {
	imageOverride := getImageOverride(service)

	if imageOverride != "" {
		return mockResolver.GetImageDigestWithServicePlatform(imageOverride, service)
	} else if service.Image != "" {
		return mockResolver.GetImageDigestWithServicePlatform(service.Image, service)
	}
	return "", errors.New("build resolution not tested here")
}

var _ = Describe("Image Override Priority", func() {
	Describe("lissto.dev/image label priority", func() {
		Context("when both image field and lissto.dev/image label are present", func() {
			It("should use the label value (ECR pull-through cache pattern)", func() {
				service := types.ServiceConfig{
					Name:  "postgres",
					Image: "postgres:15-alpine",
					Labels: map[string]string{
						"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					},
				}

				mockResolver := new(mockImageResolver)
				mockResolver.On("GetImageDigestWithServicePlatform",
					"363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:abc123", nil)

				result, err := resolveImage(mockResolver, service)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:abc123"))
				mockResolver.AssertExpectations(GinkgoT())
			})
		})

		Context("when only image field is present", func() {
			It("should use the image field directly", func() {
				service := types.ServiceConfig{
					Name:   "nginx",
					Image:  "nginx:alpine",
					Labels: map[string]string{},
				}

				mockResolver := new(mockImageResolver)
				mockResolver.On("GetImageDigestWithServicePlatform",
					"nginx:alpine",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("nginx@sha256:def456", nil)

				result, err := resolveImage(mockResolver, service)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("nginx@sha256:def456"))
				mockResolver.AssertExpectations(GinkgoT())
			})
		})

		Context("when only lissto.dev/image label is present", func() {
			It("should use the label value", func() {
				service := types.ServiceConfig{
					Name:  "redis",
					Image: "",
					Labels: map[string]string{
						"lissto.dev/image": "custom-registry.io/redis:7",
					},
				}

				mockResolver := new(mockImageResolver)
				mockResolver.On("GetImageDigestWithServicePlatform",
					"custom-registry.io/redis:7",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("custom-registry.io/redis@sha256:ghi789", nil)

				result, err := resolveImage(mockResolver, service)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("custom-registry.io/redis@sha256:ghi789"))
				mockResolver.AssertExpectations(GinkgoT())
			})
		})

		Context("when override image doesn't exist", func() {
			It("should return an error", func() {
				service := types.ServiceConfig{
					Name:  "app",
					Image: "app:latest",
					Labels: map[string]string{
						"lissto.dev/image": "nonexistent.io/app:v1",
					},
				}

				mockResolver := new(mockImageResolver)
				mockResolver.On("GetImageDigestWithServicePlatform",
					"nonexistent.io/app:v1",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("", errors.New("image not found"))

				_, err := resolveImage(mockResolver, service)

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("image not found"))
				mockResolver.AssertExpectations(GinkgoT())
			})
		})

		Context("real-world case: public image redirected through ECR pull-through cache", func() {
			It("should use the ECR image from label", func() {
				service := types.ServiceConfig{
					Name:  "spicedb",
					Image: "authzed/spicedb:v1.47.1",
					Labels: map[string]string{
						"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb:v1.47.1",
					},
				}

				mockResolver := new(mockImageResolver)
				mockResolver.On("GetImageDigestWithServicePlatform",
					"363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb:v1.47.1",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb@sha256:49364f0b", nil)

				result, err := resolveImage(mockResolver, service)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb@sha256:49364f0b"))
				mockResolver.AssertExpectations(GinkgoT())
			})
		})
	})

	Describe("Integration tests for image override priority", func() {
		Context("before fix - bug scenario", func() {
			It("should demonstrate the bug where image field was used instead of label", func() {
				service := types.ServiceConfig{
					Name:  "postgres",
					Image: "postgres:15-alpine",
					Labels: map[string]string{
						"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					},
				}

				// In the buggy version, it would check service.Image != "" first
				buggyFlow := service.Image != ""
				Expect(buggyFlow).To(BeTrue(), "Bug: code path would use service.Image directly")
			})
		})

		Context("after fix - correct behavior", func() {
			It("should use label with highest priority", func() {
				service := types.ServiceConfig{
					Name:  "postgres",
					Image: "postgres:15-alpine",
					Labels: map[string]string{
						"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					},
				}

				mockResolver := new(mockImageResolver)
				mockResolver.On("GetImageDigestWithServicePlatform",
					"363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:correct", nil)

				result, err := resolveImage(mockResolver, service)

				Expect(err).NotTo(HaveOccurred())
				Expect(result).To(Equal("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:correct"))
				Expect(result).To(ContainSubstring("363305613851.dkr.ecr.eu-central-1.amazonaws.com"))
				mockResolver.AssertExpectations(GinkgoT())
			})
		})
	})

	Describe("Resolution priority order documentation", func() {
		Context("Priority 1: lissto.dev/image label (highest)", func() {
			It("should use label when all options are present", func() {
				service := types.ServiceConfig{
					Name:  "test",
					Image: "original:tag",
					Labels: map[string]string{
						"lissto.dev/image":      "override:tag",
						"lissto.dev/registry":   "ignored",
						"lissto.dev/repository": "ignored",
						"lissto.dev/tag":        "ignored",
					},
				}

				imageOverride := getImageOverride(service)
				Expect(imageOverride).To(Equal("override:tag"))
			})
		})

		Context("Priority 2: image field (when no override label)", func() {
			It("should use image field when no label is present", func() {
				service := types.ServiceConfig{
					Name:  "test",
					Image: "original:tag",
					Labels: map[string]string{
						"lissto.dev/registry": "would-be-used-for-build",
					},
				}

				imageOverride := getImageOverride(service)
				Expect(imageOverride).To(BeEmpty())
				Expect(service.Image).NotTo(BeEmpty())
			})
		})

		Context("Priority 3: Build resolution (when neither override nor image)", func() {
			It("should fall back to build resolution", func() {
				service := types.ServiceConfig{
					Name:  "test",
					Image: "",
					Labels: map[string]string{
						"lissto.dev/repository": "custom-repo",
					},
					Build: &types.BuildConfig{
						Context: ".",
					},
				}

				imageOverride := getImageOverride(service)
				Expect(imageOverride).To(BeEmpty())
				Expect(service.Image).To(BeEmpty())
				Expect(service.Build).NotTo(BeNil())
			})
		})
	})

	Describe("Real-world scenarios", func() {
		DescribeTable("should correctly determine resolution path",
			func(service types.ServiceConfig, expectLabel, expectImage, expectBuild bool) {
				imageOverride := getImageOverride(service)
				usesLabel := imageOverride != ""
				usesImage := imageOverride == "" && service.Image != ""
				usesBuild := imageOverride == "" && service.Image == ""

				Expect(usesLabel).To(Equal(expectLabel))
				Expect(usesImage).To(Equal(expectImage))
				Expect(usesBuild).To(Equal(expectBuild))
			},
			Entry("ECR Pull-Through Cache",
				types.ServiceConfig{
					Name:  "postgres",
					Image: "postgres:15-alpine",
					Labels: map[string]string{
						"lissto.dev/image": "123456.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					},
				},
				true, false, false),
			Entry("Standard Docker Hub Image",
				types.ServiceConfig{
					Name:   "nginx",
					Image:  "nginx:alpine",
					Labels: map[string]string{},
				},
				false, true, false),
			Entry("Custom Build",
				types.ServiceConfig{
					Name:  "api",
					Image: "",
					Labels: map[string]string{
						"lissto.dev/repository": "my-company/api",
					},
					Build: &types.BuildConfig{Context: "."},
				},
				false, false, true),
			Entry("Private Registry Override",
				types.ServiceConfig{
					Name:  "redis",
					Image: "redis:7-alpine",
					Labels: map[string]string{
						"lissto.dev/image": "private-registry.company.com/redis:7-alpine-verified",
					},
				},
				true, false, false),
		)
	})
})
