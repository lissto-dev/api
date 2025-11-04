package authz

import (
	"fmt"

	"github.com/lissto-dev/api/pkg/logging"
)

// NamespaceRequest interface for requests with namespace fields
type NamespaceRequest interface {
	GetBranch() string
	GetAuthor() string
}

// DetermineNamespace calculates target namespace based on role and request
func (a *Authorizer) DetermineNamespace(
	role Role,
	username string,
	req NamespaceRequest,
) (string, error) {
	branch := req.GetBranch()
	author := req.GetAuthor()

	// Verify author matches authenticated user (only if author is provided)
	if role == User && author != "" && author != username {
		logging.LogDenied("author_mismatch", username, "POST /stacks")
		return "", fmt.Errorf("author '%s' does not match authenticated user '%s'", author, username)
	}

	if role == Deploy && branch == "" {
		return "", fmt.Errorf("branch required for deploy role")
	}

	// Check if branch is global
	if a.nsManager.config.IsGlobalBranch(branch) {
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
