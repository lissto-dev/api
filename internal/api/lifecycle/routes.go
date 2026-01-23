package lifecycle

import (
	"github.com/labstack/echo/v4"
)

// RegisterRoutes registers lifecycle-related routes
func RegisterRoutes(g *echo.Group, h *Handler, authMiddleware echo.MiddlewareFunc) {
	g.GET("/lifecycles", h.GetLifecycles, authMiddleware)
	g.GET("/lifecycles/:id", h.GetLifecycle, authMiddleware)
	g.POST("/lifecycles", h.CreateLifecycle, authMiddleware)
	g.PUT("/lifecycles/:id", h.UpdateLifecycle, authMiddleware)
	g.DELETE("/lifecycles/:id", h.DeleteLifecycle, authMiddleware)
}
