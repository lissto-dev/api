package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/lissto-dev/controller/pkg/config"
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
	Volumes  []string        `json:"volumes,omitempty"`
	Networks []string        `json:"networks,omitempty"`
}

// LisstoConfig contains x-lissto extension configuration
type LisstoConfig struct {
	Registry         string `json:"registry,omitempty"`
	Repository       string `json:"repository,omitempty"`       // Single repository for all services
	RepositoryPrefix string `json:"repositoryPrefix,omitempty"` // Prefix + service name
}

// ParseBlueprintMetadata parses docker-compose content and extracts:
// - Title with priority: x-lissto.title → repo.Name → repo.URL
// - Service categorization based on build phase and lissto.dev/group label
func ParseBlueprintMetadata(composeContent string, repoConfig config.RepoConfig) (*BlueprintMetadata, error) {
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

	// Extract title with priority: x-lissto.title → repo.Name → repo.URL
	title := extractTitle(project, repoConfig)

	// Categorize services
	services, infra := categorizeServices(project.Services)

	// Extract volumes
	volumes := extractVolumeNames(project.Volumes)

	// Extract networks
	networks := extractNetworkNames(project.Networks)

	return &BlueprintMetadata{
		Title: title,
		Services: ServiceMetadata{
			Services: services,
			Infra:    infra,
		},
		Volumes:  volumes,
		Networks: networks,
	}, nil
}

// extractTitle extracts title with priority:
// 1. x-lissto.title (explicit in docker-compose)
// 2. repo.Name (configured name from repos config)
// 3. repo.URL (normalized repository URL)
func extractTitle(project *types.Project, repoConfig config.RepoConfig) string {
	// Priority 1: Check for explicit x-lissto.title
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

	// Priority 2: Use configured repository name if available
	if repoConfig.Name != "" {
		return repoConfig.Name
	}

	// Priority 3: Fall back to normalized repository URL
	return config.NormalizeRepositoryURL(repoConfig.URL)
}

// ExtractLisstoConfig extracts x-lissto extension configuration from a project
func ExtractLisstoConfig(project *types.Project) *LisstoConfig {
	config := &LisstoConfig{}

	if project.Extensions == nil {
		return config
	}

	lisstoExt, ok := project.Extensions["x-lissto"]
	if !ok {
		return config
	}

	extMap, ok := lisstoExt.(map[string]interface{})
	if !ok {
		return config
	}

	// Extract registry
	if registryVal, ok := extMap["registry"]; ok {
		if registryStr, ok := registryVal.(string); ok && registryStr != "" {
			config.Registry = registryStr
		}
	}

	// Extract repository (single image for all services)
	if repoVal, ok := extMap["repository"]; ok {
		if repoStr, ok := repoVal.(string); ok && repoStr != "" {
			config.Repository = repoStr
		}
	}

	// Extract repositoryPrefix (prefix + service name)
	if prefixVal, ok := extMap["repositoryPrefix"]; ok {
		if prefixStr, ok := prefixVal.(string); ok && prefixStr != "" {
			config.RepositoryPrefix = prefixStr
		}
	}

	return config
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

// extractVolumeNames extracts volume names from the project
func extractVolumeNames(volumes types.Volumes) []string {
	var volumeNames []string
	for name := range volumes {
		volumeNames = append(volumeNames, name)
	}
	return volumeNames
}

// extractNetworkNames extracts network names from the project
func extractNetworkNames(networks types.Networks) []string {
	var networkNames []string
	for name := range networks {
		networkNames = append(networkNames, name)
	}
	return networkNames
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
