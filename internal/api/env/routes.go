package env

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers env routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	// All authorization is handled in the handler methods
	g.POST("", handler.CreateEnv)
	g.GET("", handler.GetEnvs)
	g.GET("/:id", handler.GetEnv)
}
