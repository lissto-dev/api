package prepare

import (
	"context"
	"fmt"
	"time"

	"github.com/compose-spec/compose-go/v2/loader"
	"github.com/compose-spec/compose-go/v2/types"
	"github.com/google/uuid"
	"github.com/labstack/echo/v4"
	"go.uber.org/zap"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/cache"
	"github.com/lissto-dev/api/pkg/compose"
	"github.com/lissto-dev/api/pkg/image"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	"github.com/lissto-dev/api/pkg/preprocessor"
	operatorConfig "github.com/lissto-dev/controller/pkg/config"
)

// Handler handles stack preparation requests
type Handler struct {
	k8sClient     *k8s.Client
	authorizer    *authz.Authorizer
	nsManager     *authz.NamespaceManager
	config        *operatorConfig.Config
	imageResolver *image.ImageResolver
	cache         cache.Cache
}

// NewHandler creates a new stack preparation handler
func NewHandler(
	k8sClient *k8s.Client,
	authorizer *authz.Authorizer,
	nsManager *authz.NamespaceManager,
	config *operatorConfig.Config,
	cache cache.Cache,
) *Handler {
	// Create image existence checker with K8s authentication
	// This will automatically use:
	// - Image pull secrets from the pod's service account
	// - Node IAM credentials (ECR on AWS, Workload Identity on GCP, etc.)
	// - Docker config files and credential helpers
	// Falls back to anonymous access if authentication is not available
	ctx := context.Background()
	imageChecker := image.NewImageExistenceCheckerWithK8sAuth(ctx)

	// Create image resolver with global config
	imageResolver := image.NewImageResolver(
		config.Stacks.Global.Images.Registry,
		config.Stacks.Global.Images.RepositoryPrefix,
		imageChecker,
	)

	logging.Logger.Info("Image resolver created with global config",
		zap.String("global_registry", config.Stacks.Global.Images.Registry),
		zap.String("global_repository_prefix", config.Stacks.Global.Images.RepositoryPrefix))

	return &Handler{
		k8sClient:     k8sClient,
		authorizer:    authorizer,
		nsManager:     nsManager,
		config:        config,
		imageResolver: imageResolver,
		cache:         cache,
	}
}

// PrepareStack handles POST /stacks/prepare
func (h *Handler) PrepareStack(c echo.Context) error {
	var req common.PrepareStackRequest
	user, _ := middleware.GetUserFromContext(c)

	// Bind and validate
	if err := c.Bind(&req); err != nil {
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		return c.String(400, err.Error())
	}

	logging.Logger.Info("Stack prepare request",
		zap.String("user", user.Name),
		zap.String("blueprint", req.Blueprint),
		zap.String("commit", req.Commit),
		zap.String("branch", req.Branch),
		zap.String("tag", req.Tag),
		zap.String("env", req.Env))

	// Validate env exists
	namespace := h.nsManager.GetDeveloperNamespace(user.Name)
	env, err := h.k8sClient.GetEnv(c.Request().Context(), namespace, req.Env)
	if err != nil {
		logging.Logger.Error("Failed to get env",
			zap.String("env", req.Env),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(404, fmt.Sprintf("Env '%s' not found", req.Env))
	}
	if env == nil {
		return c.String(404, fmt.Sprintf("Env '%s' not found", req.Env))
	}

	// Parse blueprint reference
	blueprintNamespace, blueprintName, err := common.ParseBlueprintReference(req.Blueprint)
	if err != nil {
		logging.Logger.Error("Failed to parse blueprint reference",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(400, fmt.Sprintf("Invalid blueprint reference: %v", err))
	}

	// Get blueprint from Kubernetes
	blueprint, err := h.k8sClient.GetBlueprint(c.Request().Context(), blueprintNamespace, blueprintName)
	if err != nil {
		logging.Logger.Error("Failed to get blueprint",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(404, "Blueprint not found")
	}

	// Parse Docker Compose content
	project, err := h.parseDockerCompose(blueprint.Spec.DockerCompose)
	if err != nil {
		logging.Logger.Error("Failed to parse Docker Compose",
			zap.String("blueprint", req.Blueprint),
			zap.Error(err))
		return c.String(400, "Invalid Docker Compose content")
	}

	// Extract x-lissto configuration from compose file
	lisstoConfig := compose.ExtractLisstoConfig(project)
	logging.Logger.Info("Extracted x-lissto configuration",
		zap.String("registry", lisstoConfig.Registry),
		zap.String("repository", lisstoConfig.Repository),
		zap.String("repositoryPrefix", lisstoConfig.RepositoryPrefix))

	// Create expose preprocessor for checking exposed services and calculating URLs
	exposePreprocessor := preprocessor.NewExposePreprocessor(
		h.config.Stacks.Public.HostSuffix,
		h.config.Stacks.Public.IngressClass,
	)

	// Resolve images for each service
	var results []common.DetailedImageResolutionInfo
	var exposedServices []common.ExposedServiceInfo

	logging.Logger.Info("Starting image resolution for services",
		zap.Int("total_services", len(project.Services)),
		zap.Strings("service_names", getServiceNames(project.Services)),
		zap.Bool("detailed", req.Detailed),
		zap.String("env", req.Env))

	for serviceName, service := range project.Services {
		logging.Logger.Info("Processing service for image resolution",
			zap.String("service", serviceName),
			zap.String("has_image", fmt.Sprintf("%t", service.Image != "")),
			zap.String("has_build", fmt.Sprintf("%t", service.Build != nil)),
			zap.String("image", service.Image),
			zap.Any("labels", service.Labels))

		// Always collect detailed information
		var info common.DetailedImageResolutionInfo
		info.Service = serviceName

		// If service has image, resolve to digest
		if service.Image != "" {
			logging.Logger.Info("Service has explicit image, resolving to digest",
				zap.String("service", serviceName),
				zap.String("image", service.Image))

			imageWithDigest, err := h.imageResolver.GetImageDigest(service.Image)
			if err != nil {
				logging.Logger.Error("Failed to get image digest",
					zap.String("service", serviceName),
					zap.String("image", service.Image),
					zap.Error(err))

				// In detailed mode, continue processing and show the error
				if req.Detailed {
					info.Image = service.Image // Keep original image even on error
					info.Method = "original"
					info.Candidates = []common.ImageCandidate{{
						ImageURL: service.Image,
						Tag:      "original",
						Source:   "original",
						Success:  false,
						Error:    err.Error(),
					}}
				} else {
					return c.String(400, fmt.Sprintf("Failed to resolve image for service %s: %v", serviceName, err))
				}
			} else {
				info.Digest = imageWithDigest // Full digest (e.g., nginx@sha256:...)
				info.Image = service.Image    // User-friendly tag (e.g., nginx:alpine)
				info.Method = "original"
				info.Candidates = []common.ImageCandidate{{
					ImageURL: service.Image,
					Tag:      "original",
					Source:   "original",
					Success:  true,
					Digest:   imageWithDigest,
				}}
			}
		} else {
			// Service has build or needs resolution - try candidates
			logging.Logger.Info("Service needs image resolution, trying candidates",
				zap.String("service", serviceName),
				zap.String("commit", req.Commit),
				zap.String("branch", req.Branch))

			result, err := h.imageResolver.ResolveImageDetailed(
				service,
				image.ResolutionConfig{
					Commit:            req.Commit,
					Branch:            req.Branch,
					ComposeRegistry:   lisstoConfig.Registry,
					ComposeRepository: lisstoConfig.Repository,
					ComposePrefix:     lisstoConfig.RepositoryPrefix,
				},
			)
			if err != nil {
				logging.Logger.Error("Failed to resolve image for service",
					zap.String("service", serviceName),
					zap.Error(err))

				// Always use result data, even on error
				info.Digest = result.FinalImage
				info.Image = result.Selected
				info.Method = result.Method
				info.Registry = result.Registry
				info.ImageName = result.ImageName
				info.Candidates = result.Candidates

				// In standard mode, return error immediately
				if !req.Detailed {
					return c.String(400, fmt.Sprintf("Failed to resolve image for service %s: %v", serviceName, err))
				}
			} else {
				info.Digest = result.FinalImage
				info.Image = result.Selected
				info.Method = result.Method
				info.Registry = result.Registry
				info.ImageName = result.ImageName
				info.Candidates = result.Candidates
			}
		}

		// Check if service is exposed and calculate URL (env is now mandatory)
		exposedURL := exposePreprocessor.GetExposedServiceURL(service, serviceName, req.Env)
		if exposedURL != "" {
			info.Exposed = true
			info.URL = exposedURL
			exposedServices = append(exposedServices, common.ExposedServiceInfo{
				Service: serviceName,
				URL:     exposedURL,
			})
		} else {
			info.Exposed = false
		}

		results = append(results, info)

		logging.Logger.Info("Image resolved for service",
			zap.String("service", serviceName),
			zap.String("digest", info.Digest),
			zap.String("image", info.Image),
			zap.String("method", info.Method),
			zap.Bool("exposed", info.Exposed),
			zap.String("url", info.URL),
			zap.Int("candidates_tried", len(info.Candidates)))
	}

	// Generate request ID
	requestID := uuid.New().String()

	// Build cache entry with namespace for ownership verification
	cacheEntry := &cache.PrepareResultCache{
		Namespace: namespace,
		Images:    make(map[string]cache.ImageInfoCache),
	}

	for _, result := range results {
		cacheEntry.Images[result.Service] = cache.ImageInfoCache{
			Digest: result.Digest, // Full digest
			Image:  result.Image,  // User-friendly tag
			URL:    result.URL,    // Exposed URL (if applicable)
		}
	}

	// Cache with 15 min TTL
	if err := h.cache.Set(c.Request().Context(), requestID, cacheEntry, 15*time.Minute); err != nil {
		logging.Logger.Warn("Failed to cache prepare result", zap.Error(err))
		// Continue anyway - cache is optional
	} else {
		logging.Logger.Info("Cached prepare result",
			zap.String("request_id", requestID),
			zap.String("namespace", namespace),
			zap.Int("services", len(cacheEntry.Images)))
	}

	// Return appropriate response based on mode
	if req.Detailed {
		response := common.DetailedPrepareStackResponse{
			RequestID: requestID,
			Blueprint: req.Blueprint,
			Images:    results,
			Exposed:   exposedServices,
		}

		return c.JSON(200, response)
	} else {
		// Convert to standard format
		images := make([]common.ImageResolutionInfo, len(results))
		for i, result := range results {
			images[i] = common.ImageResolutionInfo{
				Service: result.Service,
				Image:   result.Digest,
				Method:  result.Method,
				Tag:     result.Image,
			}
		}

		response := common.PrepareStackResponse{
			Blueprint: req.Blueprint,
			Images:    images,
		}

		return c.JSON(200, response)
	}
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

// getServiceNames extracts service names from the project services map
func getServiceNames(services map[string]types.ServiceConfig) []string {
	names := make([]string, 0, len(services))
	for name := range services {
		names = append(names, name)
	}
	return names
}
