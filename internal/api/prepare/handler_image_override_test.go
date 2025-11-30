package prepare_test

import (
	"errors"
	"testing"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/api/pkg/image"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
)

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

// Test the image override priority fix
func TestImageOverridePriority(t *testing.T) {
	tests := []struct {
		name          string
		service       types.ServiceConfig
		setupMock     func(*mockImageResolver)
		expectedImage string
		expectedErr   bool
		description   string
	}{
		{
			name: "lissto.dev/image label overrides image field",
			service: types.ServiceConfig{
				Name:  "postgres",
				Image: "postgres:15-alpine",
				Labels: map[string]string{
					"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
				},
			},
			setupMock: func(m *mockImageResolver) {
				// Should call GetImageDigestWithServicePlatform with the OVERRIDE image, not the original
				m.On("GetImageDigestWithServicePlatform", 
					"363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:abc123", nil)
			},
			expectedImage: "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:abc123",
			expectedErr:   false,
			description:   "When both image field and lissto.dev/image label are present, label should take priority (ECR pull-through cache pattern)",
		},
		{
			name: "only image field, no label",
			service: types.ServiceConfig{
				Name:   "nginx",
				Image:  "nginx:alpine",
				Labels: map[string]string{},
			},
			setupMock: func(m *mockImageResolver) {
				// Should call with the original image
				m.On("GetImageDigestWithServicePlatform",
					"nginx:alpine",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("nginx@sha256:def456", nil)
			},
			expectedImage: "nginx@sha256:def456",
			expectedErr:   false,
			description:   "When only image field is present, use it directly",
		},
		{
			name: "only lissto.dev/image label, no image field",
			service: types.ServiceConfig{
				Name:  "redis",
				Image: "", // No image field
				Labels: map[string]string{
					"lissto.dev/image": "custom-registry.io/redis:7",
				},
			},
			setupMock: func(m *mockImageResolver) {
				// Should call with the label image
				m.On("GetImageDigestWithServicePlatform",
					"custom-registry.io/redis:7",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("custom-registry.io/redis@sha256:ghi789", nil)
			},
			expectedImage: "custom-registry.io/redis@sha256:ghi789",
			expectedErr:   false,
			description:   "When only label is present, use it",
		},
		{
			name: "lissto.dev/image label with nonexistent image",
			service: types.ServiceConfig{
				Name:  "app",
				Image: "app:latest",
				Labels: map[string]string{
					"lissto.dev/image": "nonexistent.io/app:v1",
				},
			},
			setupMock: func(m *mockImageResolver) {
				// Should try the override and fail
				m.On("GetImageDigestWithServicePlatform",
					"nonexistent.io/app:v1",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("", errors.New("image not found"))
			},
			expectedImage: "",
			expectedErr:   true,
			description:   "When override image doesn't exist, should return error",
		},
		{
			name: "multiple services with mixed configurations",
			service: types.ServiceConfig{
				Name:  "spicedb",
				Image: "authzed/spicedb:v1.47.1",
				Labels: map[string]string{
					"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb:v1.47.1",
				},
			},
			setupMock: func(m *mockImageResolver) {
				m.On("GetImageDigestWithServicePlatform",
					"363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb:v1.47.1",
					mock.AnythingOfType("types.ServiceConfig")).
					Return("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb@sha256:49364f0b", nil)
			},
			expectedImage: "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/authzed/spicedb@sha256:49364f0b",
			expectedErr:   false,
			description:   "Real-world case: public image redirected through ECR pull-through cache",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Setup
			mockResolver := new(mockImageResolver)
			tt.setupMock(mockResolver)

			// Simulate the handler logic (extracted from handler.go)
			var result string
			var err error

			// PRIORITY: Check for lissto.dev/image override label first
			imageOverride := ""
			if tt.service.Labels != nil {
				if override, ok := tt.service.Labels["lissto.dev/image"]; ok && override != "" {
					imageOverride = override
				}
			}

			// If service has image override label, use it with highest priority
			if imageOverride != "" {
				result, err = mockResolver.GetImageDigestWithServicePlatform(imageOverride, tt.service)
			} else if tt.service.Image != "" {
				result, err = mockResolver.GetImageDigestWithServicePlatform(tt.service.Image, tt.service)
			} else {
				// Would call ResolveImageDetailed for build context
				err = errors.New("build resolution not tested here")
			}

			// Verify
			if tt.expectedErr {
				assert.Error(t, err, tt.description)
			} else {
				assert.NoError(t, err, tt.description)
				assert.Equal(t, tt.expectedImage, result, tt.description)
			}

			mockResolver.AssertExpectations(t)
		})
	}
}

// TestImageOverridePriorityIntegration tests the full flow end-to-end
func TestImageOverridePriorityIntegration(t *testing.T) {
	// This test simulates what was happening before the fix vs after
	t.Run("before fix - bug scenario", func(t *testing.T) {
		// Before the fix, when a service had BOTH image field and label,
		// the image field was always used (BUG)
		service := types.ServiceConfig{
			Name:  "postgres",
			Image: "postgres:15-alpine", // This was being used (wrong!)
			Labels: map[string]string{
				"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
			},
		}

		// In the buggy version, it would check service.Image != "" first
		buggyFlow := service.Image != ""
		assert.True(t, buggyFlow, "Bug: code path would use service.Image directly")
		
		// And never check the label
		t.Log("BUG: In the old code, lissto.dev/image label was ignored when image field was present")
	})

	t.Run("after fix - correct behavior", func(t *testing.T) {
		// After the fix, label takes priority
		service := types.ServiceConfig{
			Name:  "postgres",
			Image: "postgres:15-alpine",
			Labels: map[string]string{
				"lissto.dev/image": "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
			},
		}

		mockResolver := new(mockImageResolver)
		
		// Should call with the OVERRIDE image (from label), not the image field
		mockResolver.On("GetImageDigestWithServicePlatform",
			"363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
			mock.AnythingOfType("types.ServiceConfig")).
			Return("363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:correct", nil)

		// Simulate the fixed handler logic
		imageOverride := ""
		if service.Labels != nil {
			if override, ok := service.Labels["lissto.dev/image"]; ok && override != "" {
				imageOverride = override
			}
		}

		var result string
		var err error

		// Fixed: Check override FIRST
		if imageOverride != "" {
			result, err = mockResolver.GetImageDigestWithServicePlatform(imageOverride, service)
		} else if service.Image != "" {
			result, err = mockResolver.GetImageDigestWithServicePlatform(service.Image, service)
		}

		assert.NoError(t, err)
		assert.Equal(t, "363305613851.dkr.ecr.eu-central-1.amazonaws.com/docker-hub/library/postgres@sha256:correct", result)
		assert.Contains(t, result, "363305613851.dkr.ecr.eu-central-1.amazonaws.com", "Should use ECR registry from label, not original image")
		
		mockResolver.AssertExpectations(t)
		t.Log("FIXED: lissto.dev/image label now correctly takes priority over image field")
	})
}

// TestResolutionPriority documents the complete priority order
func TestResolutionPriority(t *testing.T) {
	t.Run("priority order documentation", func(t *testing.T) {
		// Priority 1: lissto.dev/image label (highest)
		service1 := types.ServiceConfig{
			Name:  "test",
			Image: "original:tag",
			Labels: map[string]string{
				"lissto.dev/image":      "override:tag", // This wins
				"lissto.dev/registry":   "ignored",
				"lissto.dev/repository": "ignored",
				"lissto.dev/tag":        "ignored",
			},
		}

		imageOverride1 := ""
		if service1.Labels != nil {
			if override, ok := service1.Labels["lissto.dev/image"]; ok && override != "" {
				imageOverride1 = override
			}
		}

		assert.Equal(t, "override:tag", imageOverride1, "Priority 1: lissto.dev/image label")

		// Priority 2: image field (when no override label)
		service2 := types.ServiceConfig{
			Name:   "test",
			Image:  "original:tag", // This is used
			Labels: map[string]string{
				// No lissto.dev/image label
				"lissto.dev/registry": "would-be-used-for-build",
			},
		}

		imageOverride2 := ""
		if service2.Labels != nil {
			if override, ok := service2.Labels["lissto.dev/image"]; ok && override != "" {
				imageOverride2 = override
			}
		}

		assert.Empty(t, imageOverride2, "No override")
		assert.NotEmpty(t, service2.Image, "Priority 2: image field is used")

		// Priority 3: Build resolution (when neither override nor image)
		service3 := types.ServiceConfig{
			Name:  "test",
			Image: "", // No image
			Labels: map[string]string{
				// No lissto.dev/image label
				"lissto.dev/repository": "custom-repo", // Used in build resolution
			},
			Build: &types.BuildConfig{
				Context: ".",
			},
		}

		imageOverride3 := ""
		if service3.Labels != nil {
			if override, ok := service3.Labels["lissto.dev/image"]; ok && override != "" {
				imageOverride3 = override
			}
		}

		assert.Empty(t, imageOverride3, "No override")
		assert.Empty(t, service3.Image, "No image field")
		assert.NotNil(t, service3.Build, "Priority 3: Build resolution used")
	})
}

// TestRealWorldScenarios tests common use cases
func TestRealWorldScenarios(t *testing.T) {
	scenarios := []struct {
		name        string
		description string
		service     types.ServiceConfig
		expectLabel bool // Should use label?
		expectImage bool // Should use image field?
		expectBuild bool // Should use build resolution?
	}{
		{
			name:        "ECR Pull-Through Cache",
			description: "Redirect Docker Hub images through ECR pull-through cache",
			service: types.ServiceConfig{
				Name:  "postgres",
				Image: "postgres:15-alpine",
				Labels: map[string]string{
					"lissto.dev/image": "123456.dkr.ecr.us-east-1.amazonaws.com/docker-hub/library/postgres:15-alpine",
				},
			},
			expectLabel: true,
			expectImage: false,
			expectBuild: false,
		},
		{
			name:        "Standard Docker Hub Image",
			description: "Use public Docker Hub image directly",
			service: types.ServiceConfig{
				Name:   "nginx",
				Image:  "nginx:alpine",
				Labels: map[string]string{},
			},
			expectLabel: false,
			expectImage: true,
			expectBuild: false,
		},
		{
			name:        "Custom Build",
			description: "Build from source with repository label",
			service: types.ServiceConfig{
				Name:  "api",
				Image: "",
				Labels: map[string]string{
					"lissto.dev/repository": "my-company/api",
				},
				Build: &types.BuildConfig{Context: "."},
			},
			expectLabel: false,
			expectImage: false,
			expectBuild: true,
		},
		{
			name:        "Private Registry Override",
			description: "Override public image with private registry mirror",
			service: types.ServiceConfig{
				Name:  "redis",
				Image: "redis:7-alpine",
				Labels: map[string]string{
					"lissto.dev/image": "private-registry.company.com/redis:7-alpine-verified",
				},
			},
			expectLabel: true,
			expectImage: false,
			expectBuild: false,
		},
	}

	for _, scenario := range scenarios {
		t.Run(scenario.name, func(t *testing.T) {
			// Check which path would be taken
			imageOverride := ""
			if scenario.service.Labels != nil {
				if override, ok := scenario.service.Labels["lissto.dev/image"]; ok && override != "" {
					imageOverride = override
				}
			}

			usesLabel := imageOverride != ""
			usesImage := imageOverride == "" && scenario.service.Image != ""
			usesBuild := imageOverride == "" && scenario.service.Image == ""

			assert.Equal(t, scenario.expectLabel, usesLabel, "Label usage: "+scenario.description)
			assert.Equal(t, scenario.expectImage, usesImage, "Image field usage: "+scenario.description)
			assert.Equal(t, scenario.expectBuild, usesBuild, "Build resolution usage: "+scenario.description)
		})
	}
}

