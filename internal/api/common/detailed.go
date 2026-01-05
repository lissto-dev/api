package common

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// DetailedMetadata represents k8s object metadata for detailed API responses
type DetailedMetadata struct {
	Name        string            `json:"name"`
	Namespace   string            `json:"namespace"` // Normalized: "global" or developer name
	Labels      map[string]string `json:"labels,omitempty"`
	Annotations map[string]string `json:"annotations,omitempty"`
	CreatedAt   string            `json:"createdAt,omitempty"`
}

// ExtractDetailedMetadata extracts and normalizes metadata from k8s objects
// Uses the controller's NormalizeToScope for namespace normalization
// Returns error if namespace type is unknown
func ExtractDetailedMetadata(obj metav1.ObjectMeta, nsManager *authz.NamespaceManager) (DetailedMetadata, error) {
	normalizedNS, err := nsManager.NormalizeToScope(obj.Namespace)
	if err != nil {
		return DetailedMetadata{}, fmt.Errorf("failed to normalize namespace: %w", err)
	}

	return DetailedMetadata{
		Name:        obj.Name,
		Namespace:   normalizedNS,
		Labels:      obj.Labels,
		Annotations: obj.Annotations,
		CreatedAt:   obj.CreationTimestamp.Format(time.RFC3339),
	}, nil
}

// DetailedResponse represents a k8s resource with full metadata and spec
// This is a generic response type used by all resources (Blueprint, Stack, Env, etc.)
type DetailedResponse struct {
	Metadata DetailedMetadata `json:"metadata"`
	Spec     interface{}      `json:"spec"`
}

// NewDetailedResponse creates a detailed response from a k8s object
// The spec parameter should be the resource's Spec field (e.g., blueprint.Spec, stack.Spec)
func NewDetailedResponse(obj metav1.ObjectMeta, spec interface{}, nsManager *authz.NamespaceManager) (DetailedResponse, error) {
	metadata, err := ExtractDetailedMetadata(obj, nsManager)
	if err != nil {
		return DetailedResponse{}, err
	}

	return DetailedResponse{
		Metadata: metadata,
		Spec:     spec,
	}, nil
}

// Formattable is an interface for resources that can be formatted as detailed or standard
// This follows the same pattern as fmt.Stringer
type Formattable interface {
	ToDetailed() (DetailedResponse, error)
	ToStandard() interface{}
}

// HandleFormatResponse handles ?format=detailed query parameter for any Formattable resource
// This is the single place where format logic lives - called by ALL resource handlers
func HandleFormatResponse(c echo.Context, resource Formattable) error {
	format := c.QueryParam("format")

	if format == "detailed" {
		detailed, err := resource.ToDetailed()
		if err != nil {
			logging.Logger.Error("Failed to format detailed response", zap.Error(err))
			return c.String(500, "Failed to extract resource details")
		}
		return c.JSON(200, detailed)
	}

	// Default: return standard format
	return c.JSON(200, resource.ToStandard())
}
