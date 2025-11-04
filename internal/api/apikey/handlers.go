package apikey

import (
	"github.com/labstack/echo/v4"
	"github.com/lissto.dev/api/internal/middleware"
	"github.com/lissto.dev/api/pkg/authz"
	"github.com/lissto.dev/api/pkg/config"
	"github.com/lissto.dev/api/pkg/k8s"
	"github.com/lissto.dev/api/pkg/logging"
	"github.com/lissto.dev/api/pkg/response"
	operatorConfig "github.com/lissto.dev/controller/pkg/config"
	"go.uber.org/zap"
)

// Handler handles API key management requests
type Handler struct {
	k8sClient      *k8s.Client
	config         *operatorConfig.Config
	apiKeysUpdater func([]config.APIKey) error
}

// NewHandler creates a new API key handler
func NewHandler(
	k8sClient *k8s.Client,
	cfg *operatorConfig.Config,
	apiKeysUpdater func([]config.APIKey) error,
) *Handler {
	return &Handler{
		k8sClient:      k8sClient,
		config:         cfg,
		apiKeysUpdater: apiKeysUpdater,
	}
}

// CreateAPIKeyRequest represents the request to create a new API key
type CreateAPIKeyRequest struct {
	Name        string `json:"name" validate:"required"`
	Role        string `json:"role" validate:"required"`
	SlackUserID string `json:"slack_user_id,omitempty"`
}

// CreateAPIKeyResponse represents the response after creating an API key
type CreateAPIKeyResponse struct {
	APIKey string `json:"api_key"`
	Name   string `json:"name"`
	Role   string `json:"role"`
}

// CreateAPIKey handles POST /_internal/api-keys
func (h *Handler) CreateAPIKey(c echo.Context) error {
	user, ok := middleware.GetUserFromContext(c)
	if !ok {
		return response.Unauthorized(c, "User not authenticated")
	}

	// Require admin role
	if user.Role != authz.Admin {
		logging.Logger.Warn("Non-admin user attempted to create API key",
			zap.String("user", user.Name),
			zap.String("role", user.Role.String()))
		return response.Forbidden(c, "Admin role required")
	}

	var req CreateAPIKeyRequest
	if err := c.Bind(&req); err != nil {
		logging.Logger.Error("Failed to bind request", zap.Error(err))
		return response.BadRequest(c, "Invalid request")
	}

	if err := c.Validate(&req); err != nil {
		logging.Logger.Error("Request validation failed", zap.Error(err))
		return response.BadRequest(c, err.Error())
	}

	// Validate role
	role := authz.ParseRole(req.Role)
	if role == authz.User && req.Role != "user" {
		// ParseRole defaults to User for unknown roles, check explicitly
		return response.BadRequest(c, "Invalid role. Must be one of: admin, deploy, user")
	}

	// Generate API key with role-based prefix
	apiKeyValue, err := config.GenerateAPIKey(req.Role)
	if err != nil {
		logging.Logger.Error("Failed to generate API key", zap.Error(err))
		return response.InternalServerError(c, "Failed to generate API key")
	}

	// Create new API key
	newAPIKey := config.APIKey{
		Role:        req.Role,
		APIKey:      apiKeyValue,
		Name:        req.Name,
		SlackUserID: req.SlackUserID,
	}

	// Load current keys from secret
	ctx := c.Request().Context()
	currentKeys, err := config.LoadAPIKeysFromSecret(ctx, h.k8sClient, h.config.Namespaces.Global, config.SecretName)
	if err != nil {
		logging.Logger.Error("Failed to load API keys from secret", zap.Error(err))
		return response.InternalServerError(c, "Failed to load API keys")
	}

	// Check if key with same name already exists
	for _, key := range currentKeys {
		if key.Name == req.Name {
			return response.BadRequest(c, "API key with this name already exists")
		}
	}

	// Add new key
	updatedKeys := append(currentKeys, newAPIKey)

	// Save to secret
	if err := config.SaveAPIKeysToSecret(ctx, h.k8sClient, h.config.Namespaces.Global, config.SecretName, updatedKeys); err != nil {
		logging.Logger.Error("Failed to save API key to secret", zap.Error(err))
		return response.InternalServerError(c, "Failed to save API key")
	}

	// Update in-memory list
	if h.apiKeysUpdater != nil {
		if err := h.apiKeysUpdater(updatedKeys); err != nil {
			logging.Logger.Warn("Failed to update in-memory API keys", zap.Error(err))
			// Don't fail the request, key is saved to secret
		}
	}

	logging.Logger.Info("API key created",
		zap.String("name", req.Name),
		zap.String("role", req.Role),
		zap.String("created_by", user.Name),
		zap.String("key_prefix", apiKeyValue[:min(8, len(apiKeyValue))]+"..."))

	// Return the new API key (only on creation)
	return response.Created(c, "API key created", CreateAPIKeyResponse{
		APIKey: apiKeyValue,
		Name:   req.Name,
		Role:   req.Role,
	})
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
