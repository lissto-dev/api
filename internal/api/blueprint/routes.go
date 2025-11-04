package blueprint

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers blueprint routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	// All authorization is handled in the handler methods
	g.GET("", handler.GetBlueprints)
	g.GET("/:id", handler.GetBlueprint)
	g.POST("", handler.CreateBlueprint)
	g.DELETE("/:id", handler.DeleteBlueprint)
}
