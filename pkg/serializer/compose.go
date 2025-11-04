package serializer

import (
	"fmt"

	"github.com/compose-spec/compose-go/v2/types"
	"gopkg.in/yaml.v3"
)

type ComposeSerializer struct{}

func NewComposeSerializer() *ComposeSerializer {
	return &ComposeSerializer{}
}

// Serialize converts types.Project to Docker Compose YAML string
func (cs *ComposeSerializer) Serialize(project *types.Project) (string, error) {
	// Marshal to YAML
	yamlBytes, err := yaml.Marshal(project)
	if err != nil {
		return "", fmt.Errorf("failed to serialize project to YAML: %w", err)
	}

	return string(yamlBytes), nil
}
