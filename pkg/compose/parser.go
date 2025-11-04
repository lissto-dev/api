package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
)

// ServiceMetadata contains categorized service information
type ServiceMetadata struct {
	Services []string `json:"services"`
	Infra    []string `json:"infra"`
}

// BlueprintMetadata contains parsed blueprint metadata
type BlueprintMetadata struct {
	Title    string          `json:"title,omitempty"`
	Services ServiceMetadata `json:"services"`
}

// ParseBlueprintMetadata parses docker-compose content and extracts:
// - Title from x-lissto.title extension
// - Service categorization based on build phase and lissto.dev/group label
func ParseBlueprintMetadata(composeContent string, repositoryFallback string) (*BlueprintMetadata, error) {
	// Parse docker-compose
	project, err := loader.LoadWithContext(
		context.Background(),
		types.ConfigDetails{
			ConfigFiles: []types.ConfigFile{
				{
					Filename: "docker-compose.yml",
					Content:  []byte(composeContent),
				},
			},
			WorkingDir: "/tmp",
		},
		loader.WithSkipValidation,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Docker Compose: %w", err)
	}

	// Extract title from x-lissto.title
	title := extractTitle(project, repositoryFallback)

	// Categorize services
	services, infra := categorizeServices(project.Services)

	return &BlueprintMetadata{
		Title: title,
		Services: ServiceMetadata{
			Services: services,
			Infra:    infra,
		},
	}, nil
}

// extractTitle extracts title from x-lissto.title extension field, or falls back to repository
func extractTitle(project *types.Project, repositoryFallback string) string {
	// Check for x-lissto extension
	if project.Extensions != nil {
		if lisstoExt, ok := project.Extensions["x-lissto"]; ok {
			if extMap, ok := lisstoExt.(map[string]interface{}); ok {
				if titleVal, ok := extMap["title"]; ok {
					if titleStr, ok := titleVal.(string); ok && titleStr != "" {
						return titleStr
					}
				}
			}
		}
	}

	// Fallback to repository name
	return repositoryFallback
}

// categorizeServices categorizes services into "services" (with build) and "infra" (without build)
// Respects lissto.dev/group label override
func categorizeServices(services types.Services) (servicesList []string, infraList []string) {
	for name, service := range services {
		// Check for explicit group label override
		group := getGroupFromLabels(service.Labels)

		if group != "" {
			// Label override - map to appropriate category
			groupLower := strings.ToLower(group)
			switch groupLower {
			case "service", "services":
				servicesList = append(servicesList, name)
			case "data", "infra", "infrastructure", "cache":
				infraList = append(infraList, name)
			default:
				// Unknown group, default to services category
				servicesList = append(servicesList, name)
			}
			continue
		}

		// No label override - check for build phase
		if service.Build != nil {
			// Has build phase - categorize as service
			servicesList = append(servicesList, name)
		} else {
			// No build phase - categorize as infra
			infraList = append(infraList, name)
		}
	}

	return servicesList, infraList
}

// getGroupFromLabels extracts lissto.dev/group label value
func getGroupFromLabels(labels types.Labels) string {
	if labels == nil {
		return ""
	}
	return labels["lissto.dev/group"]
}

// ServiceMetadataToJSON converts ServiceMetadata to JSON string
func ServiceMetadataToJSON(metadata ServiceMetadata) (string, error) {
	jsonBytes, err := json.Marshal(metadata)
	if err != nil {
		return "", fmt.Errorf("failed to marshal service metadata: %w", err)
	}
	return string(jsonBytes), nil
}

// ServiceMetadataFromJSON parses JSON string to ServiceMetadata
func ServiceMetadataFromJSON(jsonStr string) (*ServiceMetadata, error) {
	if jsonStr == "" {
		return &ServiceMetadata{
			Services: []string{},
			Infra:    []string{},
		}, nil
	}

	var metadata ServiceMetadata
	if err := json.Unmarshal([]byte(jsonStr), &metadata); err != nil {
		return nil, fmt.Errorf("failed to unmarshal service metadata: %w", err)
	}
	return &metadata, nil
}
