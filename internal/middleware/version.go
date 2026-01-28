package middleware

import (
	"github.com/labstack/echo/v4"
)

// VersionMiddleware adds the X-Lissto-API-Version header to all responses
// This allows CLI clients to check server version for backward compatibility
func VersionMiddleware(version string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			c.Response().Header().Set("X-Lissto-API-Version", version)
			return next(c)
		}
	}
}
