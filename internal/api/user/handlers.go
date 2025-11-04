package user

import (
	"github.com/labstack/echo/v4"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
)

// Handler handles user-related HTTP requests
type Handler struct{}

// NewHandler creates a new user handler
func NewHandler() *Handler {
	return &Handler{}
}

// GetCurrentUser handles GET /user or GET /me
func (h *Handler) GetCurrentUser(c echo.Context) error {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		return c.NoContent(401)
	}

	response := common.UserInfoResponse{
		Name: user.Name,
		Role: user.Role.String(),
	}
	return c.JSON(200, response)
}
