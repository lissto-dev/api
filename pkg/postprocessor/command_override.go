package postprocessor

import (
	"encoding/json"
	"strings"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// CommandOverrider overrides container commands based on lissto.dev/command and lissto.dev/entrypoint labels
// Primary use case: preserve Kubernetes env var syntax $(VAR) that Kompose strips out
type CommandOverrider struct{}

// NewCommandOverrider creates a new command overrider
func NewCommandOverrider() *CommandOverrider {
	return &CommandOverrider{}
}

// OverrideCommands applies command overrides from service labels to Kubernetes objects
// serviceLabelMap maps service name to its labels from docker-compose
func (c *CommandOverrider) OverrideCommands(objects []runtime.Object, serviceLabelMap map[string]map[string]string) []runtime.Object {
	if len(serviceLabelMap) == 0 {
		return objects
	}

	for i, obj := range objects {
		switch resource := obj.(type) {
		case *appsv1.Deployment:
			// Match by deployment name (equals service name in Kompose)
			serviceName := resource.Name
			if labels, exists := serviceLabelMap[serviceName]; exists {
				c.overrideContainerCommands(resource.Spec.Template.Spec.Containers, labels, serviceName)
			}
			objects[i] = resource

		case *appsv1.StatefulSet:
			// Match by statefulset name (equals service name in Kompose)
			serviceName := resource.Name
			if labels, exists := serviceLabelMap[serviceName]; exists {
				c.overrideContainerCommands(resource.Spec.Template.Spec.Containers, labels, serviceName)
			}
			objects[i] = resource

		case *corev1.Pod:
			// Match by pod name or io.kompose.service label
			serviceName := resource.Name
			if komposeService, ok := resource.Labels["io.kompose.service"]; ok {
				serviceName = komposeService
			}
			if labels, exists := serviceLabelMap[serviceName]; exists {
				c.overrideContainerCommands(resource.Spec.Containers, labels, serviceName)
			}
			objects[i] = resource
		}
	}

	return objects
}

// overrideContainerCommands applies command/entrypoint overrides to containers
func (c *CommandOverrider) overrideContainerCommands(containers []corev1.Container, labels map[string]string, serviceName string) {
	// Check for lissto.dev/entrypoint label (becomes K8s command)
	if entrypointLabel, exists := labels["lissto.dev/entrypoint"]; exists && entrypointLabel != "" {
		entrypoint, err := c.parseCommandLabel(entrypointLabel)
		if err != nil {
			logging.Logger.Warn("Failed to parse lissto.dev/entrypoint label",
				zap.String("service", serviceName),
				zap.String("label_value", entrypointLabel),
				zap.Error(err))
		} else if len(entrypoint) > 0 {
			// Apply to all containers in the pod
			for i := range containers {
				containers[i].Command = entrypoint
				logging.Logger.Info("Overriding container command (entrypoint)",
					zap.String("service", serviceName),
					zap.String("container", containers[i].Name),
					zap.Strings("command", entrypoint))
			}
		}
	}

	// Check for lissto.dev/command label (becomes K8s args)
	if commandLabel, exists := labels["lissto.dev/command"]; exists && commandLabel != "" {
		command, err := c.parseCommandLabel(commandLabel)
		if err != nil {
			logging.Logger.Warn("Failed to parse lissto.dev/command label",
				zap.String("service", serviceName),
				zap.String("label_value", commandLabel),
				zap.Error(err))
		} else if len(command) > 0 {
			// Apply to all containers in the pod
			for i := range containers {
				containers[i].Args = command
				logging.Logger.Info("Overriding container args (command)",
					zap.String("service", serviceName),
					zap.String("container", containers[i].Name),
					zap.Strings("args", command))
			}
		}
	}
}

// parseCommandLabel parses a command label value supporting both formats:
// 1. JSON array: '["sh", "-c", "echo $(VAR)"]'
// 2. Space-separated: "sh -c echo $(VAR)"
// The function preserves Kubernetes env var syntax $(VAR) as-is
func (c *CommandOverrider) parseCommandLabel(labelValue string) ([]string, error) {
	if labelValue == "" {
		return nil, nil
	}

	// Try JSON array first
	var command []string
	err := json.Unmarshal([]byte(labelValue), &command)
	if err == nil {
		return command, nil
	}

	// Fall back to space-separated string
	// Use strings.Fields to split by whitespace (handles multiple spaces)
	return strings.Fields(labelValue), nil
}
