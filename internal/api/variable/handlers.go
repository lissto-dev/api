package variable

import (
	"fmt"

	"github.com/labstack/echo/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	"github.com/lissto-dev/api/pkg/metadata"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	"github.com/lissto-dev/controller/pkg/config"
	"go.uber.org/zap"
)

// Handler handles variable-related HTTP requests
type Handler struct {
	k8sClient  *k8s.Client
	authorizer *authz.Authorizer
	nsManager  *authz.NamespaceManager
	config     *config.Config
}

// NewHandler creates a new variable handler
func NewHandler(
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	config *config.Config,
) *Handler {
	return &Handler{
		k8sClient:  k8sClient,
		authorizer: authorizer,
		nsManager:  nsManager,
		config:     config,
	}
}

// CreateVariableRequest represents a request to create a variable config
type CreateVariableRequest struct {
	Name       string            `json:"name" validate:"required"`
	Scope      string            `json:"scope,omitempty"`      // defaults to "env"
	Env        string            `json:"env,omitempty"`        // required for scope=env
	Repository string            `json:"repository,omitempty"` // required for scope=repo
	Data       map[string]string `json:"data" validate:"required"`
}

// UpdateVariableRequest represents a request to update a variable config
type UpdateVariableRequest struct {
	Data map[string]string `json:"data" validate:"required"`
}

// VariableResponse represents a variable config response
type VariableResponse struct {
	ID           string            `json:"id"`
	Name         string            `json:"name"`
	Scope        string            `json:"scope"`
	Env          string            `json:"env,omitempty"`
	Repository   string            `json:"repository,omitempty"`
	Data         map[string]string `json:"data"`
	CreatedAt    string            `json:"created_at,omitempty"`
	KeyUpdatedAt map[string]int64  `json:"key_updated_at,omitempty"` // Unix timestamps per key
}

// FormattableVariable wraps a k8s LisstoVariable to implement common.Formattable
type FormattableVariable struct {
	k8sObj    *envv1alpha1.LisstoVariable
	nsManager *authz.NamespaceManager
}

func (f *FormattableVariable) ToDetailed() (common.DetailedResponse, error) {
	return common.NewDetailedResponse(f.k8sObj.ObjectMeta, f.k8sObj.Spec, f.nsManager)
}

func (f *FormattableVariable) ToStandard() interface{} {
	return extractVariableResponse(f.k8sObj)
}

// extractVariableResponse extracts standard data from variable
func extractVariableResponse(variable *envv1alpha1.LisstoVariable) VariableResponse {
	return VariableResponse{
		ID:           fmt.Sprintf("%s/%s", variable.Namespace, variable.Name),
		Name:         variable.Name,
		Scope:        variable.GetScope(),
		Env:          variable.Spec.Env,
		Repository:   variable.Spec.Repository,
		Data:         variable.Spec.Data,
		CreatedAt:    variable.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
		KeyUpdatedAt: metadata.GetKeyTimestamps(variable),
	}
}

// CreateVariable handles POST /variables
func (h *Handler) CreateVariable(c echo.Context) error {
	var req CreateVariableRequest
	user, _ := middleware.GetUserFromContext(c)

	if err := c.Bind(&req); err != nil {
		logging.Logger.Error("Failed to bind request", zap.Error(err))
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		logging.Logger.Error("Request validation failed", zap.Error(err))
		return c.String(400, err.Error())
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

	logging.Logger.Info("Variable creation request",
		zap.String("user", user.Name),
		zap.String("name", req.Name),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionCreate, authz.ResourceVariable, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "POST /variables", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Ensure namespace exists
	if err := h.k8sClient.EnsureNamespace(c.Request().Context(), namespace); err != nil {
		logging.Logger.Error("Failed to ensure namespace",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create namespace")
	}

	// Check if variable already exists
	existing, err := h.k8sClient.GetLisstoVariable(c.Request().Context(), namespace, req.Name)
	if err == nil && existing != nil {
		logging.Logger.Error("Variable already exists",
			zap.String("name", req.Name),
			zap.String("namespace", namespace))
		return c.String(409, fmt.Sprintf("Variable '%s' already exists", req.Name))
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

	// Create LisstoVariable resource
	variable := &envv1alpha1.LisstoVariable{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespace,
			Labels:    labels,
		},
		Spec: envv1alpha1.LisstoVariableSpec{
			Scope:      scope,
			Env:        req.Env,
			Repository: req.Repository,
			Data:       req.Data,
		},
	}

	// Track key timestamps for all initial keys
	keys := make([]string, 0, len(req.Data))
	for key := range req.Data {
		keys = append(keys, key)
	}
	metadata.UpdateKeyTimestamps(variable, keys)

	if err := h.k8sClient.CreateLisstoVariable(c.Request().Context(), variable); err != nil {
		logging.Logger.Error("Failed to create variable",
			zap.String("name", req.Name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create variable")
	}

	logging.Logger.Info("Variable created successfully",
		zap.String("name", req.Name),
		zap.String("scope", scope),
		zap.String("namespace", namespace),
		zap.String("user", user.Name))

	return c.JSON(201, VariableResponse{
		ID:         fmt.Sprintf("%s/%s", namespace, req.Name),
		Name:       req.Name,
		Scope:      scope,
		Env:        req.Env,
		Repository: req.Repository,
		Data:       req.Data,
	})
}

// GetVariables handles GET /variables
func (h *Handler) GetVariables(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)
	namespace := h.nsManager.GetDeveloperNamespace(user.Name)

	logging.Logger.Info("Variable list request",
		zap.String("user", user.Name),
		zap.String("namespace", namespace))

	// List from user's namespace
	variableList, err := h.k8sClient.ListLisstoVariables(c.Request().Context(), namespace)
	if err != nil {
		logging.Logger.Error("Failed to list variables",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to list variables")
	}

	// Also list global variables if user has access
	globalNS := h.nsManager.GetGlobalNamespace()
	globalList, err := h.k8sClient.ListLisstoVariables(c.Request().Context(), globalNS)
	if err != nil {
		logging.Logger.Warn("Failed to list global variables",
			zap.String("namespace", globalNS),
			zap.Error(err))
		// Continue without global variables
	}

	// Combine and convert to response format
	var variables []VariableResponse
	for _, v := range variableList.Items {
		variables = append(variables, VariableResponse{
			ID:           fmt.Sprintf("%s/%s", v.Namespace, v.Name),
			Name:         v.Name,
			Scope:        v.GetScope(),
			Env:          v.Spec.Env,
			Repository:   v.Spec.Repository,
			Data:         v.Spec.Data,
			CreatedAt:    v.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
			KeyUpdatedAt: metadata.GetKeyTimestamps(&v),
		})
	}
	if globalList != nil {
		for _, v := range globalList.Items {
			variables = append(variables, VariableResponse{
				ID:           fmt.Sprintf("%s/%s", v.Namespace, v.Name),
				Name:         v.Name,
				Scope:        v.GetScope(),
				Env:          v.Spec.Env,
				Repository:   v.Spec.Repository,
				Data:         v.Spec.Data,
				CreatedAt:    v.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
				KeyUpdatedAt: metadata.GetKeyTimestamps(&v),
			})
		}
	}

	return c.JSON(200, variables)
}

// GetVariable handles GET /variables/:id
func (h *Handler) GetVariable(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)
	id := c.Param("id")

	// Get scope from query params to determine namespace (like CreateVariable)
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
	_, name, err := parseVariableID(id, namespace)
	if err != nil {
		return c.String(400, err.Error())
	}

	logging.Logger.Info("Variable get request",
		zap.String("user", user.Name),
		zap.String("id", id),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Check if user can access this namespace
	globalNS := h.nsManager.GetGlobalNamespace()
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	if namespace != userNS && namespace != globalNS {
		return c.String(403, "Cannot access variables in other namespaces")
	}

	variable, err := h.k8sClient.GetLisstoVariable(c.Request().Context(), namespace, name)
	if err != nil {
		logging.Logger.Error("Failed to get variable",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Variable '%s' not found", name))
	}

	return common.HandleFormatResponse(c, &FormattableVariable{
		k8sObj:    variable,
		nsManager: h.nsManager,
	})
}

// UpdateVariable handles PUT /variables/:id
func (h *Handler) UpdateVariable(c echo.Context) error {
	var req UpdateVariableRequest
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
	_, name, err := parseVariableID(id, namespace)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionUpdate, authz.ResourceVariable, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "PUT /variables/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	logging.Logger.Info("Variable update request",
		zap.String("user", user.Name),
		zap.String("id", id),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	// Get existing variable
	variable, err := h.k8sClient.GetLisstoVariable(c.Request().Context(), namespace, name)
	if err != nil {
		logging.Logger.Error("Failed to get variable",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Variable '%s' not found", name))
	}

	// Update data
	variable.Spec.Data = req.Data

	// Track key timestamps for updated keys
	keys := make([]string, 0, len(req.Data))
	for key := range req.Data {
		keys = append(keys, key)
	}
	metadata.UpdateKeyTimestamps(variable, keys)

	if err := h.k8sClient.UpdateLisstoVariable(c.Request().Context(), variable); err != nil {
		logging.Logger.Error("Failed to update variable",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to update variable")
	}

	logging.Logger.Info("Variable updated successfully",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("user", user.Name))

	return c.JSON(200, VariableResponse{
		ID:           fmt.Sprintf("%s/%s", variable.Namespace, variable.Name),
		Name:         variable.Name,
		Scope:        variable.GetScope(),
		Env:          variable.Spec.Env,
		Repository:   variable.Spec.Repository,
		Data:         variable.Spec.Data,
		CreatedAt:    variable.CreationTimestamp.Format("2006-01-02T15:04:05Z07:00"),
		KeyUpdatedAt: metadata.GetKeyTimestamps(variable),
	})
}

// DeleteVariable handles DELETE /variables/:id
func (h *Handler) DeleteVariable(c echo.Context) error {
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
	_, name, err := parseVariableID(id, namespace)
	if err != nil {
		return c.String(400, err.Error())
	}

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionDelete, authz.ResourceVariable, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "DELETE /variables/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	logging.Logger.Info("Variable delete request",
		zap.String("user", user.Name),
		zap.String("id", id),
		zap.String("scope", scope),
		zap.String("namespace", namespace))

	if err := h.k8sClient.DeleteLisstoVariable(c.Request().Context(), namespace, name); err != nil {
		logging.Logger.Error("Failed to delete variable",
			zap.String("name", name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to delete variable")
	}

	logging.Logger.Info("Variable deleted successfully",
		zap.String("name", name),
		zap.String("namespace", namespace),
		zap.String("user", user.Name))

	return c.NoContent(204)
}

// parseVariableID parses a variable ID in format "namespace/name" or just "name"
func parseVariableID(id, defaultNamespace string) (namespace, name string, err error) {
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
