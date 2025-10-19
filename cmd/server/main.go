package main

import (
	"log"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"github.com/lirgo.dev/api/internal/server"
	"github.com/lirgo.dev/api/pkg/config"
)

// CustomValidator wraps the validator
type CustomValidator struct {
	validator *validator.Validate
}

// Validate validates the struct
func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

func main() {
	// Load API keys configuration
	apiKeys, err := config.LoadAPIKeys("api-keys.yaml")
	if err != nil {
		log.Fatalf("Failed to load API keys: %v", err)
	}

	// Create Echo instance
	e := echo.New()

	// Add validator
	e.Validator = &CustomValidator{validator: validator.New()}

	// Add middleware
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())

	// Initialize and start server
	srv := server.New(e, apiKeys)
	if err := srv.Start(); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
