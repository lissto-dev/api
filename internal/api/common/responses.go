package common

import (
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	"github.com/lissto-dev/controller/pkg/namespace"
)

// Re-export namespace functions for backward compatibility.
// These delegate to the controller's namespace package.
var (
	IsNamespaceAllowed        = namespace.IsNamespaceAllowed
	ResolveNamespacesToSearch = namespace.ResolveNamespacesToSearch
)

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

// StackPhaseResponse contains stack phase and lifecycle information
type StackPhaseResponse struct {
	Phase        string                   `json:"phase"`
	PhaseHistory []PhaseTransition        `json:"phaseHistory,omitempty"`
	Services     map[string]ServiceStatus `json:"services,omitempty"`
}

// PhaseTransition records a phase change
type PhaseTransition struct {
	Phase          string `json:"phase"`
	TransitionTime string `json:"transitionTime"`
	Reason         string `json:"reason,omitempty"`
	Message        string `json:"message,omitempty"`
}

// ServiceStatus tracks the status of a single service
type ServiceStatus struct {
	Phase       string `json:"phase"`
	SuspendedAt string `json:"suspendedAt,omitempty"`
}

// LifecycleResponse represents a Lifecycle resource
type LifecycleResponse struct {
	Name       string `json:"name"`
	TargetKind string `json:"targetKind"`
	Interval   string `json:"interval"`
	LastRun    string `json:"lastRun,omitempty"`
	NextRun    string `json:"nextRun,omitempty"`
}

// FormattableLifecycle wraps a Lifecycle to implement Formattable
type FormattableLifecycle struct {
	K8sObj *envv1alpha1.Lifecycle
}

func (f *FormattableLifecycle) ToDetailed() (DetailedResponse, error) {
	return NewDetailedResponse(f.K8sObj.ObjectMeta, f.K8sObj.Spec, nil)
}

func (f *FormattableLifecycle) ToStandard() interface{} {
	return extractLifecycleResponse(f.K8sObj)
}

// extractLifecycleResponse extracts standard data from lifecycle
func extractLifecycleResponse(lifecycle *envv1alpha1.Lifecycle) LifecycleResponse {
	resp := LifecycleResponse{
		Name:       lifecycle.Name,
		TargetKind: lifecycle.Spec.TargetKind,
		Interval:   lifecycle.Spec.Interval.Duration.String(),
	}

	if lifecycle.Status.LastRunTime != nil {
		resp.LastRun = lifecycle.Status.LastRunTime.Format("2006-01-02T15:04:05Z")
	}
	if lifecycle.Status.NextRunTime != nil {
		resp.NextRun = lifecycle.Status.NextRunTime.Format("2006-01-02T15:04:05Z")
	}

	return resp
}
