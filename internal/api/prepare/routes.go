package prepare

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers prepare routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	// All authorization is handled in the handler methods
	g.POST("/prepare", handler.PrepareStack)
}
