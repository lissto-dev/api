package common

import (
	"fmt"
	"strings"

	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
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

// ParseScopedReference parses a scoped reference and returns namespace, name
// Falls back to empty namespace for legacy format (name only)
// This is a convenience wrapper around ParseBlueprintReference that returns empty namespace on error
func ParseScopedReference(idParam string) (namespace, name string) {
	ns, n, err := ParseBlueprintReference(idParam)
	if err != nil {
		// Legacy format: just name
		return "", idParam
	}
	return ns, n
}

// ResolveNamespaceFromID parses a scoped ID and validates against allowed namespaces
// Returns:
//   - targetNamespace: the specific namespace to search (if scoped ID and allowed)
//   - name: the resource name
//   - searchAllowed: true if should search all allowed namespaces (legacy behavior)
//
// This centralizes the logic for handling both scoped IDs and legacy format
func ResolveNamespaceFromID(idParam string, allowedNamespaces []string) (targetNamespace, name string, searchAllowed bool) {
	// Parse the ID (could be scoped like "daniel/ID" or legacy format "ID")
	parsedNS, parsedName := ParseScopedReference(idParam)

	// If scoped reference, check if user can access that namespace
	if parsedNS != "" {
		if IsNamespaceAllowed(parsedNS, allowedNamespaces) {
			// User can access this specific namespace
			return parsedNS, parsedName, false
		}
		// User cannot access this namespace - return empty to trigger 404
		return "", parsedName, false
	}

	// Legacy format: search all allowed namespaces
	return "", parsedName, true
}

// ResolveNamespacesToSearch determines which namespaces to search for a resource
// Returns an ordered list of namespaces to try (user namespace first, then global if allowed)
func ResolveNamespacesToSearch(targetNS, userNS, globalNS string, searchAll bool, allowedNS []string) []string {
	// Scoped ID: search only that specific namespace
	if targetNS != "" {
		return []string{targetNS}
	}

	// Not authorized for scoped namespace
	if !searchAll {
		return []string{}
	}

	// Legacy ID: try user namespace first
	namespaces := []string{userNS}

	// Add global if allowed (for read operations)
	if IsNamespaceAllowed(globalNS, allowedNS) {
		namespaces = append(namespaces, globalNS)
	}

	return namespaces
}

// IsNamespaceAllowed checks if a namespace is in the allowed list
// Supports wildcard "*" which allows all namespaces
func IsNamespaceAllowed(namespace string, allowedNamespaces []string) bool {
	if len(allowedNamespaces) > 0 && allowedNamespaces[0] == "*" {
		return true
	}
	for _, allowed := range allowedNamespaces {
		if allowed == namespace {
			return true
		}
	}
	return false
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

// ExtractBlueprintTitle extracts the title from blueprint annotations
// Falls back to the provided fallback value if annotation is not present or empty
func ExtractBlueprintTitle(bp *envv1alpha1.Blueprint, fallback string) string {
	if bp.Annotations != nil {
		if title, ok := bp.Annotations["lissto.dev/title"]; ok && title != "" {
			return title
		}
	}
	return fallback
}
