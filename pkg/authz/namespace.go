package authz

import (
	"github.com/lissto-dev/api/pkg/logging"
	controllerconfig "github.com/lissto-dev/controller/pkg/config"
	"github.com/lissto-dev/controller/pkg/namespace"
	"go.uber.org/zap"
)

// NamespaceManager handles namespace scoping and access control.
// It embeds the controller's namespace.Manager for core operations
// and adds API-specific functionality.
type NamespaceManager struct {
	*namespace.Manager
	config *controllerconfig.Config
}

// NewNamespaceManager creates a new namespace manager
func NewNamespaceManager(cfg *controllerconfig.Config) *NamespaceManager {
	return &NamespaceManager{
		Manager: cfg.Namespaces.NewManager(),
		config:  cfg,
	}
}

// GetOwnerFromNamespace extracts the owner username from a developer namespace.
// This wraps the embedded Manager method to add logging.
func (nm *NamespaceManager) GetOwnerFromNamespace(ns string) (string, error) {
	owner, err := nm.Manager.GetOwnerFromNamespace(ns)
	if err != nil {
		logging.Logger.Error("Invalid namespace access",
			zap.String("namespace", ns),
			zap.String("reason", "not_developer_namespace"))
	}
	return owner, err
}

// IsGlobalBranch checks if a branch is configured as global for a specific repository
func (nm *NamespaceManager) IsGlobalBranch(repository, branch string) bool {
	return nm.config.IsGlobalBranch(repository, branch)
}
