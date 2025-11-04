package image

import (
	"testing"
)

func TestFormatImageWithDigest(t *testing.T) {
	tests := []struct {
		name     string
		imageURL string
		digest   string
		expected string
	}{
		{
			name:     "nginx:latest with digest",
			imageURL: "nginx:latest",
			digest:   "sha256:029d4461bd98f124e531380505ceea2072418fdf28752aa73b7b273ba3048903",
			expected: "nginx@sha256:029d4461bd98f124e531380505ceea2072418fdf28752aa73b7b273ba3048903",
		},
		{
			name:     "nginx:1.25-alpine with digest",
			imageURL: "nginx:1.25-alpine",
			digest:   "sha256:516475cc129da42866742567714ddc681e5eed7b9ee0b9e9c015e464b4221a00",
			expected: "nginx@sha256:516475cc129da42866742567714ddc681e5eed7b9ee0b9e9c015e464b4221a00",
		},
		{
			name:     "registry.com/nginx:latest with digest",
			imageURL: "registry.com/nginx:latest",
			digest:   "sha256:abc123",
			expected: "registry.com/nginx@sha256:abc123",
		},
		{
			name:     "registry with port and namespace",
			imageURL: "registry.com:5000/namespace/nginx:latest",
			digest:   "sha256:abc123",
			expected: "registry.com:5000/namespace/nginx@sha256:abc123",
		},
		{
			name:     "image without tag",
			imageURL: "nginx",
			digest:   "sha256:abc123",
			expected: "nginx@sha256:abc123",
		},
		{
			name:     "image already with digest (replace)",
			imageURL: "nginx@sha256:old-digest",
			digest:   "sha256:new-digest",
			expected: "nginx@sha256:new-digest",
		},
		{
			name:     "complex registry path",
			imageURL: "myregistry.com:8080/myorg/myproject/myapp:v1.2.3",
			digest:   "sha256:def456",
			expected: "myregistry.com:8080/myorg/myproject/myapp@sha256:def456",
		},
		{
			name:     "image with numeric tag",
			imageURL: "nginx:1.25",
			digest:   "sha256:ghi789",
			expected: "nginx@sha256:ghi789",
		},
	}

	resolver := &ImageResolver{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.formatImageWithDigest(tt.imageURL, tt.digest)
			if result != tt.expected {
				t.Errorf("formatImageWithDigest(%q, %q) = %q, want %q",
					tt.imageURL, tt.digest, result, tt.expected)
			}
		})
	}
}

func TestFormatImageWithDigestEdgeCases(t *testing.T) {
	tests := []struct {
		name     string
		imageURL string
		digest   string
		expected string
	}{
		{
			name:     "empty image URL",
			imageURL: "",
			digest:   "sha256:abc123",
			expected: "@sha256:abc123",
		},
		{
			name:     "image URL with multiple colons (port)",
			imageURL: "localhost:5000/nginx:latest",
			digest:   "sha256:abc123",
			expected: "localhost:5000/nginx@sha256:abc123",
		},
		{
			name:     "image URL with IPv6-like format",
			imageURL: "registry:5000/nginx:latest",
			digest:   "sha256:abc123",
			expected: "registry:5000/nginx@sha256:abc123",
		},
	}

	resolver := &ImageResolver{}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := resolver.formatImageWithDigest(tt.imageURL, tt.digest)
			if result != tt.expected {
				t.Errorf("formatImageWithDigest(%q, %q) = %q, want %q",
					tt.imageURL, tt.digest, result, tt.expected)
			}
		})
	}
}
