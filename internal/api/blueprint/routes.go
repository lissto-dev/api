package blueprint

import (
	"github.com/labstack/echo/v4"
	"github.com/lirgo.dev/api/internal/middleware"
	authpkg "github.com/lirgo.dev/api/pkg/auth"
)

// RegisterRoutes registers blueprint routes
func RegisterRoutes(g *echo.Group) {
	handler := NewHandler()

	// Blueprint routes require developer role or higher (developer OR admin)
	g.Use(middleware.RequireRole(authpkg.Developer))
	g.GET("", handler.GetBlueprints)
	g.GET("/:id", handler.GetBlueprint)
	g.POST("", handler.CreateBlueprint)
	g.PUT("/:id", handler.UpdateBlueprint)
	g.DELETE("/:id", handler.DeleteBlueprint)
}
