package server

import (
	"os"
	"sync"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/lissto-dev/api/internal/api/apikey"
	"github.com/lissto-dev/api/internal/api/blueprint"
	"github.com/lissto-dev/api/internal/api/env"
	"github.com/lissto-dev/api/internal/api/prepare"
	"github.com/lissto-dev/api/internal/api/secret"
	"github.com/lissto-dev/api/internal/api/stack"
	"github.com/lissto-dev/api/internal/api/user"
	"github.com/lissto-dev/api/internal/api/variable"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/cache"
	"github.com/lissto-dev/api/pkg/config"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	operatorConfig "github.com/lissto-dev/controller/pkg/config"
)

// Server represents the API server
type Server struct {
	echo       *echo.Echo
	apiKeys    []config.APIKey
	apiKeysMu  sync.RWMutex
	config     *operatorConfig.Config
	k8sClient  *k8s.Client
	instanceID string
	publicURL  string
}

// GetAPIKeys returns a copy of the current API keys
func (s *Server) GetAPIKeys() []config.APIKey {
	s.apiKeysMu.RLock()
	defer s.apiKeysMu.RUnlock()
	keys := make([]config.APIKey, len(s.apiKeys))
	copy(keys, s.apiKeys)
	return keys
}

// UpdateAPIKeys updates the in-memory API keys list
func (s *Server) UpdateAPIKeys(keys []config.APIKey) error {
	s.apiKeysMu.Lock()
	defer s.apiKeysMu.Unlock()
	s.apiKeys = make([]config.APIKey, len(keys))
	copy(s.apiKeys, keys)
	return nil
}

// New creates a new API server instance
func New(
	e *echo.Echo,
	apiKeys []config.APIKey,
	cfg *operatorConfig.Config,
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	apiNamespace string, // namespace where API is running (for API keys storage)
	instanceID string, // API instance ID for verification
	publicURL string, // Public URL if configured
) *Server {
	// Create server instance
	srv := &Server{
		echo:       e,
		apiKeys:    apiKeys,
		config:     cfg,
		k8sClient:  k8sClient,
		instanceID: instanceID,
		publicURL:  publicURL,
	}

	// Create in-memory cache for prepare results
	memCache := cache.NewMemoryCache()
	logging.Logger.Info("Initialized in-memory cache for prepare results")

	// Create handlers with dependencies
	stackHandler := stack.NewHandler(k8sClient, authorizer, nsManager, cfg, memCache)
	blueprintHandler := blueprint.NewHandler(k8sClient, authorizer, nsManager, cfg)
	envHandler := env.NewHandler(k8sClient, authorizer, nsManager, cfg)
	userHandler := user.NewHandler()
	prepareHandler := prepare.NewHandler(k8sClient, authorizer, nsManager, cfg, memCache)
	variableHandler := variable.NewHandler(k8sClient, authorizer, nsManager, cfg)
	secretHandler := secret.NewHandler(k8sClient, authorizer, nsManager, cfg)

	// Create API key handler with updater function
	// API keys are stored in the same namespace where API is running
	apiKeyUpdater := func(keys []config.APIKey) error {
		return srv.UpdateAPIKeys(keys)
	}
	apiKeyHandler := apikey.NewHandler(k8sClient, cfg, apiKeyUpdater, apiNamespace)

	// API routes with authentication
	// Use function-based middleware to get current keys dynamically
	api := e.Group("/api/v1")
	api.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			// Get current API keys on each request
			currentKeys := srv.GetAPIKeys()
			// Apply API key middleware with current keys
			return middleware.APIKeyMiddleware(currentKeys, authorizer)(next)(c)
		}
	})

	// Register resource routes
	stack.RegisterRoutes(api.Group("/stacks"), stackHandler)
	blueprint.RegisterRoutes(api.Group("/blueprints"), blueprintHandler)
	env.RegisterRoutes(api.Group("/envs"), envHandler)
	user.RegisterRoutes(api.Group("/user"), userHandler)
	prepare.RegisterRoutes(api.Group(""), prepareHandler)
	variable.RegisterRoutes(api.Group("/variables"), variableHandler)
	secret.RegisterRoutes(api.Group("/secrets"), secretHandler)

	// Register internal admin routes (apikey routes register themselves)
	apikey.RegisterRoutes(api, apiKeyHandler)

	// Health check (no auth required)
	// Supports ?info=true to return API information (public URL and API ID)
	e.GET("/health", srv.handleHealth)

	return srv
}

// handleHealth handles the health check endpoint
// Returns 200 OK for normal health checks
// Returns JSON with API info when ?info=true is specified
func (s *Server) handleHealth(c echo.Context) error {
	// Check if info parameter is set
	if c.QueryParam("info") == "true" {
		info := map[string]string{
			"public_url": s.publicURL,
			"api_id":     s.instanceID,
		}
		return c.JSON(200, info)
	}

	// Normal health check - just return 200
	return c.NoContent(200)
}

// Start starts the API server
func (s *Server) Start() error {
	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}
	port = ":" + port
	logging.Logger.Info("Starting server", zap.String("port", port))
	return s.echo.Start(port)
}
