package image

import (
	"context"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/authn/k8schain"
	"github.com/lissto-dev/api/pkg/logging"
	"go.uber.org/zap"
)

// GetK8sKeychain returns a keychain that uses Kubernetes authentication
// This automatically discovers:
// - Image pull secrets from the pod's service account
// - Node IAM credentials (ECR on AWS, Workload Identity on GCP)
// - Docker config files (~/.docker/config.json)
// - Credential helpers (docker-credential-ecr-login, etc.)
func GetK8sKeychain(ctx context.Context) (authn.Keychain, error) {
	logging.Logger.Info("Initializing Kubernetes authentication keychain for container registries")

	keychain, err := k8schain.NewInCluster(ctx, k8schain.Options{
		// Options can be empty - k8schain will automatically discover:
		// - The pod's namespace from the service account
		// - Image pull secrets attached to the service account
		// - Node credentials via the cloud provider's credential helper
	})

	if err != nil {
		logging.Logger.Warn("Failed to initialize K8s keychain, falling back to anonymous access",
			zap.Error(err))
		return nil, err
	}

	logging.Logger.Info("Successfully initialized K8s authentication keychain",
		zap.String("auth_method", "k8schain"))

	return keychain, nil
}
