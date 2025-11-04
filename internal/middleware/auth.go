package middleware

import (
	"github.com/labstack/echo/v4"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/config"
	"github.com/lissto-dev/api/pkg/logging"
	"github.com/lissto-dev/api/pkg/response"
	"go.uber.org/zap"
)

// SimpleRequest implements NamespaceRequest interface
type SimpleRequest struct {
	branch string
	author string
}

func (r SimpleRequest) GetBranch() string { return r.branch }
func (r SimpleRequest) GetAuthor() string { return r.author }

// User represents an authenticated user
type User struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Role        authz.Role `json:"role"`
	Email       string     `json:"email"`
	SlackUserID string     `json:"slack_user_id,omitempty"`
}

// APIKeyMiddleware validates API keys and creates user context
func APIKeyMiddleware(apiKeys []config.APIKey, authorizer *authz.Authorizer) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get API key from header
			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey == "" {
				endpoint := c.Request().Method + " " + c.Request().URL.Path
				logging.LogDeniedWithIP("missing_api_key", "", endpoint, c.RealIP())
				return response.Unauthorized(c, "API key required")
			}

			// Validate API key
			keyData, found := config.FindAPIKeyByKey(apiKeys, apiKey)
			if !found {
				endpoint := c.Request().Method + " " + c.Request().URL.Path
				logging.LogDeniedWithIP("invalid_api_key", "", endpoint, c.RealIP())
				return response.Unauthorized(c, "Invalid API key")
			}

			// Set user in context
			user := &User{
				ID:          keyData.Name,
				Name:        keyData.Name,
				Role:        authz.ParseRole(keyData.Role),
				Email:       keyData.Name + "@lissto.dev",
				SlackUserID: keyData.SlackUserID,
			}
			c.Set("user", user)
			c.Set("authorizer", authorizer)

			logging.Logger.Info("User authenticated",
				zap.String("user", user.Name),
				zap.String("role", user.Role.String()),
				zap.String("endpoint", c.Request().Method+" "+c.Request().URL.Path))

			return next(c)
		}
	}
}

// RequirePermission middleware checks specific permissions
func RequirePermission(action authz.Action, resourceType authz.ResourceType) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := c.Get("user")
			if user == nil {
				return response.Unauthorized(c, "User not authenticated")
			}

			authorizer := c.Get("authorizer").(*authz.Authorizer)
			userData := user.(*User)

			// Determine namespace from request parameters
			req := SimpleRequest{
				branch: c.QueryParam("branch"),
				author: c.QueryParam("author"),
			}
			namespace, err := authorizer.DetermineNamespace(
				userData.Role,
				userData.Name,
				req,
			)
			if err != nil {
				return response.BadRequest(c, err.Error())
			}

			// Check permission
			permission := authorizer.CanAccess(
				userData.Role,
				action,
				resourceType,
				namespace,
				userData.Name,
			)

			if !permission.Allowed {
				endpoint := c.Request().Method + " " + c.Request().URL.Path
				logging.LogDeniedWithIP("insufficient_perms", userData.Name, endpoint, c.RealIP())
				return response.Forbidden(c, permission.Reason)
			}

			// Set namespace in context for handlers
			c.Set("namespace", namespace)

			return next(c)
		}
	}
}

// GetUserFromContext extracts user from Echo context
func GetUserFromContext(c echo.Context) (*User, bool) {
	user := c.Get("user")
	if user == nil {
		return nil, false
	}
	return user.(*User), true
}
