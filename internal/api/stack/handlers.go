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
	controllerconfig "github.com/lissto-dev/controller/pkg/config"
	"github.com/lissto-dev/controller/pkg/namespace"
)

// Handler handles all stack-related HTTP requests
type Handler struct {
	k8sClient          *k8s.Client
	authorizer         *authz.Authorizer
	nsManager          *authz.NamespaceManager
	config             *controllerconfig.Config
	exposePreprocessor *preprocessor.ExposePreprocessor
	cache              cache.Cache
}

// StackResponse represents standard stack data
type StackResponse struct {
	Name               string                          `json:"name"`
	Namespace          string                          `json:"namespace"`
	BlueprintReference string                          `json:"blueprintReference"`
	EnvReference       string                          `json:"envReference"`
	Phase              string                          `json:"phase,omitempty"`
	Services           map[string]common.ServiceStatus `json:"services,omitempty"`
}

// FormattableStack wraps a k8s Stack to implement common.Formattable
type FormattableStack struct {
	k8sObj    *envv1alpha1.Stack
	nsManager *authz.NamespaceManager
}

func (f *FormattableStack) ToDetailed() (common.DetailedResponse, error) {
	return common.NewDetailedResponse(f.k8sObj.ObjectMeta, f.k8sObj.Spec, f.nsManager)
}

func (f *FormattableStack) ToStandard() interface{} {
	return extractStackResponse(f.k8sObj)
}

// extractStackResponse extracts standard data from stack
func extractStackResponse(stack *envv1alpha1.Stack) StackResponse {
	return StackResponse{
		Name:               stack.Name,
		Namespace:          stack.Namespace,
		BlueprintReference: stack.Spec.BlueprintReference,
		EnvReference:       stack.Spec.Env,
		Phase:              string(stack.Status.Phase),
		Services:           convertAllServiceStatuses(stack.Status.Services),
	}
}

// NewHandler creates a new stack handler
func NewHandler(
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	cfg *controllerconfig.Config,
	cache cache.Cache,
) *Handler {
	// Create internal config if available
	var internalConfig *preprocessor.IngressConfig
	if cfg.Stacks.Ingress.Internal != nil {
		internalConfig = &preprocessor.IngressConfig{
			IngressClass: cfg.Stacks.Ingress.Internal.IngressClass,
			HostSuffix:   cfg.Stacks.Ingress.Internal.HostSuffix,
			TLSSecret:    cfg.Stacks.Ingress.Internal.TLSSecret,
		}
	}

	// Create internet config if available
	var internetConfig *preprocessor.IngressConfig
	if cfg.Stacks.Ingress.Internet != nil {
		internetConfig = &preprocessor.IngressConfig{
			IngressClass: cfg.Stacks.Ingress.Internet.IngressClass,
			HostSuffix:   cfg.Stacks.Ingress.Internet.HostSuffix,
			TLSSecret:    cfg.Stacks.Ingress.Internet.TLSSecret,
		}
	}

	// Create expose preprocessor with internal and internet configs
	exposePreprocessor := preprocessor.NewExposePreprocessor(internalConfig, internetConfig)

	return &Handler{
		k8sClient:          k8sClient,
		authorizer:         authorizer,
		nsManager:          nsManager,
		config:             cfg,
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

	// Validate blueprint reference format
	_, _, err := h.nsManager.ParseScopedID(req.Blueprint)
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
	blueprintNamespace, blueprintName, err := h.nsManager.ParseScopedID(req.Blueprint)
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

		// Extract container_name from service if specified
		service := composeConfig.Services[serviceName]
		if service.ContainerName != "" {
			imageInfo.ContainerName = service.ContainerName
			enrichedImages[serviceName] = imageInfo
			logging.Logger.Info("Extracted custom container name",
				zap.String("service", serviceName),
				zap.String("containerName", service.ContainerName))
		}

		// Apply provided image to service
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
	processedServices, err := h.exposePreprocessor.ProcessServices(composeConfig.Services, envName, stackName)
	if err != nil {
		logging.Logger.Error("Failed to process service exposure configuration",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(400, fmt.Sprintf("Service exposure configuration error: %s", err.Error()))
	}
	composeConfig.Services = processedServices

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
				"lissto.dev/created-by":      user.Name, // NEW: for metadata injection
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
	identifier := h.nsManager.MustGenerateScopedID(namespace, stackName)
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
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces for authorization
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionRead, authz.ResourceStack, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to find the stack
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	stack, found := h.findStack(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS)
	if !found {
		return c.String(404, fmt.Sprintf("Stack '%s' not found", idParam))
	}

	return common.HandleFormatResponse(c, &FormattableStack{k8sObj: stack, nsManager: h.nsManager})
}

// findStack searches for a stack in the appropriate namespace(s)
func (h *Handler) findStack(c echo.Context, targetNS, name string, searchAll bool, userNS, globalNS string, allowedNS []string) (*envv1alpha1.Stack, bool) {
	ctx := c.Request().Context()

	// Get ordered list of namespaces to search
	namespaces := namespace.ResolveNamespacesToSearch(targetNS, userNS, globalNS, searchAll, allowedNS)

	// Try each namespace in order
	for _, ns := range namespaces {
		if stack, err := h.k8sClient.GetStack(ctx, ns, name); err == nil {
			return stack, true
		}
	}

	return nil, false
}

// DeleteStack handles DELETE /stacks/:id
func (h *Handler) DeleteStack(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces for authorization
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionDelete, authz.ResourceStack, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to delete the stack
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	if h.deleteStack(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS) {
		return c.NoContent(204)
	}

	return c.String(404, fmt.Sprintf("Stack '%s' not found", idParam))
}

// deleteStack searches for and deletes a stack in the appropriate namespace(s)
func (h *Handler) deleteStack(c echo.Context, targetNS, name string, searchAll bool, userNS, globalNS string, allowedNS []string) bool {
	ctx := c.Request().Context()

	// Get ordered list of namespaces to search
	namespaces := namespace.ResolveNamespacesToSearch(targetNS, userNS, globalNS, searchAll, allowedNS)

	// Try to delete from each namespace in order
	for _, ns := range namespaces {
		if h.k8sClient.DeleteStack(ctx, ns, name) == nil {
			return true
		}
	}

	return false
}

// UpdateStack handles PUT /stacks/:id
func (h *Handler) UpdateStack(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Parse request body
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
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionUpdate, authz.ResourceStack, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to find the stack
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	stack, found := h.findStack(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS)
	if !found {
		return c.String(404, fmt.Sprintf("Stack '%s' not found", idParam))
	}

	return h.updateStackImages(c, stack, req.Images, user.Name)
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
			Digest:        newDigest,
			Image:         newImage,                   // Use new tag if provided
			URL:           existingInfo.URL,           // Preserve URL
			ContainerName: existingInfo.ContainerName, // Preserve container name
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
	identifier := h.nsManager.MustGenerateScopedID(stack.Namespace, stack.Name)
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
	// 1. Extract service labels before Kompose conversion (for command override)
	serviceLabelMap := h.extractServiceLabels(project)

	// 2. Serialize preprocessed project to compose YAML
	ser := serializer.NewComposeSerializer()
	composeYAML, err := ser.Serialize(project)
	if err != nil {
		return "", fmt.Errorf("failed to serialize Docker Compose: %w", err)
	}

	// 3. Convert with Kompose (pure conversion)
	converter := kompose.NewConverter(namespace)
	objects, err := converter.ConvertToObjects(composeYAML)
	if err != nil {
		return "", fmt.Errorf("kompose conversion failed: %w", err)
	}

	// 4. Post-process: normalize PVC accessModes to ReadWriteOnce
	pvcNormalizer := postprocessor.NewPVCAccessModeNormalizer()
	objects = pvcNormalizer.NormalizeAccessModes(objects)

	// 5. Post-process: inject stack labels to pod templates
	labelInjector := postprocessor.NewStackLabelInjector()
	objects = labelInjector.InjectLabels(objects, stackName)

	// 6. Post-process: override commands based on lissto.dev labels
	commandOverrider := postprocessor.NewCommandOverrider()
	objects = commandOverrider.OverrideCommands(objects, serviceLabelMap)

	// 7. Post-process: inject resource class annotations (required by controller)
	classifier := postprocessor.NewResourceClassifier()
	objects = classifier.InjectClassAnnotations(objects)

	// 8. Serialize to YAML
	yamlManifests, err := converter.SerializeToYAML(objects)
	if err != nil {
		return "", fmt.Errorf("YAML serialization failed: %w", err)
	}

	return yamlManifests, nil
}

// extractServiceLabels extracts labels from each service before Kompose conversion
// This is needed for command override postprocessor which needs access to original labels
func (h *Handler) extractServiceLabels(project *types.Project) map[string]map[string]string {
	labelMap := make(map[string]map[string]string)
	for name, service := range project.Services {
		if service.Labels != nil {
			labelMap[name] = service.Labels
		}
	}
	return labelMap
}

// SuspendStack handles POST /stacks/:id/suspend
func (h *Handler) SuspendStack(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	var req common.SuspendStackRequest
	if err := c.Bind(&req); err != nil {
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		return c.String(400, err.Error())
	}

	// Get allowed namespaces for suspend
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionSuspend, authz.ResourceStack, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to find the stack
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	stack, found := h.findStack(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS)
	if !found {
		return c.String(404, fmt.Sprintf("Stack '%s' not found", idParam))
	}

	// Create suspension spec
	suspension := &envv1alpha1.SuspensionSpec{
		Services: req.Services,
	}

	// Parse timeout if provided
	if req.Timeout != "" {
		timeout, err := metav1.ParseDuration(req.Timeout)
		if err != nil {
			return c.String(400, fmt.Sprintf("Invalid timeout format: %v", err))
		}
		suspension.Timeout = &timeout
	}

	// Update stack with suspension
	stack.Spec.Suspension = suspension

	if err := h.k8sClient.UpdateStack(c.Request().Context(), stack); err != nil {
		logging.Logger.Error("Failed to suspend stack",
			zap.String("namespace", stack.Namespace),
			zap.String("name", stack.Name),
			zap.Error(err))
		return c.String(500, "Failed to suspend stack")
	}

	logging.Logger.Info("Stack suspended successfully",
		zap.String("stack_name", stack.Name),
		zap.String("namespace", stack.Namespace),
		zap.String("user", user.Name),
		zap.Strings("services", req.Services))

	return c.JSON(200, map[string]interface{}{
		"message": "Stack suspension initiated",
		"phase":   string(stack.Status.Phase),
	})
}

// ResumeStack handles POST /stacks/:id/resume
func (h *Handler) ResumeStack(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces for resume
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionResume, authz.ResourceStack, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to find the stack
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	stack, found := h.findStack(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS)
	if !found {
		return c.String(404, fmt.Sprintf("Stack '%s' not found", idParam))
	}

	// Clear suspension
	stack.Spec.Suspension = nil

	if err := h.k8sClient.UpdateStack(c.Request().Context(), stack); err != nil {
		logging.Logger.Error("Failed to resume stack",
			zap.String("namespace", stack.Namespace),
			zap.String("name", stack.Name),
			zap.Error(err))
		return c.String(500, "Failed to resume stack")
	}

	logging.Logger.Info("Stack resumed successfully",
		zap.String("stack_name", stack.Name),
		zap.String("namespace", stack.Namespace),
		zap.String("user", user.Name))

	return c.JSON(200, map[string]interface{}{
		"message": "Stack resume initiated",
		"phase":   string(stack.Status.Phase),
	})
}

// GetStackPhase handles GET /stacks/:id/phase
func (h *Handler) GetStackPhase(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionRead, authz.ResourceStack, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to find the stack
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	stack, found := h.findStack(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS)
	if !found {
		return c.String(404, fmt.Sprintf("Stack '%s' not found", idParam))
	}

	// Build phase response
	resp := common.StackPhaseResponse{
		Phase: string(stack.Status.Phase),
	}

	// Convert phase history
	if len(stack.Status.PhaseHistory) > 0 {
		resp.PhaseHistory = make([]common.PhaseTransition, 0, len(stack.Status.PhaseHistory))
		for _, pt := range stack.Status.PhaseHistory {
			resp.PhaseHistory = append(resp.PhaseHistory, common.PhaseTransition{
				Phase:          string(pt.Phase),
				TransitionTime: pt.TransitionTime.Format("2006-01-02T15:04:05Z"),
				Reason:         pt.Reason,
				Message:        pt.Message,
			})
		}
	}

	// Convert service status
	resp.Services = convertAllServiceStatuses(stack.Status.Services)

	return c.JSON(200, resp)
}

// convertServiceStatus converts a single service status from controller format to API response format
func convertServiceStatus(serviceStatus envv1alpha1.ServiceStatus) common.ServiceStatus {
	ss := common.ServiceStatus{
		Phase: string(serviceStatus.Phase),
	}
	if serviceStatus.SuspendedAt != nil {
		ss.SuspendedAt = serviceStatus.SuspendedAt.Format("2006-01-02T15:04:05Z")
	}
	return ss
}

// convertAllServiceStatuses converts a map of service statuses from controller to API format
func convertAllServiceStatuses(services map[string]envv1alpha1.ServiceStatus) map[string]common.ServiceStatus {
	if len(services) == 0 {
		return nil
	}
	result := make(map[string]common.ServiceStatus)
	for name, status := range services {
		result[name] = convertServiceStatus(status)
	}
	return result
}

// RestoreStack handles POST /stacks/:id/restore (stub for future implementation)
func (h *Handler) RestoreStack(c echo.Context) error {
	return c.String(501, "Stack restoration from snapshots is not yet implemented")
}
