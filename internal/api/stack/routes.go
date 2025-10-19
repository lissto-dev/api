package stack

import (
	"github.com/labstack/echo/v4"
	"github.com/lirgo.dev/api/internal/middleware"
	authpkg "github.com/lirgo.dev/api/pkg/auth"
)

// RegisterRoutes registers stack routes
func RegisterRoutes(g *echo.Group) {
	handler := NewHandler()

	// All stack routes require admin role (highest privilege)
	g.Use(middleware.RequireRole(authpkg.Admin))

	// Stack routes
	g.GET("", handler.GetStacks)
	g.GET("/:id", handler.GetStack)
	g.POST("", handler.CreateStack)
	g.PUT("/:id", handler.UpdateStack)
	g.DELETE("/:id", handler.DeleteStack)
}
