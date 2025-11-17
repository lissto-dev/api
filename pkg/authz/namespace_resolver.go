package authz

import (
	"fmt"

	"github.com/lissto-dev/api/pkg/logging"
)

// NamespaceRequest interface for requests with namespace fields (generic)
type NamespaceRequest interface {
	GetBranch() string
	GetAuthor() string
}

// BlueprintRequest interface for blueprint creation requests (includes repository)
type BlueprintRequest interface {
	GetBranch() string
	GetAuthor() string
	GetRepository() string
}

// DetermineNamespaceForBlueprint calculates target namespace for blueprint creation
// Repository is required for validation
func (a *Authorizer) DetermineNamespaceForBlueprint(
	role Role,
	username string,
	req BlueprintRequest,
) (string, error) {
	branch := req.GetBranch()
	author := req.GetAuthor()
	repository := req.GetRepository()

	// Repository is required for all roles
	if repository == "" {
		return "", fmt.Errorf("repository is required")
	}

	// Verify author matches authenticated user (only if author is provided)
	if role == User && author != "" && author != username {
		logging.LogDenied("author_mismatch", username, "POST /blueprints")
		return "", fmt.Errorf("author '%s' does not match authenticated user '%s'", author, username)
	}

	if role == Deploy && branch == "" {
		return "", fmt.Errorf("branch required for deploy role")
	}

	// Check if branch is global for the specific repository
	if a.nsManager.config.IsGlobalBranch(repository, branch) {
		// Only admin and deploy roles can use global namespace
		if role == Admin || role == Deploy {
			return a.nsManager.GetGlobalNamespace(), nil
		}
	}

	// For deploy role, use author for namespace determination
	if role == Deploy {
		if author == "" {
			return "", fmt.Errorf("author required for deploy role")
		}
		return a.nsManager.GetDeveloperNamespace(author), nil
	}

	// For other roles, use authenticated username
	return a.nsManager.GetDeveloperNamespace(username), nil
}

// DetermineNamespace calculates target namespace for non-blueprint requests (stacks, etc.)
// Does not require repository field
func (a *Authorizer) DetermineNamespace(
	role Role,
	username string,
	req NamespaceRequest,
) (string, error) {
	author := req.GetAuthor()

	// Verify author matches authenticated user (only if author is provided)
	if role == User && author != "" && author != username {
		logging.LogDenied("author_mismatch", username, "POST /stacks")
		return "", fmt.Errorf("author '%s' does not match authenticated user '%s'", author, username)
	}

	// For deploy role, use author for namespace determination
	if role == Deploy {
		if author == "" {
			return "", fmt.Errorf("author required for deploy role")
		}
		return a.nsManager.GetDeveloperNamespace(author), nil
	}

	// For other roles, use authenticated username
	return a.nsManager.GetDeveloperNamespace(username), nil
}
