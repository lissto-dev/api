package preprocessor

import (
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// VisibilityType represents the ingress visibility level
type VisibilityType string

const (
	VisibilityInternal VisibilityType = "internal"
	VisibilityInternet VisibilityType = "internet"
)

// IngressConfig holds configuration for a specific ingress visibility type
type IngressConfig struct {
	IngressClass string
	HostSuffix   string
	TLSSecret    string
}

// ExposePreprocessor handles conversion of lissto.dev/expose labels to Kompose labels
type ExposePreprocessor struct {
	internalConfig *IngressConfig // nil if not configured
	internetConfig *IngressConfig // nil if not configured
	defaultType    VisibilityType // which type to use for "true" value
}

// NewExposePreprocessor creates a new expose preprocessor
// Either internalConfig or internetConfig can be nil, but not both
func NewExposePreprocessor(internalConfig, internetConfig *IngressConfig) *ExposePreprocessor {
	// Determine default type based on what's configured
	defaultType := VisibilityInternal
	if internalConfig == nil && internetConfig != nil {
		defaultType = VisibilityInternet
	}

	return &ExposePreprocessor{
		internalConfig: internalConfig,
		internetConfig: internetConfig,
		defaultType:    defaultType,
	}
}

// ExposureError represents an error during service exposure processing
type ExposureError struct {
	ServiceName   string
	RequestedType VisibilityType
	Message       string
}

func (e *ExposureError) Error() string {
	return fmt.Sprintf("service '%s': %s", e.ServiceName, e.Message)
}

func NewMissingConfigError(serviceName string, visType VisibilityType) *ExposureError {
	available := "internet"
	if visType == VisibilityInternet {
		available = "internal"
	}
	return &ExposureError{
		ServiceName:   serviceName,
		RequestedType: visType,
		Message: fmt.Sprintf("requested '%s' visibility but only '%s' is configured",
			visType, available),
	}
}

// getVisibilityType extracts the visibility type from labels
func (ep *ExposePreprocessor) getVisibilityType(service types.ServiceConfig) VisibilityType {
	if service.Labels == nil {
		return ep.defaultType
	}

	exposeValue, exists := service.Labels["lissto.dev/expose"]
	if !exists {
		return ep.defaultType
	}

	switch exposeValue {
	case "internet":
		return VisibilityInternet
	case "internal":
		return VisibilityInternal
	case "true", "":
		return ep.defaultType
	default:
		return ep.defaultType
	}
}

// getConfigForVisibility returns the appropriate config based on visibility type
func (ep *ExposePreprocessor) getConfigForVisibility(visType VisibilityType) *IngressConfig {
	if visType == VisibilityInternet {
		return ep.internetConfig
	}
	return ep.internalConfig
}

// isVisibilityConfigured checks if a visibility type has configuration
func (ep *ExposePreprocessor) isVisibilityConfigured(visType VisibilityType) bool {
	return ep.getConfigForVisibility(visType) != nil
}

// ProcessServices converts lissto.dev/expose labels to Kompose labels for ingress generation
// Returns an error if a service requests a visibility type that is not configured
func (ep *ExposePreprocessor) ProcessServices(services types.Services, envName, stackName string) (types.Services, error) {
	processed := make(types.Services)

	for name, service := range services {
		baseLabels := ep.removeKomposeExposeLabels(service.Labels)
		newService := service
		newService = ep.injectStackLabelToDeploy(newService, stackName)

		if ep.shouldExposeService(service) {
			// Determine visibility type
			visType := ep.getVisibilityType(service)

			// Validate that this visibility type is configured
			if !ep.isVisibilityConfigured(visType) {
				return nil, NewMissingConfigError(name, visType)
			}

			config := ep.getConfigForVisibility(visType)
			hostname := ep.generateHostnameWithConfig(name, envName, *config)
			komposeLabels := ep.convertToKomposeLabels(baseLabels, hostname, *config)
			newService.Labels = komposeLabels

			logging.Logger.Info("Service marked for exposure",
				zap.String("service", name),
				zap.String("hostname", hostname),
				zap.String("visibility", string(visType)),
				zap.String("ingress-class", config.IngressClass),
				zap.String("tls-secret", config.TLSSecret),
				zap.String("stack", stackName))

			processed[name] = newService
		} else {
			newService.Labels = baseLabels
			processed[name] = newService
		}
	}

	return processed, nil
}

// shouldExposeService determines if a service should be exposed based on labels
func (ep *ExposePreprocessor) shouldExposeService(service types.ServiceConfig) bool {
	if service.Labels == nil {
		return false
	}

	exposeValue, exists := service.Labels["lissto.dev/expose"]
	if !exists {
		return false
	}

	// Accept: "true", "internal", "internet", or any non-empty value
	return exposeValue == "true" || exposeValue == "internal" || exposeValue == "internet" || exposeValue != ""
}

// generateHostnameWithConfig creates a hostname using the provided config
func (ep *ExposePreprocessor) generateHostnameWithConfig(serviceName, envName string, config IngressConfig) string {
	return fmt.Sprintf("%s-%s%s", serviceName, envName, config.HostSuffix)
}

// GetExposedServiceURL returns the expected URL for an exposed service
// Returns empty string if service is not exposed or visibility type is not configured
func (ep *ExposePreprocessor) GetExposedServiceURL(service types.ServiceConfig, serviceName, envName string) string {
	if !ep.shouldExposeService(service) || envName == "" {
		return ""
	}
	visType := ep.getVisibilityType(service)
	config := ep.getConfigForVisibility(visType)
	if config == nil {
		return ""
	}
	return ep.generateHostnameWithConfig(serviceName, envName, *config)
}

// convertToKomposeLabels converts lissto.dev/expose labels to Kompose-compatible labels
func (ep *ExposePreprocessor) convertToKomposeLabels(labels map[string]string, hostname string, config IngressConfig) map[string]string {
	komposeLabels := make(map[string]string)

	// Copy non-expose labels
	for key, value := range labels {
		if !strings.HasPrefix(key, "lissto.dev/expose") {
			komposeLabels[key] = value
		}
	}

	// Ensure no pre-existing kompose expose labels remain
	komposeLabels = ep.removeKomposeExposeLabels(komposeLabels)

	// Set Kompose expose label
	komposeLabels["kompose.service.expose"] = hostname

	// Set ingress class
	komposeLabels["kompose.service.expose.ingress-class-name"] = config.IngressClass

	// Set TLS secret (always present due to validation)
	komposeLabels["kompose.service.expose.tls-secret"] = config.TLSSecret

	return komposeLabels
}

// removeKomposeExposeLabels returns a copy of labels without kompose service expose labels
func (ep *ExposePreprocessor) removeKomposeExposeLabels(labels map[string]string) map[string]string {
	cleaned := make(map[string]string)
	if labels == nil {
		return cleaned
	}
	for key, value := range labels {
		if key == "kompose.service.expose" ||
			key == "kompose.service.expose.ingress-class-name" ||
			key == "kompose.service.expose.tls-secret" {
			continue
		}
		cleaned[key] = value
	}
	return cleaned
}

// injectStackLabelToDeploy adds the lissto.dev/stack label to service's deploy.labels
// Using deploy.labels ensures it becomes a Kubernetes label (not annotation) on Deployments/Pods
func (ep *ExposePreprocessor) injectStackLabelToDeploy(service types.ServiceConfig, stackName string) types.ServiceConfig {
	// Initialize Deploy if nil
	if service.Deploy == nil {
		service.Deploy = &types.DeployConfig{}
	}

	// Initialize Deploy.Labels if nil
	if service.Deploy.Labels == nil {
		service.Deploy.Labels = make(map[string]string)
	}

	// Add stack label to deploy.labels (kompose converts this to K8s metadata.labels)
	service.Deploy.Labels["lissto.dev/stack"] = stackName

	return service
}
