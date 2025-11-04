package preprocessor

import (
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// ExposePreprocessor handles conversion of lissto.dev/expose labels to Kompose labels
type ExposePreprocessor struct {
	hostSuffix   string
	ingressClass string
}

// NewExposePreprocessor creates a new expose preprocessor
func NewExposePreprocessor(hostSuffix, ingressClass string) *ExposePreprocessor {
	return &ExposePreprocessor{
		hostSuffix:   hostSuffix,
		ingressClass: ingressClass,
	}
}

// ProcessServices converts lissto.dev/expose labels to Kompose labels for ingress generation
// Also injects lissto.dev/stack label to all resources via deploy.labels
func (ep *ExposePreprocessor) ProcessServices(services types.Services, envName, stackName string) types.Services {
	processed := make(types.Services)

	for name, service := range services {
		// Always start by stripping any pre-existing Kompose expose labels
		baseLabels := ep.removeKomposeExposeLabels(service.Labels)

		// Create new service
		newService := service

		// Inject stack label via deploy.labels (becomes K8s labels, not annotations)
		newService = ep.injectStackLabelToDeploy(newService, stackName)

		// Check if service has expose labels
		if ep.shouldExposeService(service) {
			// Generate hostname
			hostname := ep.generateHostname(name, envName)

			// Convert expose labels to Kompose labels
			komposeLabels := ep.convertToKomposeLabels(baseLabels, hostname)

			// Update labels
			newService.Labels = komposeLabels

			logging.Logger.Info("Service marked for exposure",
				zap.String("service", name),
				zap.String("hostname", hostname),
				zap.String("ingress-class", ep.ingressClass),
				zap.String("stack", stackName))

			processed[name] = newService
		} else {
			// Keep service without any kompose labels if no expose labels
			newService.Labels = baseLabels
			processed[name] = newService
		}
	}

	return processed
}

// shouldExposeService determines if a service should be exposed based on labels
func (ep *ExposePreprocessor) shouldExposeService(service types.ServiceConfig) bool {
	if service.Labels == nil {
		return false
	}

	// Check for lissto.dev/expose label
	exposeValue, exists := service.Labels["lissto.dev/expose"]
	return exists && (exposeValue == "true" || exposeValue != "")
}

// generateHostname creates a hostname for the exposed service using env name
func (ep *ExposePreprocessor) generateHostname(serviceName, envName string) string {
	// Format: {serviceName}-{envName}{hostSuffix}
	return fmt.Sprintf("%s-%s%s", serviceName, envName, ep.hostSuffix)
}

// GetExposedServiceURL returns the expected URL for an exposed service, or empty string if not exposed
// Requires envName to be provided to generate the URL
func (ep *ExposePreprocessor) GetExposedServiceURL(service types.ServiceConfig, serviceName, envName string) string {
	if !ep.shouldExposeService(service) {
		return ""
	}
	if envName == "" {
		return "" // Cannot generate URL without env name
	}
	return ep.generateHostname(serviceName, envName)
}

// convertToKomposeLabels converts lissto.dev/expose labels to Kompose-compatible labels
func (ep *ExposePreprocessor) convertToKomposeLabels(labels map[string]string, hostname string) map[string]string {
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
	komposeLabels["kompose.service.expose.ingress-class-name"] = ep.ingressClass

	// // Set TLS configuration
	// if tlsValue := ep.getLabelValue(labels, "lissto.dev/expose.tls", "true"); tlsValue == "false" {
	// 	komposeLabels["kompose.service.expose.tls"] = "false"
	// }

	return komposeLabels
}

// removeKomposeExposeLabels returns a copy of labels without kompose service expose labels
// Only removes: kompose.service.expose and kompose.service.expose.ingress-class-name
func (ep *ExposePreprocessor) removeKomposeExposeLabels(labels map[string]string) map[string]string {
	cleaned := make(map[string]string)
	if labels == nil {
		return cleaned
	}
	for key, value := range labels {
		// Only remove the two specific ingress-related labels
		if key == "kompose.service.expose" || key == "kompose.service.expose.ingress-class-name" {
			// Skip these labels
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
