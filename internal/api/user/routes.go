package user

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers user routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	g.GET("", handler.GetCurrentUser)
	g.GET("/me", handler.GetCurrentUser)
}
