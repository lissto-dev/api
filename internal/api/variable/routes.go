package variable

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers variable routes
func RegisterRoutes(g *echo.Group, handler *Handler) {
	g.POST("", handler.CreateVariable)
	g.GET("", handler.GetVariables)
	g.GET("/:id", handler.GetVariable)
	g.PUT("/:id", handler.UpdateVariable)
	g.DELETE("/:id", handler.DeleteVariable)
}


