package server

import (
	"log"
	"time"

	"github.com/go-pkgz/auth/v2"
	"github.com/go-pkgz/auth/v2/token"
	"github.com/labstack/echo/v4"
	"github.com/lirgo.dev/api/internal/api/blueprint"
	"github.com/lirgo.dev/api/internal/api/stack"
	"github.com/lirgo.dev/api/internal/middleware"
	"github.com/lirgo.dev/api/pkg/config"
	"github.com/lirgo.dev/api/pkg/response"
)

// Server represents the API server
type Server struct {
	echo    *echo.Echo
	apiKeys []config.APIKey
	auth    *auth.Service
}

// New creates a new server instance
func New(e *echo.Echo, apiKeys []config.APIKey) *Server {
	// Initialize go-pkgz/auth service
	authService := auth.NewService(auth.Opts{
		SecretReader: token.SecretFunc(func(id string) (string, error) {
			return "lirgo-secret-key-change-in-production", nil
		}),
		TokenDuration:  time.Hour * 24,
		CookieDuration: time.Hour * 24 * 7,
		Issuer:         "lirgo-api",
		URL:            "http://localhost:8080",
		Validator: token.ValidatorFunc(func(_ string, claims token.Claims) bool {
			// Allow all users with valid claims
			return claims.User != nil && claims.User.ID != ""
		}),
	})

	return &Server{
		echo:    e,
		apiKeys: apiKeys,
		auth:    authService,
	}
}

// Start starts the server
func (s *Server) Start() error {
	s.setupRoutes()

	log.Println("Starting Lirgo API server on :8080")
	log.Println("Sample API Keys:")
	for _, ak := range s.apiKeys {
		log.Printf("%s: %s", ak.Role, ak.APIKey)
	}

	return s.echo.Start(":8080")
}

// setupRoutes configures all routes
func (s *Server) setupRoutes() {
	// Health check endpoint (no auth required)
	s.echo.GET("/health", func(c echo.Context) error {
		return response.OK(c, "Service is healthy", map[string]interface{}{
			"status":  "healthy",
			"service": "lirgo-api",
		})
	})

	// Auth endpoints using go-pkgz/auth
	authRoutes, _ := s.auth.Handlers()
	s.echo.Any("/auth/*", echo.WrapHandler(authRoutes))

	// Protected API v1 routes
	v1 := s.echo.Group("/api/v1")
	v1.Use(middleware.APIKeyMiddleware(s.apiKeys, s.auth))

	// Register resource routes
	stack.RegisterRoutes(v1.Group("/stacks"))
	blueprint.RegisterRoutes(v1.Group("/blueprints"))
}
