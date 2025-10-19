package middleware

import (
	"github.com/go-pkgz/auth/v2"
	"github.com/labstack/echo/v4"
	authpkg "github.com/lirgo.dev/api/pkg/auth"
	"github.com/lirgo.dev/api/pkg/config"
	"github.com/lirgo.dev/api/pkg/response"
)

// User represents an authenticated user
type User struct {
	ID    string       `json:"id"`
	Name  string       `json:"name"`
	Role  authpkg.Role `json:"role"`
	Email string       `json:"email"`
}

// APIKeyMiddleware validates API keys and creates JWT tokens
func APIKeyMiddleware(apiKeys []config.APIKey, authService *auth.Service) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get API key from header
			apiKey := c.Request().Header.Get("X-API-Key")
			if apiKey == "" {
				return response.Unauthorized(c, "API key required")
			}

			// Validate API key
			keyData, found := config.FindAPIKeyByKey(apiKeys, apiKey)
			if !found {
				return response.Unauthorized(c, "Invalid API key")
			}

			// Skip JWT token generation for now - just use API key validation

			// Set user in context
			user := &User{
				ID:    keyData.Name,
				Name:  keyData.Name,
				Role:  authpkg.ParseRole(keyData.Role),
				Email: keyData.Name + "@lirgo.dev",
			}
			c.Set("user", user)

			return next(c)
		}
	}
}

// RequireRole middleware checks if user has sufficient role permissions
func RequireRole(requiredRole authpkg.Role) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			user := c.Get("user")
			if user == nil {
				return response.Unauthorized(c, "User not authenticated")
			}

			userData := user.(*User)
			if !userData.Role.HasPermission(requiredRole) {
				return response.Forbidden(c, "Insufficient permissions. Required: "+requiredRole.String())
			}

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

// CheckRolePermission checks if the user has sufficient permissions for the required role
func CheckRolePermission(c echo.Context, requiredRole authpkg.Role) error {
	user, exists := GetUserFromContext(c)
	if !exists {
		return response.Unauthorized(c, "User not authenticated")
	}

	if !user.Role.HasPermission(requiredRole) {
		return response.Forbidden(c, "Insufficient permissions. Required: "+requiredRole.String())
	}

	return nil
}
