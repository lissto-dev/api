package authz

import (
	"strings"
)

// Action represents an operation on a resource
type Action string

const (
	ActionCreate Action = "create"
	ActionRead   Action = "read"
	ActionUpdate Action = "update"
	ActionDelete Action = "delete"
	ActionList   Action = "list"
)

// ResourceType represents the type of resource
type ResourceType string

const (
	ResourceBlueprint ResourceType = "blueprint"
	ResourceStack     ResourceType = "stack"
	ResourceEnv       ResourceType = "env"
	ResourceVariable  ResourceType = "variable"
	ResourceSecret    ResourceType = "secret"
)

// Permission represents a permission check result
type Permission struct {
	Allowed bool
	Reason  string
}

// Authorizer handles authorization decisions
type Authorizer struct {
	nsManager *NamespaceManager
}

// NewAuthorizer creates a new authorizer
func NewAuthorizer(nsManager *NamespaceManager) *Authorizer {
	return &Authorizer{
		nsManager: nsManager,
	}
}

// CanAccess checks if a user can perform an action on a resource
func (a *Authorizer) CanAccess(role Role, action Action, resourceType ResourceType, namespace, username string) Permission {
	// Admin can only list, read (get), and delete across any namespace
	// Admin cannot create or update blueprints, stacks, or envs. it's a role for managing the platform, not for managing resources.
	// Exception: Admin can create/update Variables and Secrets in global namespace (for global configs)
	if role == Admin {
		// Allow create/update for Variables and Secrets in global namespace
		if (resourceType == ResourceVariable || resourceType == ResourceSecret) &&
			(action == ActionCreate || action == ActionUpdate) &&
			a.nsManager.IsGlobalNamespace(namespace) {
			return Permission{
				Allowed: true,
				Reason:  "admin can create/update global variables and secrets",
			}
		}

		// Block create and update actions for other resources
		if action == ActionCreate || action == ActionUpdate {
			return Permission{
				Allowed: false,
				Reason:  "admin cannot create or update resources",
			}
		}

		// Allow list, read, and delete across any namespace
		if action == ActionList || action == ActionRead || action == ActionDelete {
			return Permission{
				Allowed: true,
				Reason:  "admin can list, read, and delete resources",
			}
		}

		return Permission{
			Allowed: false,
			Reason:  "action not permitted for admin role",
		}
	}

	// Deploy role - can only create blueprints
	if role == Deploy {
		if resourceType == ResourceBlueprint && action == ActionCreate {
			return Permission{
				Allowed: true,
				Reason:  "deploy can create blueprints",
			}
		}
		if action == ActionRead || action == ActionList {
			return Permission{
				Allowed: true,
				Reason:  "deploy can read/list for verification",
			}
		}
		return Permission{
			Allowed: false,
			Reason:  "deploy can only create blueprints and read/list",
		}
	}

	// User role
	if role == User {
		// Users can CRUD their own namespaced resources
		if a.isOwnNamespace(namespace, username) {
			// For stack/env creation, ensure no global scoped resources
			if (resourceType == ResourceStack || resourceType == ResourceEnv) && action == ActionCreate && a.nsManager.IsGlobalNamespace(namespace) {
				return Permission{
					Allowed: false,
					Reason:  "no global scoped stacks/envs allowed",
				}
			}
			return Permission{
				Allowed: true,
				Reason:  "user owns this namespace",
			}
		}

		// Users can read/list global blueprints only
		if a.nsManager.IsGlobalNamespace(namespace) && resourceType == ResourceBlueprint {
			if action == ActionRead || action == ActionList {
				return Permission{
					Allowed: true,
					Reason:  "user can read global blueprints",
				}
			}
		}

		return Permission{
			Allowed: false,
			Reason:  "insufficient permissions",
		}
	}

	return Permission{
		Allowed: false,
		Reason:  "unknown role",
	}
}

// isOwnNamespace checks if the namespace belongs to the user
func (a *Authorizer) isOwnNamespace(namespace, username string) bool {
	return a.nsManager.IsDeveloperNamespace(namespace) &&
		strings.HasSuffix(namespace, username)
}

// GetAllowedNamespaces returns all namespaces a user can access for a given action
func (a *Authorizer) GetAllowedNamespaces(role Role, action Action, resourceType ResourceType, username string) []string {
	var namespaces []string

	// Admin can only list, read, and delete across any namespace
	if role == Admin {
		// Block create and update actions
		if action == ActionCreate || action == ActionUpdate {
			return []string{} // No namespaces allowed
		}

		// Allow list, read, and delete across all namespaces
		if action == ActionList || action == ActionRead || action == ActionDelete {
			return []string{"*"} // Wildcard means all namespaces
		}

		return []string{} // No namespaces for other actions
	}

	// Deploy role
	if role == Deploy {
		if action == ActionCreate || action == ActionRead || action == ActionList {
			return []string{"*"}
		}
		return []string{}
	}

	// User role
	if role == User {
		// Can read from global
		if action == ActionRead || action == ActionList {
			namespaces = append(namespaces, a.nsManager.GetGlobalNamespace())
		}

		// Can do everything in own namespace
		namespaces = append(namespaces, a.nsManager.GetDeveloperNamespace(username))
	}

	return namespaces
}
