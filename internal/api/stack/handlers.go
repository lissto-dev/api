package stack

import (
	"context"
	"fmt"
	"strings"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/cache"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/kompose"
	"github.com/lissto-dev/api/pkg/logging"
	"github.com/lissto-dev/api/pkg/postprocessor"
	"github.com/lissto-dev/api/pkg/preprocessor"
	"github.com/lissto-dev/api/pkg/serializer"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	operatorConfig "github.com/lissto-dev/controller/pkg/config"
)

// Handler handles all stack-related HTTP requests
type Handler struct {
	k8sClient          *k8s.Client
	authorizer         *authz.Authorizer
	nsManager          *authz.NamespaceManager
	config             *operatorConfig.Config
	exposePreprocessor *preprocessor.ExposePreprocessor
	cache              cache.Cache
}

// NewHandler creates a new stack handler
func NewHandler(
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	config *operatorConfig.Config,
	cache cache.Cache,
) *Handler {
	// Create expose preprocessor with host suffix and ingress class
	exposePreprocessor := preprocessor.NewExposePreprocessor(
		config.Stacks.Public.HostSuffix,
		config.Stacks.Public.IngressClass,
	)

	return &Handler{
		k8sClient:          k8sClient,
		authorizer:         authorizer,
		nsManager:          nsManager,
		config:             config,
		exposePreprocessor: exposePreprocessor,
		cache:              cache,
	}
}

// CreateStack handles POST /stacks
func (h *Handler) CreateStack(c echo.Context) error {
	var req common.CreateStackRequest
	user, _ := middleware.GetUserFromContext(c)

	// Bind and validate
	if err := c.Bind(&req); err != nil {
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		return c.String(400, err.Error())
	}

	// Log request details
	logging.Logger.Info("Stack creation request",
		zap.String("user", user.Name),
		zap.String("role", user.Role.String()),
		zap.String("blueprint", req.Blueprint),
		zap.String("env", req.Env),
		zap.String("request_id", req.RequestID))

	// Determine target namespace from blueprint (for reading the blueprint)
	blueprintNamespace, _, err := common.ParseBlueprintReference(req.Blueprint)
	if err != nil {
		logging.Logger.Error("Failed to parse blueprint reference",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(400, fmt.Sprintf("Invalid blueprint reference: %v", err))
	}

	// Stack is always created in user's namespace, not blueprint's namespace
	userNamespace := h.nsManager.GetDeveloperNamespace(user.Name)
	namespace := userNamespace

	// Validate env exists (env is always in user's namespace)
	env, err := h.k8sClient.GetEnv(c.Request().Context(), userNamespace, req.Env)
	if err != nil {
		logging.Logger.Error("Failed to get env",
			zap.String("env", req.Env),
			zap.String("namespace", userNamespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Env '%s' not found", req.Env))
	}
	envName := env.Name

	// Retrieve cached prepare result
	var cachedResult cache.PrepareResultCache
	if err := h.cache.Get(c.Request().Context(), req.RequestID, &cachedResult); err != nil {
		logging.Logger.Error("Failed to retrieve cached prepare result",
			zap.String("request_id", req.RequestID),
			zap.Error(err))
		return c.String(400, "Invalid or expired request ID. Please run /prepare again.")
	}

	// Verify namespace ownership
	if cachedResult.Namespace != userNamespace {
		logging.Logger.Warn("Request ID namespace mismatch",
			zap.String("request_id", req.RequestID),
			zap.String("cached_namespace", cachedResult.Namespace),
			zap.String("user_namespace", userNamespace))
		return c.String(404, "Request ID not found")
	}

	// Build enriched images from cache
	enrichedImages := make(map[string]envv1alpha1.ImageInfo)
	for service, info := range cachedResult.Images {
		enrichedImages[service] = envv1alpha1.ImageInfo{
			Digest: info.Digest,
			Image:  info.Image,
			URL:    info.URL,
		}
	}

	logging.Logger.Info("Retrieved cached prepare result",
		zap.String("request_id", req.RequestID),
		zap.Int("services", len(enrichedImages)))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionCreate, authz.ResourceStack, namespace, user.Name)
	if !perm.Allowed {
		logging.LogDeniedWithIP("insufficient_permissions", user.Name, "POST /stacks", c.RealIP())
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Ensure namespace exists
	if err := h.k8sClient.EnsureNamespace(c.Request().Context(), namespace); err != nil {
		logging.Logger.Error("Failed to create namespace",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create namespace")
	}

	// Step 1: Parse blueprint reference and get blueprint
	blueprintNamespace, blueprintName, err := common.ParseBlueprintReference(req.Blueprint)
	if err != nil {
		logging.Logger.Error("Failed to parse blueprint reference",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(400, fmt.Sprintf("Invalid blueprint reference: %v", err))
	}

	blueprint, err := h.k8sClient.GetBlueprint(c.Request().Context(), blueprintNamespace, blueprintName)
	if err != nil {
		logging.Logger.Error("Failed to get blueprint",
			zap.String("blueprint", req.Blueprint),
			zap.String("blueprint_namespace", blueprintNamespace),
			zap.String("blueprint_name", blueprintName),
			zap.Error(err))
		return c.String(404, "Blueprint not found")
	}

	// Parse Docker Compose content
	composeConfig, err := h.parseDockerCompose(blueprint.Spec.DockerCompose)
	if err != nil {
		logging.Logger.Error("Failed to parse Docker Compose",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(400, "Invalid Docker Compose content")
	}

	// Step 2: Validate and apply provided service images
	for serviceName := range composeConfig.Services {
		imageInfo, hasImage := enrichedImages[serviceName]
		if !hasImage {
			return c.String(400, fmt.Sprintf("Missing image for service: %s", serviceName))
		}

		// Validate image contains digest (@sha256:...)
		if !strings.Contains(imageInfo.Digest, "@sha256:") {
			return c.String(400, fmt.Sprintf("Image for service %s must contain digest (@sha256:...), got: %s", serviceName, imageInfo.Digest))
		}

		// Apply provided image to service
		service := composeConfig.Services[serviceName]
		service.Image = imageInfo.Digest
		composeConfig.Services[serviceName] = service

		logging.Logger.Info("Applied image to service",
			zap.String("service", serviceName),
			zap.String("digest", imageInfo.Digest),
			zap.String("image", imageInfo.Image))
	}

	// Step 3: Generate stack name (needed for label injection)
	// Generate timestamp-based name since we don't have commit/tag in request anymore
	stackName := common.GenerateStackName("", "")

	// Step 4: Expose services preprocessing (using env name for URL generation and stack name for labels)
	composeConfig.Services = h.exposePreprocessor.ProcessServices(composeConfig.Services, envName, stackName)

	// Step 5: Generate Kubernetes manifests using Kompose (isolated)
	k8sManifests, err := h.generateKubernetesManifests(composeConfig, namespace, stackName)
	if err != nil {
		logging.Logger.Error("Failed to generate Kubernetes manifests",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(500, "Failed to generate Kubernetes manifests")
	}

	// Step 5.5: Validate manifest size (ConfigMap 1MB limit)
	const maxConfigMapSize = 1 * 1024 * 1024 // 1MB
	if len(k8sManifests) > maxConfigMapSize {
		logging.Logger.Error("Kubernetes manifests exceed ConfigMap size limit",
			zap.Int("size", len(k8sManifests)),
			zap.Int("limit", maxConfigMapSize))
		return c.String(400, "Generated manifests exceed 1MB size limit")
	}

	// Step 6: Create ConfigMap with manifests
	configMapName := fmt.Sprintf("lissto-%s", stackName)

	// Step 1: Create ConfigMap with manifests (no owner reference yet)
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "lissto",
				"lissto.dev/stack":             stackName,
			},
		},
		Data: map[string]string{
			"manifests.yaml": k8sManifests,
		},
	}

	if err := h.k8sClient.CreateConfigMap(c.Request().Context(), configMap); err != nil {
		logging.Logger.Error("Failed to create manifests ConfigMap",
			zap.String("configmap_name", configMapName),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create manifests ConfigMap")
	}

	// Step 2: Create Stack CRD
	// Extract blueprint title
	blueprintTitle := common.ExtractBlueprintTitle(blueprint, blueprint.Name)

	stack := &envv1alpha1.Stack{
		ObjectMeta: metav1.ObjectMeta{
			Name:      stackName,
			Namespace: namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "lissto",
			},
			Annotations: map[string]string{
				"lissto.dev/blueprint-title": blueprintTitle,
			},
		},
		Spec: envv1alpha1.StackSpec{
			BlueprintReference:    req.Blueprint,
			Env:                   envName,
			ManifestsConfigMapRef: configMapName,
			Images:                enrichedImages,
		},
	}

	if err := h.k8sClient.CreateStack(c.Request().Context(), stack); err != nil {
		logging.Logger.Error("Failed to create stack",
			zap.String("stack_name", stackName),
			zap.String("namespace", namespace),
			zap.Error(err))
		// Clean up ConfigMap since Stack creation failed
		if cleanupErr := h.k8sClient.DeleteConfigMap(c.Request().Context(), namespace, configMapName); cleanupErr != nil {
			logging.Logger.Error("Failed to cleanup ConfigMap after Stack creation failure",
				zap.String("configmap_name", configMapName),
				zap.Error(cleanupErr))
		}
		return c.String(500, "Failed to create stack")
	}

	// Step 3: Update ConfigMap with owner reference for automatic cleanup
	if err := controllerutil.SetOwnerReference(stack, configMap, h.k8sClient.Scheme()); err != nil {
		logging.Logger.Error("Failed to set owner reference",
			zap.String("stack_name", stackName),
			zap.String("configmap_name", configMapName),
			zap.Error(err))
		// Clean up both resources since owner reference failed
		if cleanupErr := h.k8sClient.DeleteStack(c.Request().Context(), namespace, stackName); cleanupErr != nil {
			logging.Logger.Error("Failed to cleanup Stack after owner reference failure",
				zap.String("stack_name", stackName),
				zap.Error(cleanupErr))
		}
		if cleanupErr := h.k8sClient.DeleteConfigMap(c.Request().Context(), namespace, configMapName); cleanupErr != nil {
			logging.Logger.Error("Failed to cleanup ConfigMap after owner reference failure",
				zap.String("configmap_name", configMapName),
				zap.Error(cleanupErr))
		}
		return c.String(500, "Failed to set owner reference")
	}

	// Update ConfigMap with owner reference
	if err := h.k8sClient.UpdateConfigMap(c.Request().Context(), configMap); err != nil {
		logging.Logger.Error("Failed to update ConfigMap with owner reference",
			zap.String("configmap_name", configMapName),
			zap.String("namespace", namespace),
			zap.Error(err))
		// Clean up both resources since update failed
		if cleanupErr := h.k8sClient.DeleteStack(c.Request().Context(), namespace, stackName); cleanupErr != nil {
			logging.Logger.Error("Failed to cleanup Stack after ConfigMap update failure",
				zap.String("stack_name", stackName),
				zap.Error(cleanupErr))
		}
		if cleanupErr := h.k8sClient.DeleteConfigMap(c.Request().Context(), namespace, configMapName); cleanupErr != nil {
			logging.Logger.Error("Failed to cleanup ConfigMap after update failure",
				zap.String("configmap_name", configMapName),
				zap.Error(cleanupErr))
		}
		return c.String(500, "Failed to update ConfigMap with owner reference")
	}

	logging.Logger.Info("Stack created successfully",
		zap.String("stack_name", stackName),
		zap.String("namespace", namespace),
		zap.String("user", user.Name))

	// Return scoped identifier
	identifier := common.GenerateScopedIdentifier(namespace, stackName)
	return c.String(201, identifier)
}

// GetStacks handles GET /stacks
func (h *Handler) GetStacks(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces
	allowedNS := h.authorizer.GetAllowedNamespaces(
		user.Role,
		authz.ActionList,
		authz.ResourceStack,
		user.Name,
	)

	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	var allStacks []envv1alpha1.Stack

	// List from allowed namespaces
	if allowedNS[0] == "*" {
		// Admin: list from all namespaces
		stackList, err := h.k8sClient.ListStacks(c.Request().Context(), "")
		if err != nil {
			return c.String(500, "Failed to list stacks")
		}
		allStacks = append(allStacks, stackList.Items...)
	} else {
		// List from each allowed namespace
		for _, ns := range allowedNS {
			stackList, err := h.k8sClient.ListStacks(c.Request().Context(), ns)
			if err != nil {
				continue
			}
			allStacks = append(allStacks, stackList.Items...)
		}
	}

	// Return list of stack objects (JSON marshaller handles serialization)
	return c.JSON(200, allStacks)
}

// GetStack handles GET /stacks/:id
func (h *Handler) GetStack(c echo.Context) error {
	name := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces
	allowedNS := h.authorizer.GetAllowedNamespaces(
		user.Role,
		authz.ActionRead,
		authz.ResourceStack,
		user.Name,
	)

	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Try to find in allowed namespaces
	if allowedNS[0] == "*" {
		// Admin: search in all namespaces (no query param needed)
		// First try global namespace
		stack, err := h.k8sClient.GetStack(c.Request().Context(), h.nsManager.GetGlobalNamespace(), name)
		if err == nil {
			identifier := common.GenerateScopedIdentifier(stack.Namespace, stack.Name)
			return c.String(200, identifier)
		}

		// If not found in global, search all developer namespaces
		// This is a simplified approach - in production you might want to list all namespaces
		return c.String(404, fmt.Sprintf("Stack '%s' not found in any accessible namespace", name))
	}

	// Try each allowed namespace
	for _, ns := range allowedNS {
		stack, err := h.k8sClient.GetStack(c.Request().Context(), ns, name)
		if err == nil {
			identifier := common.GenerateScopedIdentifier(stack.Namespace, stack.Name)
			return c.String(200, identifier)
		}
	}

	return c.String(404, fmt.Sprintf("Stack '%s' not found in your namespace", name))
}

// DeleteStack handles DELETE /stacks/:id
func (h *Handler) DeleteStack(c echo.Context) error {
	name := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces
	allowedNS := h.authorizer.GetAllowedNamespaces(
		user.Role,
		authz.ActionDelete,
		authz.ResourceStack,
		user.Name,
	)

	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Try to find and delete in allowed namespaces
	if allowedNS[0] == "*" {
		// Admin: search in all namespaces (no query param needed)
		// First try global namespace
		if err := h.k8sClient.DeleteStack(c.Request().Context(), h.nsManager.GetGlobalNamespace(), name); err == nil {
			return c.NoContent(204)
		}

		// If not found in global, search all developer namespaces
		// This is a simplified approach - in production you might want to list all namespaces
		return c.String(404, fmt.Sprintf("Stack '%s' not found in any accessible namespace", name))
	}

	// Try each allowed namespace
	for _, ns := range allowedNS {
		if err := h.k8sClient.DeleteStack(c.Request().Context(), ns, name); err == nil {
			return c.NoContent(204)
		}
	}

	return c.String(404, fmt.Sprintf("Stack '%s' not found in your namespace", name))
}

// UpdateStack handles PUT /stacks/:id
func (h *Handler) UpdateStack(c echo.Context) error {
	name := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Parse request body - accept both image info and simple strings
	var req struct {
		Images map[string]interface{} `json:"images"`
	}
	if err := c.Bind(&req); err != nil {
		return c.String(400, "Invalid request body")
	}

	if len(req.Images) == 0 {
		return c.String(400, "No images provided")
	}

	// Get allowed namespaces for update
	allowedNS := h.authorizer.GetAllowedNamespaces(
		user.Role,
		authz.ActionUpdate,
		authz.ResourceStack,
		user.Name,
	)

	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Try to find and update in allowed namespaces
	if allowedNS[0] == "*" {
		// Admin: search in all namespaces
		// First try global namespace
		stack, err := h.k8sClient.GetStack(c.Request().Context(), h.nsManager.GetGlobalNamespace(), name)
		if err == nil {
			return h.updateStackImages(c, stack, req.Images, user.Name)
		}

		// If not found in global, try developer namespace
		userNamespace := h.nsManager.GetDeveloperNamespace(user.Name)
		stack, err = h.k8sClient.GetStack(c.Request().Context(), userNamespace, name)
		if err == nil {
			return h.updateStackImages(c, stack, req.Images, user.Name)
		}

		return c.String(404, fmt.Sprintf("Stack '%s' not found in any accessible namespace", name))
	}

	// Try each allowed namespace
	for _, ns := range allowedNS {
		stack, err := h.k8sClient.GetStack(c.Request().Context(), ns, name)
		if err == nil {
			return h.updateStackImages(c, stack, req.Images, user.Name)
		}
	}

	return c.String(404, fmt.Sprintf("Stack '%s' not found in your namespace", name))
}

// updateStackImages is a helper to update stack images
func (h *Handler) updateStackImages(c echo.Context, stack *envv1alpha1.Stack, images map[string]interface{}, userName string) error {
	// Build updated images map
	updatedImages := make(map[string]envv1alpha1.ImageInfo)
	for service, imageData := range images {
		// Get existing info to preserve URL
		existingInfo := stack.Spec.Images[service]

		var newImage, newDigest string

		// Handle both string (digest only) and object (digest + tag) formats
		switch v := imageData.(type) {
		case string:
			// Legacy format: just digest, preserve existing tag
			newDigest = v
			newImage = existingInfo.Image
		case map[string]interface{}:
			// New format: object with digest and tag
			if digest, ok := v["digest"].(string); ok {
				newDigest = digest
			}
			if image, ok := v["image"].(string); ok && image != "" {
				newImage = image
			} else {
				newImage = existingInfo.Image // Fallback to existing tag
			}
		default:
			// Fallback: preserve existing
			newDigest = existingInfo.Digest
			newImage = existingInfo.Image
		}

		updatedImages[service] = envv1alpha1.ImageInfo{
			Digest: newDigest,
			Image:  newImage,         // Use new tag if provided
			URL:    existingInfo.URL, // Preserve URL
		}
	}

	// Update stack images
	stack.Spec.Images = updatedImages

	// Update in Kubernetes
	if err := h.k8sClient.UpdateStack(c.Request().Context(), stack); err != nil {
		logging.Logger.Error("Failed to update stack",
			zap.String("namespace", stack.Namespace),
			zap.String("name", stack.Name),
			zap.Error(err))
		return c.String(500, "Failed to update stack")
	}

	logging.Logger.Info("Stack updated successfully",
		zap.String("stack_name", stack.Name),
		zap.String("namespace", stack.Namespace),
		zap.String("user", userName),
		zap.Int("updated_services", len(updatedImages)))

	// Return updated stack identifier
	identifier := common.GenerateScopedIdentifier(stack.Namespace, stack.Name)
	return c.JSON(200, map[string]interface{}{
		"data": map[string]string{
			"id": identifier,
		},
	})
}

// parseDockerCompose parses Docker Compose content into a project
func (h *Handler) parseDockerCompose(composeContent string) (*types.Project, error) {
	project, err := loader.LoadWithContext(
		context.Background(),
		types.ConfigDetails{
			ConfigFiles: []types.ConfigFile{
				{
					Filename: "docker-compose.yml",
					Content:  []byte(composeContent),
				},
			},
			WorkingDir: "/tmp",
		},
		loader.WithSkipValidation,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to parse Docker Compose content: %w", err)
	}

	if project.Name == "" {
		project.Name = "stack"
	}

	logging.Logger.Info("Docker Compose parsed successfully",
		zap.Int("services_count", len(project.Services)),
		zap.String("project_name", project.Name))

	return project, nil
}

// generateKubernetesManifests converts Docker Compose project to Kubernetes manifests using Kompose
func (h *Handler) generateKubernetesManifests(project *types.Project, namespace, stackName string) (string, error) {
	// 1. Serialize preprocessed project to compose YAML
	ser := serializer.NewComposeSerializer()
	composeYAML, err := ser.Serialize(project)
	if err != nil {
		return "", fmt.Errorf("failed to serialize Docker Compose: %w", err)
	}

	// 2. Convert with Kompose (pure conversion)
	converter := kompose.NewConverter(namespace)
	objects, err := converter.ConvertToObjects(composeYAML)
	if err != nil {
		return "", fmt.Errorf("kompose conversion failed: %w", err)
	}

	// 3. Post-process: normalize PVC accessModes to ReadWriteOnce
	pvcNormalizer := postprocessor.NewPVCAccessModeNormalizer()
	objects = pvcNormalizer.NormalizeAccessModes(objects)

	// 4. Post-process: inject stack labels to pod templates
	labelInjector := postprocessor.NewStackLabelInjector()
	objects = labelInjector.InjectLabels(objects, stackName)

	// 5. Serialize to YAML
	yamlManifests, err := converter.SerializeToYAML(objects)
	if err != nil {
		return "", fmt.Errorf("YAML serialization failed: %w", err)
	}

	return yamlManifests, nil
}
