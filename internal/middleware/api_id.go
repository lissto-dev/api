package middleware

import (
	"github.com/labstack/echo/v4"
)

// APIIDMiddleware adds the X-Lissto-API-ID header to all responses
// This allows CLI clients to verify they're talking to the correct API instance
func APIIDMiddleware(instanceID string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Lissto-API-ID", instanceID)
			return next(c)
		}
	}
}

