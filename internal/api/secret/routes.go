package secret

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers secret routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	g.POST("", handler.CreateSecret)
	g.GET("", handler.GetSecrets)
	g.GET("/:id", handler.GetSecret)
	g.PUT("/:id", handler.UpdateSecret)
	g.DELETE("/:id", handler.DeleteSecret)
}


