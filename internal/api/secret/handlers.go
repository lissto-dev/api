package secret

import (
	"fmt"

	"github.com/labstack/echo/v4"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	"github.com/lissto-dev/api/pkg/metadata"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	operatorConfig "github.com/lissto-dev/controller/pkg/config"
	"go.uber.org/zap"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
)

// Handler handles secret-related HTTP requests
type Handler struct {
	k8sClient  *k8s.Client
	authorizer *authz.Authorizer
	nsManager  *authz.NamespaceManager
	config     *operatorConfig.Config
}

// NewHandler creates a new secret handler
func NewHandler(
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	config *operatorConfig.Config,
) *Handler {
	return &Handler{
		k8sClient:  k8sClient,
		authorizer: authorizer,
		nsManager:  nsManager,
		config:     config,
	}
}

// CreateSecretRequest represents a request to create a secret config
type CreateSecretRequest struct {
	Name       string            `json:"name" validate:"required"`
	Scope      string            `json:"scope,omitempty"`      // defaults to "env"
	Env        string            `json:"env,omitempty"`        // required for scope=env
	Repository string            `json:"repository,omitempty"` // required for scope=repo
	Secrets    map[string]string `json:"secrets,omitempty"`    // key-value pairs to set initially
}

// SetSecretRequest represents a request to set/update secret values
type SetSecretRequest struct {
	Secrets map[string]string `json:"secrets" validate:"required"`
}

// SecretResponse represents a secret config response (write-only - no values)
type SecretResponse struct {
	ID           string           `json:"id"`
	Name         string           `json:"name"`
	Scope        string           `json:"scope"`
	Env          string           `json:"env,omitempty"`
	Repository   string           `json:"repository,omitempty"`
	Keys         []string         `json:"keys"` // Only key names, never values
	CreatedAt    string           `json:"created_at,omitempty"`
	KeyUpdatedAt map[string]int64 `json:"key_updated_at,omitempty"` // Unix timestamps per key
}

// CreateSecret handles POST /secrets
func (h *Handler) CreateSecret(c echo.Context) error {
	var req CreateSecretRequest
	user, _ := middleware.GetUserFromContext(c)

	if err := c.Bind(&req); err != nil {
		logging.Logger.Error("Failed to bind request", zap.Error(err))
		return c.String(400, "Invalid request")
	}
	if req.Name == "" {
		return c.String(400, "name is required")
	}

	// Default scope to "env" if not specified
	scope := req.Scope
	if scope == "" {
		scope = "env"
	}

	// Validate scope-specific requirements
	if scope == "env" && req.Env == "" {
		return c.String(400, "env is required for scope=env")
	}
	if scope == "repo" && req.Repository == "" {
		return c.String(400, "repository is required for scope=repo")
	}

	// Determine namespace based on scope
	namespace, err := h.authorizer.ResolveNamespaceForScope(user.Role, user.Name, scope)
	if err != nil {
		return c.String(400, err.Error())
	}

	logging.Logger.Info("Secret creation request",
		zap.String("user", user.Name),
		zap.String("name", req.Name),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionCreate, authz.ResourceSecret, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "POST /secrets", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Ensure namespace exists
	if err := h.k8sClient.EnsureNamespace(c.Request().Context(), namespace); err != nil {
		logging.Logger.Error("Failed to ensure namespace",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create namespace")
	}

	// Check if secret already exists
	existing, err := h.k8sClient.GetLisstoSecret(c.Request().Context(), namespace, req.Name)
	if err == nil && existing != nil {
		logging.Logger.Error("Secret already exists",
			zap.String("name", req.Name),
			zap.String("namespace", namespace))
		return c.String(409, fmt.Sprintf("Secret '%s' already exists", req.Name))
	}

	// Build labels for discovery
	labels := map[string]string{
		"lissto.dev/scope": scope,
	}
	if scope == "env" {
		labels["lissto.dev/env"] = req.Env
	}
	if scope == "repo" {
		labels["lissto.dev/repository"] = req.Repository
	}

	// Extract key names from request
	keys := make([]string, 0, len(req.Secrets))
	for k := range req.Secrets {
		keys = append(keys, k)
	}

	// Secret ref name
	secretRefName := req.Name + "-data"

	// Create LisstoSecret resource
	lisstoSecret := &envv1alpha1.LisstoSecret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: envv1alpha1.LisstoSecretSpec{
			Scope:      scope,
			Env:        req.Env,
			Repository: req.Repository,
			Keys:       keys,
			SecretRef:  secretRefName,
		},
	}

	// Track key timestamps for all initial keys
	metadata.UpdateKeyTimestamps(lisstoSecret, keys)

	if err := h.k8sClient.CreateLisstoSecret(c.Request().Context(), lisstoSecret); err != nil {
		logging.Logger.Error("Failed to create lissto secret",
			zap.String("name", req.Name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create secret config")
	}

	// Create the actual K8s Secret with the values
	k8sSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      secretRefName,
			Namespace: namespace,
			Labels: map[string]string{
				"lissto.dev/managed-by": "lissto-api",
				"lissto.dev/owner":      req.Name,
			},
		},
		Type:       corev1.SecretTypeOpaque,
		StringData: req.Secrets,
	}

	// Set owner reference so K8s Secret is garbage collected with LisstoSecret
	// This is critical - without it, secrets will be orphaned
	if err := controllerutil.SetControllerReference(lisstoSecret, k8sSecret, h.k8sClient.Scheme()); err != nil {
		logging.Logger.Error("Failed to set owner reference",
			zap.String("name", req.Name),
			zap.Error(err))
		// Clean up the LisstoSecret and fail - orphaned secrets are unacceptable
		_ = h.k8sClient.DeleteLisstoSecret(c.Request().Context(), namespace, req.Name)
		return c.String(500, "Failed to set owner reference")
	}

	if err := h.k8sClient.CreateSecret(c.Request().Context(), k8sSecret); err != nil {
		logging.Logger.Error("Failed to create k8s secret",
			zap.String("name", secretRefName),
			zap.String("namespace", namespace),
			zap.Error(err))
		// Clean up the LisstoSecret
		_ = h.k8sClient.DeleteLisstoSecret(c.Request().Context(), namespace, req.Name)
		return c.String(500, "Failed to create secret")
	}

	logging.Logger.Info("Secret created successfully",
		zap.String("name", req.Name),
		zap.String("scope", scope),
		zap.String("namespace", namespace),
		zap.String("user", user.Name),
		zap.Int("keys", len(keys)))

	return c.JSON(201, SecretResponse{
		ID:         fmt.Sprintf("%s/%s", namespace, req.Name),
		Name:       req.Name,
		Scope:      scope,
		Env:        req.Env,
		Repository: req.Repository,
		Keys:       keys,
	})
}

// GetSecrets handles GET /secrets
func (h *Handler) GetSecrets(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)
	namespace := h.nsManager.GetDeveloperNamespace(user.Name)

	logging.Logger.Info("Secret list request",
		zap.String("user", user.Name),
		zap.String("namespace", namespace))

	// List from user's namespace
	secretList, err := h.k8sClient.ListLisstoSecrets(c.Request().Context(), namespace)
	if err != nil {
		logging.Logger.Error("Failed to list secrets",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to list secrets")
	}

	// Also list global secrets
	globalNS := h.nsManager.GetGlobalNamespace()
	globalList, err := h.k8sClient.ListLisstoSecrets(c.Request().Context(), globalNS)
	if err != nil {
		logging.Logger.Warn("Failed to list global secrets",
			zap.String("namespace", globalNS),
			zap.Error(err))
	}

	// Combine and convert to response format (keys only, no values)
	var secrets []SecretResponse
	for _, s := range secretList.Items {
		secrets = append(secrets, SecretResponse{
			ID:           fmt.Sprintf("%s/%s", s.Namespace, s.Name),
			Name:         s.Name,
			Scope:        s.GetScope(),
			Env:          s.Spec.Env,
			Repository:   s.Spec.Repository,
			Keys:         s.Spec.Keys,
			CreatedAt:    s.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
			KeyUpdatedAt: metadata.GetKeyTimestamps(&s),
		})
	}
	if globalList != nil {
		for _, s := range globalList.Items {
			secrets = append(secrets, SecretResponse{
				ID:           fmt.Sprintf("%s/%s", s.Namespace, s.Name),
				Name:         s.Name,
				Scope:        s.GetScope(),
				Env:          s.Spec.Env,
				Repository:   s.Spec.Repository,
				Keys:         s.Spec.Keys,
				CreatedAt:    s.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
				KeyUpdatedAt: metadata.GetKeyTimestamps(&s),
			})
		}
	}

	return c.JSON(200, secrets)
}

// GetSecret handles GET /secrets/:id
func (h *Handler) GetSecret(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)
	id := c.Param("id")

	// Get scope from query params to determine namespace (like CreateSecret)
	scope := c.QueryParam("scope")
	if scope == "" {
		scope = "env" // default
	}

	// Determine namespace from scope
	namespace, err := h.authorizer.ResolveNamespaceForScope(user.Role, user.Name, scope)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Parse name from ID (support both "name" and "namespace/name")
	_, name, err := parseSecretID(id, namespace)
	if err != nil {
		return c.String(400, err.Error())
	}

	logging.Logger.Info("Secret get request",
		zap.String("user", user.Name),
		zap.String("id", id),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Check if user can access this namespace
	globalNS := h.nsManager.GetGlobalNamespace()
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	if namespace != userNS && namespace != globalNS {
		return c.String(403, "Cannot access secrets in other namespaces")
	}

	lisstoSecret, err := h.k8sClient.GetLisstoSecret(c.Request().Context(), namespace, name)
	if err != nil {
		logging.Logger.Error("Failed to get secret",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Secret '%s' not found", name))
	}

	// Return keys only, no values (write-only)
	return c.JSON(200, SecretResponse{
		ID:           fmt.Sprintf("%s/%s", lisstoSecret.Namespace, lisstoSecret.Name),
		Name:         lisstoSecret.Name,
		Scope:        lisstoSecret.GetScope(),
		Env:          lisstoSecret.Spec.Env,
		Repository:   lisstoSecret.Spec.Repository,
		Keys:         lisstoSecret.Spec.Keys,
		CreatedAt:    lisstoSecret.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
		KeyUpdatedAt: metadata.GetKeyTimestamps(lisstoSecret),
	})
}

// UpdateSecret handles PUT /secrets/:id - sets/updates secret values
func (h *Handler) UpdateSecret(c echo.Context) error {
	var req SetSecretRequest
	user, _ := middleware.GetUserFromContext(c)
	id := c.Param("id")

	if err := c.Bind(&req); err != nil {
		logging.Logger.Error("Failed to bind request", zap.Error(err))
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		logging.Logger.Error("Request validation failed", zap.Error(err))
		return c.String(400, err.Error())
	}

	// Get scope from query params to determine namespace (like GetSecret)
	scope := c.QueryParam("scope")
	if scope == "" {
		scope = "env" // default
	}

	// Determine namespace from scope
	namespace, err := h.authorizer.ResolveNamespaceForScope(user.Role, user.Name, scope)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Parse name from ID
	_, name, err := parseSecretID(id, namespace)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionUpdate, authz.ResourceSecret, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "PUT /secrets/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	logging.Logger.Info("Secret update request",
		zap.String("user", user.Name),
		zap.String("id", id),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Get existing LisstoSecret
	lisstoSecret, err := h.k8sClient.GetLisstoSecret(c.Request().Context(), namespace, name)
	if err != nil {
		logging.Logger.Error("Failed to get lissto secret",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Secret '%s' not found", name))
	}

	// Update LisstoSecret keys list first (metadata before data for better transaction semantics)
	existingKeys := make(map[string]bool)
	for _, k := range lisstoSecret.Spec.Keys {
		existingKeys[k] = true
	}
	oldKeys := make([]string, len(lisstoSecret.Spec.Keys))
	copy(oldKeys, lisstoSecret.Spec.Keys)

	updatedKeys := []string{}
	for k := range req.Secrets {
		updatedKeys = append(updatedKeys, k)
		if !existingKeys[k] {
			lisstoSecret.Spec.Keys = append(lisstoSecret.Spec.Keys, k)
		}
	}

	// Track key timestamps for all updated keys
	metadata.UpdateKeyTimestamps(lisstoSecret, updatedKeys)

	if err := h.k8sClient.UpdateLisstoSecret(c.Request().Context(), lisstoSecret); err != nil {
		logging.Logger.Error("Failed to update lissto secret metadata",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to update secret config")
	}

	// Get or create the K8s Secret
	secretRefName := lisstoSecret.GetSecretRef()
	k8sSecret, err := h.k8sClient.GetSecret(c.Request().Context(), namespace, secretRefName)
	if err != nil {
		// Secret doesn't exist, create it
		k8sSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretRefName,
				Namespace: namespace,
				Labels: map[string]string{
					"lissto.dev/managed-by": "lissto-api",
					"lissto.dev/owner":      name,
				},
			},
			Type:       corev1.SecretTypeOpaque,
			StringData: req.Secrets,
		}
		if err := h.k8sClient.CreateSecret(c.Request().Context(), k8sSecret); err != nil {
			logging.Logger.Error("Failed to create k8s secret",
				zap.String("name", secretRefName),
				zap.String("namespace", namespace),
				zap.Error(err))
			// Rollback LisstoSecret keys
			lisstoSecret.Spec.Keys = oldKeys
			_ = h.k8sClient.UpdateLisstoSecret(c.Request().Context(), lisstoSecret)
			return c.String(500, "Failed to create secret")
		}
	} else {
		// Update existing secret - merge new values
		if k8sSecret.Data == nil {
			k8sSecret.Data = make(map[string][]byte)
		}
		for k, v := range req.Secrets {
			k8sSecret.Data[k] = []byte(v)
		}
		if err := h.k8sClient.UpdateSecret(c.Request().Context(), k8sSecret); err != nil {
			logging.Logger.Error("Failed to update k8s secret",
				zap.String("name", secretRefName),
				zap.String("namespace", namespace),
				zap.Error(err))
			// Rollback LisstoSecret keys
			lisstoSecret.Spec.Keys = oldKeys
			_ = h.k8sClient.UpdateLisstoSecret(c.Request().Context(), lisstoSecret)
			return c.String(500, "Failed to update secret")
		}
	}

	logging.Logger.Info("Secret updated successfully",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("user", user.Name),
		zap.Int("keys", len(lisstoSecret.Spec.Keys)))

	return c.JSON(200, SecretResponse{
		ID:           fmt.Sprintf("%s/%s", lisstoSecret.Namespace, lisstoSecret.Name),
		Name:         lisstoSecret.Name,
		Scope:        lisstoSecret.GetScope(),
		Env:          lisstoSecret.Spec.Env,
		Repository:   lisstoSecret.Spec.Repository,
		Keys:         lisstoSecret.Spec.Keys,
		CreatedAt:    lisstoSecret.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
		KeyUpdatedAt: metadata.GetKeyTimestamps(lisstoSecret),
	})
}

// DeleteSecret handles DELETE /secrets/:id
func (h *Handler) DeleteSecret(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)
	id := c.Param("id")

	// Get scope from query params to determine namespace
	scope := c.QueryParam("scope")
	if scope == "" {
		scope = "env" // default
	}

	// Determine namespace from scope
	namespace, err := h.authorizer.ResolveNamespaceForScope(user.Role, user.Name, scope)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Parse name from ID
	_, name, err := parseSecretID(id, namespace)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionDelete, authz.ResourceSecret, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "DELETE /secrets/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	logging.Logger.Info("Secret delete request",
		zap.String("user", user.Name),
		zap.String("id", id),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Get the LisstoSecret to find the K8s Secret reference
	lisstoSecret, err := h.k8sClient.GetLisstoSecret(c.Request().Context(), namespace, name)
	if err == nil {
		// Delete the K8s Secret first
		secretRefName := lisstoSecret.GetSecretRef()
		if err := h.k8sClient.DeleteSecret(c.Request().Context(), namespace, secretRefName); err != nil {
			logging.Logger.Warn("Failed to delete k8s secret",
				zap.String("name", secretRefName),
				zap.String("namespace", namespace),
				zap.Error(err))
			// Continue to delete LisstoSecret anyway
		}
	}

	// Delete the LisstoSecret
	if err := h.k8sClient.DeleteLisstoSecret(c.Request().Context(), namespace, name); err != nil {
		logging.Logger.Error("Failed to delete lissto secret",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to delete secret")
	}

	logging.Logger.Info("Secret deleted successfully",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("user", user.Name))

	return c.NoContent(204)
}

// parseSecretID parses a secret ID in format "namespace/name" or just "name"
func parseSecretID(id, defaultNamespace string) (namespace, name string, err error) {
	if id == "" {
		return "", "", fmt.Errorf("id cannot be empty")
	}

	for i, ch := range id {
		if ch == '/' {
			ns := id[:i]
			n := id[i+1:]
			if ns == "" || n == "" {
				return "", "", fmt.Errorf("invalid id format: both namespace and name must be non-empty")
			}
			return ns, n, nil
		}
	}
	return defaultNamespace, id, nil
}
