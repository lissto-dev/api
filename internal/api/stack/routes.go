package stack

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers stack routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	// All authorization is handled in the handler methods
	g.GET("", handler.GetStacks)
	g.GET("/:id", handler.GetStack)
	g.POST("", handler.CreateStack)
	g.DELETE("/:id", handler.DeleteStack)
}
