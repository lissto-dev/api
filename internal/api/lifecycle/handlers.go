package lifecycle

import (
	"fmt"
	"time"

	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
)

// Handler handles all lifecycle-related HTTP requests
type Handler struct {
	k8sClient  *k8s.Client
	authorizer *authz.Authorizer
}

// NewHandler creates a new lifecycle handler
func NewHandler(k8sClient *k8s.Client, authorizer *authz.Authorizer) *Handler {
	return &Handler{
		k8sClient:  k8sClient,
		authorizer: authorizer,
	}
}

// CreateLifecycle handles POST /lifecycles
func (h *Handler) CreateLifecycle(c echo.Context) error {
	var req common.CreateLifecycleRequest
	user, _ := middleware.GetUserFromContext(c)

	// Bind and validate
	if err := c.Bind(&req); err != nil {
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		return c.String(400, err.Error())
	}

	logging.Logger.Info("Lifecycle creation request",
		zap.String("user", user.Name),
		zap.String("role", user.Role.String()),
		zap.String("name", req.Name),
		zap.String("targetKind", req.TargetKind))

	// Check authorization - admin only for cluster-scoped Lifecycle
	perm := h.authorizer.CanAccess(user.Role, authz.ActionCreate, authz.ResourceLifecycle, "", user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP("insufficient_permissions", user.Name, "POST /lifecycles", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Parse interval
	interval, err := time.ParseDuration(req.Interval)
	if err != nil {
		return c.String(400, fmt.Sprintf("Invalid interval format: %v", err))
	}

	// Build label selector
	var labelSelector *metav1.LabelSelector
	if len(req.LabelSelector) > 0 {
		labelSelector = &metav1.LabelSelector{
			MatchLabels: req.LabelSelector,
		}
	}

	// Convert tasks from request to CRD format
	tasks := make([]envv1alpha1.LifecycleTask, 0, len(req.Tasks))
	for _, taskReq := range req.Tasks {
		task := envv1alpha1.LifecycleTask{
			Name: taskReq.Name,
		}

		if taskReq.Delete != nil {
			olderThan, err := time.ParseDuration(taskReq.Delete.OlderThan)
			if err != nil {
				return c.String(400, fmt.Sprintf("Invalid olderThan duration: %v", err))
			}
			task.Delete = &envv1alpha1.DeleteTask{
				OlderThan: metav1.Duration{Duration: olderThan},
			}
		}

		if taskReq.ScaleDown != nil {
			task.ScaleDown = &envv1alpha1.ScaleDownTask{}
			if taskReq.ScaleDown.Timeout != "" {
				timeout, err := time.ParseDuration(taskReq.ScaleDown.Timeout)
				if err != nil {
					return c.String(400, fmt.Sprintf("Invalid timeout duration: %v", err))
				}
				task.ScaleDown.Timeout = metav1.Duration{Duration: timeout}
			}
		}

		if taskReq.ScaleUp != nil {
			task.ScaleUp = &envv1alpha1.ScaleUpTask{}
		}

		if taskReq.Snapshot != nil {
			task.Snapshot = &envv1alpha1.SnapshotTask{}
		}

		tasks = append(tasks, task)
	}

	// Create Lifecycle CRD
	lifecycle := &envv1alpha1.Lifecycle{
		ObjectMeta: metav1.ObjectMeta{
			Name: req.Name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "lissto",
			},
		},
		Spec: envv1alpha1.LifecycleSpec{
			TargetKind:    req.TargetKind,
			LabelSelector: labelSelector,
			Interval:      metav1.Duration{Duration: interval},
			Tasks:         tasks,
		},
	}

	if err := h.k8sClient.CreateLifecycle(c.Request().Context(), lifecycle); err != nil {
		logging.Logger.Error("Failed to create lifecycle",
			zap.String("name", req.Name),
			zap.Error(err))
		return c.String(500, "Failed to create lifecycle")
	}

	logging.Logger.Info("Lifecycle created successfully",
		zap.String("name", req.Name),
		zap.String("user", user.Name))

	return c.String(201, req.Name)
}

// GetLifecycles handles GET /lifecycles
func (h *Handler) GetLifecycles(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)

	// Check authorization - admin only
	perm := h.authorizer.CanAccess(user.Role, authz.ActionList, authz.ResourceLifecycle, "", user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP("insufficient_permissions", user.Name, "GET /lifecycles", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	lifecycleList, err := h.k8sClient.ListLifecycles(c.Request().Context())
	if err != nil {
		logging.Logger.Error("Failed to list lifecycles", zap.Error(err))
		return c.String(500, "Failed to list lifecycles")
	}

	return c.JSON(200, lifecycleList.Items)
}

// GetLifecycle handles GET /lifecycles/:id
func (h *Handler) GetLifecycle(c echo.Context) error {
	name := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Check authorization - admin only
	perm := h.authorizer.CanAccess(user.Role, authz.ActionRead, authz.ResourceLifecycle, "", user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP("insufficient_permissions", user.Name, "GET /lifecycles/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	lifecycle, err := h.k8sClient.GetLifecycle(c.Request().Context(), name)
	if err != nil {
		logging.Logger.Error("Failed to get lifecycle",
			zap.String("name", name),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Lifecycle '%s' not found", name))
	}

	return common.HandleFormatResponse(c, &common.FormattableLifecycle{k8sObj: lifecycle})
}

// UpdateLifecycle handles PUT /lifecycles/:id
func (h *Handler) UpdateLifecycle(c echo.Context) error {
	name := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	var req common.UpdateLifecycleRequest
	if err := c.Bind(&req); err != nil {
		return c.String(400, "Invalid request")
	}

	// Check authorization - admin only
	perm := h.authorizer.CanAccess(user.Role, authz.ActionUpdate, authz.ResourceLifecycle, "", user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP("insufficient_permissions", user.Name, "PUT /lifecycles/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Get existing lifecycle
	lifecycle, err := h.k8sClient.GetLifecycle(c.Request().Context(), name)
	if err != nil {
		logging.Logger.Error("Failed to get lifecycle",
			zap.String("name", name),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Lifecycle '%s' not found", name))
	}

	// Update fields if provided
	if req.TargetKind != "" {
		lifecycle.Spec.TargetKind = req.TargetKind
	}
	if req.LabelSelector != nil {
		lifecycle.Spec.LabelSelector = &metav1.LabelSelector{
			MatchLabels: req.LabelSelector,
		}
	}
	if req.Interval != "" {
		interval, err := time.ParseDuration(req.Interval)
		if err != nil {
			return c.String(400, fmt.Sprintf("Invalid interval format: %v", err))
		}
		lifecycle.Spec.Interval = metav1.Duration{Duration: interval}
	}
	if len(req.Tasks) > 0 {
		// Convert tasks from request to CRD format (same logic as create)
		tasks := make([]envv1alpha1.LifecycleTask, 0, len(req.Tasks))
		for _, taskReq := range req.Tasks {
			task := envv1alpha1.LifecycleTask{
				Name: taskReq.Name,
			}

			if taskReq.Delete != nil {
				olderThan, err := time.ParseDuration(taskReq.Delete.OlderThan)
				if err != nil {
					return c.String(400, fmt.Sprintf("Invalid olderThan duration: %v", err))
				}
				task.Delete = &envv1alpha1.DeleteTask{
					OlderThan: metav1.Duration{Duration: olderThan},
				}
			}

			if taskReq.ScaleDown != nil {
				task.ScaleDown = &envv1alpha1.ScaleDownTask{}
				if taskReq.ScaleDown.Timeout != "" {
					timeout, err := time.ParseDuration(taskReq.ScaleDown.Timeout)
					if err != nil {
						return c.String(400, fmt.Sprintf("Invalid timeout duration: %v", err))
					}
					task.ScaleDown.Timeout = metav1.Duration{Duration: timeout}
				}
			}

			if taskReq.ScaleUp != nil {
				task.ScaleUp = &envv1alpha1.ScaleUpTask{}
			}

			if taskReq.Snapshot != nil {
				task.Snapshot = &envv1alpha1.SnapshotTask{}
			}

			tasks = append(tasks, task)
		}
		lifecycle.Spec.Tasks = tasks
	}

	// Update lifecycle
	if err := h.k8sClient.UpdateLifecycle(c.Request().Context(), lifecycle); err != nil {
		logging.Logger.Error("Failed to update lifecycle",
			zap.String("name", name),
			zap.Error(err))
		return c.String(500, "Failed to update lifecycle")
	}

	logging.Logger.Info("Lifecycle updated successfully",
		zap.String("name", name),
		zap.String("user", user.Name))

	return c.JSON(200, map[string]interface{}{
		"data": map[string]string{
			"id": name,
		},
	})
}

// DeleteLifecycle handles DELETE /lifecycles/:id
func (h *Handler) DeleteLifecycle(c echo.Context) error {
	name := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Check authorization - admin only
	perm := h.authorizer.CanAccess(user.Role, authz.ActionDelete, authz.ResourceLifecycle, "", user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP("insufficient_permissions", user.Name, "DELETE /lifecycles/:id", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	if err := h.k8sClient.DeleteLifecycle(c.Request().Context(), name); err != nil {
		logging.Logger.Error("Failed to delete lifecycle",
			zap.String("name", name),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Lifecycle '%s' not found", name))
	}

	logging.Logger.Info("Lifecycle deleted successfully",
		zap.String("name", name),
		zap.String("user", user.Name))

	return c.NoContent(204)
}
