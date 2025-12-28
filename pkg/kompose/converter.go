package kompose

import (
	"fmt"
	"os"
	"strings"

	"github.com/kubernetes/kompose/pkg/kobject"
	"github.com/kubernetes/kompose/pkg/loader"
	"github.com/kubernetes/kompose/pkg/transformer/kubernetes"
	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/yaml"

	"github.com/lissto-dev/api/pkg/logging"
)

type Converter struct {
	namespace string
}

func NewConverter(namespace string) *Converter {
	return &Converter{
		namespace: namespace,
	}
}

// ConvertToObjects transforms compose YAML to Kubernetes objects (no serialization)
func (c *Converter) ConvertToObjects(composeYAML string) ([]runtime.Object, error) {
	// 1. Write compose YAML to temp file (Kompose loader expects files)
	tmpFile, err := c.writeTempComposeFile(composeYAML)
	if err != nil {
		return nil, fmt.Errorf("failed to write temp compose file: %w", err)
	}
	defer func() { _ = os.Remove(tmpFile) }()

	// 2. Load using Kompose loader
	komposeLoader, loaderErr := loader.GetLoader("compose")
	if loaderErr != nil {
		return nil, fmt.Errorf("failed to get kompose loader: %w", loaderErr)
	}
	komposeObject, err := komposeLoader.LoadFile([]string{tmpFile}, []string{}, false)
	if err != nil {
		return nil, fmt.Errorf("kompose loader failed: %w", err)
	}

	// 3. Set conversion options (namespace goes here!)
	opt := kobject.ConvertOptions{
		Provider:              "kubernetes",
		CreateChart:           false,
		CreateD:               true, // Create Deployments
		Replicas:              1,
		GenerateJSON:          false,
		GenerateYaml:          true,
		EmptyVols:             false,
		Volumes:               "persistentVolumeClaim", // Use PVCs for volumes
		PushImage:             false,
		WithKomposeAnnotation: false, // Don't add kompose annotations
		Namespace:             c.namespace,
	}

	// 4. Create Kubernetes transformer
	transformer := kubernetes.Kubernetes{Opt: opt}

	// 5. Transform to Kubernetes objects (ISOLATED TRANSFORMATION)
	objects, err := transformer.Transform(komposeObject, opt)
	if err != nil {
		return nil, fmt.Errorf("kompose transformation failed: %w", err)
	}

	logging.Logger.Info("Kompose transformation complete",
		zap.Int("object_count", len(objects)),
		zap.String("namespace", c.namespace))

	return objects, nil
}

// Convert transforms compose YAML string to Kubernetes YAML manifests
// This keeps Kompose completely isolated - it only reads/writes YAML strings
// All configuration (namespace, ingress class) is in the compose YAML labels
func (c *Converter) Convert(composeYAML string) (string, error) {
	// Use new method
	objects, err := c.ConvertToObjects(composeYAML)
	if err != nil {
		return "", err
	}

	// Serialize
	yamlOutput, err := c.SerializeToYAML(objects)
	if err != nil {
		return "", fmt.Errorf("YAML serialization failed: %w", err)
	}

	return yamlOutput, nil
}

// writeTempComposeFile writes compose YAML to a temporary file
// Uses os.CreateTemp which respects TMPDIR environment variable
func (c *Converter) writeTempComposeFile(composeYAML string) (string, error) {
	// Create temp file with pattern that includes namespace for debugging
	// os.CreateTemp uses os.TempDir() which respects TMPDIR env var
	pattern := fmt.Sprintf("compose-%s-*.yaml", strings.ReplaceAll(c.namespace, "/", "-"))
	tmpFile, err := os.CreateTemp("", pattern)
	if err != nil {
		return "", fmt.Errorf("failed to create temp file: %w", err)
	}
	defer func() { _ = tmpFile.Close() }()

	// Write compose content
	if _, err := tmpFile.Write([]byte(composeYAML)); err != nil {
		_ = os.Remove(tmpFile.Name())
		return "", fmt.Errorf("failed to write temp file: %w", err)
	}

	logging.Logger.Debug("Created temporary compose file",
		zap.String("path", tmpFile.Name()),
		zap.String("namespace", c.namespace))

	return tmpFile.Name(), nil
}

// SerializeToYAML converts runtime.Objects to YAML string
func (c *Converter) SerializeToYAML(objects []runtime.Object) (string, error) {
	var yamlDocs []string

	for _, obj := range objects {
		yamlBytes, err := yaml.Marshal(obj)
		if err != nil {
			return "", fmt.Errorf("failed to marshal object: %w", err)
		}
		yamlDocs = append(yamlDocs, string(yamlBytes))
	}

	// Join with YAML document separator
	return strings.Join(yamlDocs, "---\n"), nil
}
