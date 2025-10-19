package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// APIKey represents an API key configuration
type APIKey struct {
	Role   string `yaml:"role"`
	APIKey string `yaml:"api_key"`
	Name   string `yaml:"name,omitempty"`
}

// APIKeysConfig represents the configuration file structure
type APIKeysConfig struct {
	APIKeys []APIKey `yaml:"api_keys"`
}

// LoadAPIKeys loads API keys from a YAML file
func LoadAPIKeys(filename string) ([]APIKey, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		return nil, fmt.Errorf("failed to read API keys file: %w", err)
	}

	var config APIKeysConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		return nil, fmt.Errorf("failed to parse API keys file: %w", err)
	}

	return config.APIKeys, nil
}

// FindAPIKeyByKey finds an API key by its key value
func FindAPIKeyByKey(apiKeys []APIKey, key string) (*APIKey, bool) {
	for _, ak := range apiKeys {
		if ak.APIKey == key {
			return &ak, true
		}
	}
	return nil, false
}
