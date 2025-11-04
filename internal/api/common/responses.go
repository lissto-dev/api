package common

import (
	"fmt"
	"strings"
)

// GenerateScopedIdentifier creates scope/name format
// Example: global/20250120-150530-a3f5b2c1 or john/20250120-150530-a3f5b2c1
func GenerateScopedIdentifier(namespace, name string) string {
	scope := "global"

	// Check if namespace contains "global"
	if !strings.Contains(namespace, "global") {
		// Extract author from dev-<author>
		parts := strings.Split(namespace, "-")
		if len(parts) > 1 {
			scope = parts[1]
		}
	}

	return fmt.Sprintf("%s/%s", scope, name)
}

// ParseBlueprintReference parses a scoped blueprint reference into namespace and name
// Input: "john/20250120-150530-a3f5b2c1" or "global/20250120-150530-a3f5b2c1"
// Output: namespace ("dev-john" or "lissto-global"), name ("20250120-150530-a3f5b2c1")
func ParseBlueprintReference(blueprintRef string) (namespace, name string, err error) {
	parts := strings.Split(blueprintRef, "/")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid blueprint reference format: %s (expected scope/name)", blueprintRef)
	}

	scope := parts[0]
	name = parts[1]

	// Convert scope to actual namespace
	if scope == "global" {
		namespace = "lissto-global"
	} else {
		// Convert scope to dev-{scope} format
		namespace = fmt.Sprintf("dev-%s", scope)
	}

	return namespace, name, nil
}

// ImageCandidate represents a single image candidate that was tried
type ImageCandidate struct {
	ImageURL string `json:"image_url"`        // Full image URL that was tried
	Tag      string `json:"tag"`              // Tag that was tried
	Source   string `json:"source"`           // Source of the tag: "label", "commit", "branch", "latest"
	Success  bool   `json:"success"`          // Whether this candidate succeeded
	Error    string `json:"error,omitempty"`  // Error message if failed
	Digest   string `json:"digest,omitempty"` // Digest if successful
}

// ImageResolutionInfo contains minimal info about resolved image
type ImageResolutionInfo struct {
	Service string `json:"service"`
	Image   string `json:"image"`         // Final image with digest
	Method  string `json:"method"`        // "original", "label", "commit", "branch", "latest"
	Tag     string `json:"tag,omitempty"` // User-friendly tag (if resolved)
}

// DetailedImageResolutionInfo contains detailed info about image resolution process
type DetailedImageResolutionInfo struct {
	Service    string           `json:"service"`
	Digest     string           `json:"digest"`               // Full image digest
	Image      string           `json:"image,omitempty"`      // User-friendly image tag
	Method     string           `json:"method"`               // "original", "label", "commit", "branch", "latest"
	Registry   string           `json:"registry,omitempty"`   // Registry used
	ImageName  string           `json:"image_name,omitempty"` // Image name resolved
	Candidates []ImageCandidate `json:"candidates,omitempty"` // All candidates that were tried
	Exposed    bool             `json:"exposed,omitempty"`    // Whether this service is exposed
	URL        string           `json:"url,omitempty"`        // Expected URL if exposed and env provided
}

// PrepareStackResponse contains the result of stack preparation
type PrepareStackResponse struct {
	Blueprint string                `json:"blueprint"`
	Images    []ImageResolutionInfo `json:"images"`
}

// DetailedPrepareStackResponse contains detailed result of stack preparation
type DetailedPrepareStackResponse struct {
	RequestID string                        `json:"request_id"` // UUID for caching and stack creation
	Blueprint string                        `json:"blueprint"`
	Images    []DetailedImageResolutionInfo `json:"images"`
	Exposed   []ExposedServiceInfo          `json:"exposed,omitempty"` // List of exposed services with URLs
}

// ExposedServiceInfo contains information about an exposed service
type ExposedServiceInfo struct {
	Service string `json:"service"` // Service name
	URL     string `json:"url"`     // Expected endpoint URL (e.g., "operator-daniel.dev.lissto.dev")
}

// EnvResponse represents an env resource
type EnvResponse struct {
	ID   string `json:"id"`   // Scoped identifier: namespace/envname
	Name string `json:"name"` // Env name (metadata.name)
}

// UserInfoResponse represents the authenticated user's information
type UserInfoResponse struct {
	Name string `json:"name"` // Lissto username (from API key)
	Role string `json:"role"` // User role
}
