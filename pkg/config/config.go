package config

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os"

	"github.com/lissto.dev/api/pkg/k8s"
	"github.com/lissto.dev/api/pkg/logging"
	"go.uber.org/zap"
	"gopkg.in/yaml.v3"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	SecretDataKey = "api-keys.yaml"
	SecretName    = "lissto-api-keys"
)

// APIKey represents an API key configuration
type APIKey struct {
	Role        string `yaml:"role"`
	APIKey      string `yaml:"api_key"`
	Name        string `yaml:"name,omitempty"`
	SlackUserID string `yaml:"slack_user_id,omitempty"`
}

// APIKeysConfig represents the configuration file structure
type APIKeysConfig struct {
	APIKeys []APIKey `yaml:"api_keys"`
}

// LoadAPIKeys loads API keys from a YAML file
func LoadAPIKeys(filename string) ([]APIKey, error) {
	data, err := os.ReadFile(filename)
	if err != nil {
		logging.Logger.Error("Failed to read API keys file",
			zap.String("filename", filename),
			zap.Error(err))
		return nil, fmt.Errorf("failed to read API keys file: %w", err)
	}

	var config APIKeysConfig
	if err := yaml.Unmarshal(data, &config); err != nil {
		logging.Logger.Error("Failed to parse API keys file",
			zap.String("filename", filename),
			zap.Error(err))
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

// GenerateAPIKey generates a cryptographically secure random API key with role-based prefix
func GenerateAPIKey(role string) (string, error) {
	bytes := make([]byte, 16) // 32 hex characters
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("failed to generate random bytes: %w", err)
	}

	// Use role-based prefix
	prefix := role + "-"
	return prefix + hex.EncodeToString(bytes), nil
}

// LoadAPIKeysFromSecret loads API keys from a Kubernetes secret
func LoadAPIKeysFromSecret(ctx context.Context, k8sClient *k8s.Client, namespace, secretName string) ([]APIKey, error) {
	secret, err := k8sClient.GetSecret(ctx, namespace, secretName)
	if err != nil {
		if errors.IsNotFound(err) {
			return nil, nil // Secret doesn't exist, return empty list
		}
		logging.Logger.Error("Failed to get secret",
			zap.String("namespace", namespace),
			zap.String("secret", secretName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to get secret: %w", err)
	}

	// Get the YAML data from secret
	// Kubernetes automatically decodes base64 when reading secret.Data
	yamlData, exists := secret.Data[SecretDataKey]
	if !exists || len(yamlData) == 0 {
		return nil, nil // Secret exists but has no data
	}

	// Parse YAML (Kubernetes already decoded the base64 for us)
	var config APIKeysConfig
	if err := yaml.Unmarshal(yamlData, &config); err != nil {
		logging.Logger.Error("Failed to parse API keys from secret",
			zap.String("secret", secretName),
			zap.Error(err))
		return nil, fmt.Errorf("failed to parse API keys from secret: %w", err)
	}

	return config.APIKeys, nil
}

// SaveAPIKeysToSecret saves API keys to a Kubernetes secret
func SaveAPIKeysToSecret(ctx context.Context, k8sClient *k8s.Client, namespace, secretName string, apiKeys []APIKey) error {
	// Marshal to YAML
	config := APIKeysConfig{APIKeys: apiKeys}
	yamlData, err := yaml.Marshal(&config)
	if err != nil {
		return fmt.Errorf("failed to marshal API keys to YAML: %w", err)
	}

	// Store as string - Kubernetes will automatically base64 encode StringData
	encodedData := string(yamlData)

	// Check if secret exists
	secret, err := k8sClient.GetSecret(ctx, namespace, secretName)
	if err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to check secret existence: %w", err)
	}

	if errors.IsNotFound(err) || secret == nil {
		// Create new secret
		secret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
				Namespace: namespace,
			},
			StringData: map[string]string{
				SecretDataKey: encodedData,
			},
		}
		if err := k8sClient.CreateSecret(ctx, secret); err != nil {
			logging.Logger.Error("Failed to create secret",
				zap.String("namespace", namespace),
				zap.String("secret", secretName),
				zap.Error(err))
			return fmt.Errorf("failed to create secret: %w", err)
		}
		logging.Logger.Info("Created API keys secret",
			zap.String("namespace", namespace),
			zap.String("secret", secretName))
	} else {
		// Update existing secret
		secret.StringData = map[string]string{
			SecretDataKey: encodedData,
		}
		if err := k8sClient.UpdateSecret(ctx, secret); err != nil {
			logging.Logger.Error("Failed to update secret",
				zap.String("namespace", namespace),
				zap.String("secret", secretName),
				zap.Error(err))
			return fmt.Errorf("failed to update secret: %w", err)
		}
		logging.Logger.Info("Updated API keys secret",
			zap.String("namespace", namespace),
			zap.String("secret", secretName))
	}

	return nil
}

// EnsureAdminKey checks if an admin key exists, and generates one if not
func EnsureAdminKey(apiKeys []APIKey) ([]APIKey, bool, error) {
	// Check if admin key exists
	for _, key := range apiKeys {
		if key.Role == "admin" {
			return apiKeys, false, nil // Admin key exists, no change
		}
	}

	// Generate new admin key
	adminKey, err := GenerateAPIKey("admin")
	if err != nil {
		return nil, false, fmt.Errorf("failed to generate admin key: %w", err)
	}

	newAdminKey := APIKey{
		Role:   "admin",
		APIKey: adminKey,
		Name:   "admin",
	}

	logging.Logger.Info("Generated admin API key",
		zap.String("key_prefix", adminKey[:min(8, len(adminKey))]+"..."),
		zap.String("key", adminKey))

	// Add to list
	apiKeys = append(apiKeys, newAdminKey)
	return apiKeys, true, nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
