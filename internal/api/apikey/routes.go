package apikey

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers API key management routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	// Internal admin endpoint for creating API keys
	// Note: Authentication is already applied via the group middleware
	// Handler will check for admin role
	g.POST("/_internal/api-keys", handler.CreateAPIKey)
}
