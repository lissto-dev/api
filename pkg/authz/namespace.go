package authz

import (
	"fmt"
	"strings"

	"github.com/lissto.dev/api/pkg/logging"
	"github.com/lissto.dev/controller/pkg/config"
	"go.uber.org/zap"
)

// NamespaceManager handles namespace scoping and access control
type NamespaceManager struct {
	config *config.Config
}

// NewNamespaceManager creates a new namespace manager
func NewNamespaceManager(cfg *config.Config) *NamespaceManager {
	return &NamespaceManager{
		config: cfg,
	}
}

// GetDeveloperNamespace returns the namespace for a developer
func (nm *NamespaceManager) GetDeveloperNamespace(username string) string {
	return nm.config.Namespaces.DeveloperPrefix + username
}

// GetGlobalNamespace returns the global namespace
func (nm *NamespaceManager) GetGlobalNamespace() string {
	return nm.config.Namespaces.Global
}

// IsDeveloperNamespace checks if a namespace is a developer namespace
func (nm *NamespaceManager) IsDeveloperNamespace(namespace string) bool {
	return strings.HasPrefix(namespace, nm.config.Namespaces.DeveloperPrefix)
}

// GetOwnerFromNamespace extracts the owner username from a developer namespace
func (nm *NamespaceManager) GetOwnerFromNamespace(namespace string) (string, error) {
	if !nm.IsDeveloperNamespace(namespace) {
		logging.Logger.Error("Invalid namespace access",
			zap.String("namespace", namespace),
			zap.String("reason", "not_developer_namespace"))
		return "", fmt.Errorf("not a developer namespace: %s", namespace)
	}
	return strings.TrimPrefix(namespace, nm.config.Namespaces.DeveloperPrefix), nil
}

// IsGlobalNamespace checks if a namespace is the global namespace
func (nm *NamespaceManager) IsGlobalNamespace(namespace string) bool {
	return namespace == nm.config.Namespaces.Global
}
