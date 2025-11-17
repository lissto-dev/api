package server

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

const (
	instanceIDKey = "instance-id"
)

// GetOrCreateInstanceID retrieves or creates a unique instance ID for this API
// The ID is stored in the API keys secret to persist across restarts
func GetOrCreateInstanceID(ctx context.Context, k8sClient *k8s.Client, namespace, secretName string) (string, error) {
	// Try to get the secret
	secret, err := k8sClient.GetSecret(ctx, namespace, secretName)
	if err != nil {
		return "", fmt.Errorf("failed to get secret: %w", err)
	}

	// Check if instance ID already exists
	if secret.Data != nil {
		if idBytes, exists := secret.Data[instanceIDKey]; exists && len(idBytes) > 0 {
			instanceID := string(idBytes)
			logging.Logger.Info("Loaded existing API instance ID", zap.String("id", instanceID))
			return instanceID, nil
		}
	}

	// Generate new instance ID
	instanceID := uuid.New().String()
	logging.Logger.Info("Generated new API instance ID", zap.String("id", instanceID))

	// Store in secret
	if secret.Data == nil {
		secret.Data = make(map[string][]byte)
	}
	secret.Data[instanceIDKey] = []byte(instanceID)

	// Update the secret
	err = k8sClient.UpdateSecret(ctx, secret)
	if err != nil {
		return "", fmt.Errorf("failed to save instance ID to secret: %w", err)
	}

	logging.Logger.Info("Saved instance ID to secret",
		zap.String("namespace", namespace),
		zap.String("secret", secretName))

	return instanceID, nil
}
