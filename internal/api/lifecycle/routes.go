package lifecycle

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers lifecycle-related routes
func RegisterRoutes(g *echo.Group, h *Handler) {
	// All authorization is handled in the handler methods
	g.GET("/lifecycles", h.GetLifecycles)
	g.GET("/lifecycles/:id", h.GetLifecycle)
	g.POST("/lifecycles", h.CreateLifecycle)
	g.PUT("/lifecycles/:id", h.UpdateLifecycle)
	g.DELETE("/lifecycles/:id", h.DeleteLifecycle)
}
