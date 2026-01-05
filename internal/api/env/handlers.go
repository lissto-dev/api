package env

import (
	"fmt"

	"github.com/labstack/echo/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	controllerconfig "github.com/lissto-dev/controller/pkg/config"
	"go.uber.org/zap"
)

// Handler handles env-related HTTP requests
type Handler struct {
	k8sClient  *k8s.Client
	authorizer *authz.Authorizer
	nsManager  *authz.NamespaceManager
	config     *controllerconfig.Config
}

// FormattableEnv wraps a k8s Env to implement common.Formattable
type FormattableEnv struct {
	k8sObj    *envv1alpha1.Env
	nsManager *authz.NamespaceManager
}

func (f *FormattableEnv) ToDetailed() (common.DetailedResponse, error) {
	return common.NewDetailedResponse(f.k8sObj.ObjectMeta, f.k8sObj.Spec, f.nsManager)
}

func (f *FormattableEnv) ToStandard() interface{} {
	return extractEnvResponse(f.k8sObj, f.nsManager)
}

// extractEnvResponse extracts standard data from env
func extractEnvResponse(env *envv1alpha1.Env, nsManager *authz.NamespaceManager) common.EnvResponse {
	identifier := nsManager.MustGenerateScopedID(env.Namespace, env.Name)
	return common.EnvResponse{
		ID:   identifier,
		Name: env.Name,
	}
}

// NewHandler creates a new env handler
func NewHandler(
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	config *controllerconfig.Config,
) *Handler {
	return &Handler{
		k8sClient:  k8sClient,
		authorizer: authorizer,
		nsManager:  nsManager,
		config:     config,
	}
}

// CreateEnv handles POST /envs
func (h *Handler) CreateEnv(c echo.Context) error {
	var req common.CreateEnvRequest
	user, _ := middleware.GetUserFromContext(c)

	// Bind and validate
	if err := c.Bind(&req); err != nil {
		logging.Logger.Error("Failed to bind request", zap.Error(err))
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		logging.Logger.Error("Request validation failed", zap.Error(err))
		return c.String(400, err.Error())
	}

	// Envs are always scoped to user's namespace
	namespace := h.nsManager.GetDeveloperNamespace(user.Name)

	logging.Logger.Info("Env creation request",
		zap.String("user", user.Name),
		zap.String("role", user.Role.String()),
		zap.String("name", req.Name),
		zap.String("namespace", namespace),
		zap.String("ip", c.RealIP()))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionCreate, authz.ResourceEnv, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "POST /envs", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Check if env already exists
	existing, err := h.k8sClient.GetEnv(c.Request().Context(), namespace, req.Name)
	if err == nil && existing != nil {
		logging.Logger.Error("Env already exists",
			zap.String("name", req.Name),
			zap.String("namespace", namespace))
		return c.String(409, fmt.Sprintf("Env '%s' already exists", req.Name))
	}

	// Ensure namespace exists
	if err := h.k8sClient.EnsureNamespace(c.Request().Context(), namespace); err != nil {
		logging.Logger.Error("Failed to ensure namespace",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create namespace")
	}

	// Create env resource
	env := &envv1alpha1.Env{
		ObjectMeta: metav1.ObjectMeta{
			Name:      req.Name,
			Namespace: namespace,
		},
		Spec: envv1alpha1.EnvSpec{},
	}

	if err := h.k8sClient.CreateEnv(c.Request().Context(), env); err != nil {
		logging.Logger.Error("Failed to create env",
			zap.String("name", req.Name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create env")
	}

	logging.Logger.Info("Env created successfully",
		zap.String("name", req.Name),
		zap.String("namespace", namespace),
		zap.String("user", user.Name))

	// Return scoped identifier
	identifier := h.nsManager.MustGenerateScopedID(namespace, req.Name)
	return c.String(201, identifier)
}

// GetEnvs handles GET /envs
func (h *Handler) GetEnvs(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)

	// Envs are always in user's namespace
	namespace := h.nsManager.GetDeveloperNamespace(user.Name)

	logging.Logger.Info("Env list request",
		zap.String("user", user.Name),
		zap.String("namespace", namespace))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionList, authz.ResourceEnv, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, "GET /envs", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// List envs from user's namespace
	envList, err := h.k8sClient.ListEnvs(c.Request().Context(), namespace)
	if err != nil {
		logging.Logger.Error("Failed to list envs",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to list envs")
	}

	// Convert to response format
	var envs []common.EnvResponse
	for _, env := range envList.Items {
		identifier := h.nsManager.MustGenerateScopedID(env.Namespace, env.Name)
		envs = append(envs, common.EnvResponse{
			ID:   identifier,
			Name: env.Name,
		})
	}

	return c.JSON(200, envs)
}

// GetEnv handles GET /envs/:id
func (h *Handler) GetEnv(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)
	envRef := c.Param("id")

	// Parse env reference - for now, just use env name (it's scoped to user)
	// The env name comes from the URL param
	envName := envRef
	namespace := h.nsManager.GetDeveloperNamespace(user.Name)

	logging.Logger.Info("Env get request",
		zap.String("user", user.Name),
		zap.String("env", envName),
		zap.String("namespace", namespace))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionRead, authz.ResourceEnv, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP(perm.Reason, user.Name, fmt.Sprintf("GET /envs/%s", envName), c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Get env
	env, err := h.k8sClient.GetEnv(c.Request().Context(), namespace, envName)
	if err != nil {
		logging.Logger.Error("Failed to get env",
			zap.String("name", envName),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Environment '%s' not found", envName))
	}

	return common.HandleFormatResponse(c, &FormattableEnv{
		k8sObj:    env,
		nsManager: h.nsManager,
	})
}
