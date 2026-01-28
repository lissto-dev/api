package blueprint

import (
	"fmt"

	"github.com/labstack/echo/v4"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/lissto-dev/api/internal/api/common"
	"github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/compose"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	controllerconfig "github.com/lissto-dev/controller/pkg/config"
	"github.com/lissto-dev/controller/pkg/namespace"
	"go.uber.org/zap"
)

// Handler handles blueprint-related HTTP requests
type Handler struct {
	k8sClient  *k8s.Client
	authorizer *authz.Authorizer
	nsManager  *authz.NamespaceManager
	config     *controllerconfig.Config
}

// NewHandler creates a new blueprint handler
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

// CreateBlueprint handles POST /blueprints
func (h *Handler) CreateBlueprint(c echo.Context) error {
	var req common.CreateBlueprintRequest
	user, _ := middleware.GetUserFromContext(c)

	// Bind and validate first
	if err := c.Bind(&req); err != nil {
		logging.Logger.Error("Failed to bind request", zap.Error(err))
		return c.String(400, "Invalid request")
	}
	if err := c.Validate(&req); err != nil {
		logging.Logger.Error("Request validation failed", zap.Error(err))
		return c.String(400, err.Error())
	}

	// Log request details after binding
	logging.Logger.Info("Blueprint creation request",
		zap.String("user", user.Name),
		zap.String("role", user.Role.String()),
		zap.String("branch", req.Branch),
		zap.String("author", req.Author),
		zap.String("repository", req.Repository),
		zap.String("ip", c.RealIP()))

	// Repository is required for all roles
	if req.Repository == "" {
		logging.Logger.Error("Repository is required",
			zap.String("user", user.Name),
			zap.String("role", user.Role.String()))
		return c.String(400, "Repository field is required")
	}

	// Validate repository and get config
	repoKey, valid := h.config.ValidateRepository(req.Repository)
	if !valid {
		logging.Logger.Error("Unknown repository",
			zap.String("user", user.Name),
			zap.String("repository", req.Repository))
		return c.String(400, fmt.Sprintf("Repository '%s' is not configured. Only configured repositories are allowed.", req.Repository))
	}

	// Get the repository configuration for title extraction
	repoConfig := h.config.Repos[repoKey]

	// Determine target namespace using blueprint-specific method
	namespace, err := h.authorizer.DetermineNamespaceForBlueprint(user.Role, user.Name, &req)
	if err != nil {
		logging.Logger.Error("Namespace determination failed",
			zap.String("user", user.Name),
			zap.String("role", user.Role.String()),
			zap.String("branch", req.Branch),
			zap.String("author", req.Author),
			zap.Error(err))
		return c.String(400, err.Error())
	}

	logging.Logger.Info("Namespace determined",
		zap.String("namespace", namespace),
		zap.String("user", user.Name),
		zap.String("role", user.Role.String()))

	// Check authorization
	perm := h.authorizer.CanAccess(user.Role, authz.ActionCreate, authz.ResourceBlueprint, namespace, user.Name)
	if !perm.Allowed {
		logging.Logger.Error("Authorization denied",
			zap.String("user", user.Name),
			zap.String("role", user.Role.String()),
			zap.String("action", string(authz.ActionCreate)),
			zap.String("resource", string(authz.ResourceBlueprint)),
			zap.String("namespace", namespace),
			zap.String("reason", perm.Reason))
		return c.String(403, fmt.Sprintf("Permission denied: %s", perm.Reason))
	}

	// Hash the docker-compose content
	fullHash := req.HashDockerCompose()
	shortHash := fullHash
	if len(fullHash) > 8 {
		shortHash = fullHash[:8]
	}

	// Check if blueprint with this hash already exists (deduplication)
	// For deploy role, check all namespaces; for others, check only target namespace
	var blueprintList *envv1alpha1.BlueprintList

	if user.Role == authz.Deploy {
		// Deploy role: check all namespaces for duplicates
		blueprintList, err = h.k8sClient.ListBlueprints(c.Request().Context(), "")
		if err != nil {
			logging.Logger.Error("Failed to query blueprints across all namespaces",
				zap.Error(err))
			return c.String(500, "Failed to query blueprints")
		}
	} else {
		// Other roles: check only target namespace
		blueprintList, err = h.k8sClient.ListBlueprints(c.Request().Context(), namespace)
		if err != nil {
			logging.Logger.Error("Failed to query blueprints",
				zap.String("namespace", namespace),
				zap.Error(err))
			return c.String(500, "Failed to query blueprints")
		}
	}

	// Check for duplicates with priority: target namespace first, then global namespace
	var targetNamespaceMatch *envv1alpha1.Blueprint
	var globalNamespaceMatch *envv1alpha1.Blueprint
	globalNamespace := h.nsManager.GetGlobalNamespace()

	for _, bp := range blueprintList.Items {
		if bp.Labels != nil && bp.Labels["hash"] == shortHash {
			switch bp.Namespace {
			case namespace:
				targetNamespaceMatch = &bp
			case globalNamespace:
				globalNamespaceMatch = &bp
			}
		}
	}

	// Return the most appropriate match
	if targetNamespaceMatch != nil {
		// Same content already exists in target namespace - return 200 with identifier
		identifier := h.nsManager.MustGenerateScopedID(namespace, targetNamespaceMatch.Name)
		logging.Logger.Info("Blueprint already exists in target namespace",
			zap.String("user", user.Name),
			zap.String("namespace", namespace),
			zap.String("blueprint", targetNamespaceMatch.Name),
			zap.String("identifier", identifier))
		return c.String(200, identifier)
	}

	if globalNamespaceMatch != nil && user.Role == authz.Deploy {
		// Deploy role found duplicate in global namespace - return 200 with global identifier
		identifier := h.nsManager.MustGenerateScopedID(globalNamespace, globalNamespaceMatch.Name)
		logging.Logger.Info("Deploy role found duplicate in global namespace",
			zap.String("user", user.Name),
			zap.String("target_namespace", namespace),
			zap.String("global_namespace", globalNamespace),
			zap.String("blueprint", globalNamespaceMatch.Name),
			zap.String("identifier", identifier))
		return c.String(200, identifier)
	}

	// Blueprint doesn't exist - create new one

	// Parse docker-compose to extract metadata (title, services)
	// If parsing fails, don't create blueprint
	// Pass repo config for title extraction with priority: x-lissto.title → repo.Name → repo.URL
	metadata, err := compose.ParseBlueprintMetadata(req.Compose, repoConfig)
	if err != nil {
		logging.Logger.Error("Failed to parse docker-compose",
			zap.String("user", user.Name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(400, fmt.Sprintf("Invalid docker-compose content: %v", err))
	}

	// Convert service metadata to JSON for annotation storage
	servicesJSON, err := compose.ServiceMetadataToJSON(metadata.Services)
	if err != nil {
		logging.Logger.Error("Failed to serialize service metadata",
			zap.String("user", user.Name),
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to process blueprint metadata")
	}

	// Ensure namespace exists
	if err := h.k8sClient.EnsureNamespace(c.Request().Context(), namespace); err != nil {
		logging.Logger.Error("Failed to create namespace",
			zap.String("namespace", namespace),
			zap.Error(err))
		return c.String(500, "Failed to create namespace")
	}

	// Generate blueprint name with timestamp
	blueprintName := common.GenerateBlueprintName(fullHash)

	// Prepare annotations
	annotations := make(map[string]string)
	if metadata.Title != "" {
		annotations["lissto.dev/title"] = metadata.Title
	}
	if req.Repository != "" {
		// Normalize repository URL before storing for consistent comparison
		normalizedRepo := controllerconfig.NormalizeRepositoryURL(req.Repository)
		annotations["lissto.dev/repository"] = normalizedRepo
	}
	annotations["lissto.dev/services"] = servicesJSON

	// Create Blueprint CRD
	blueprint := &envv1alpha1.Blueprint{
		ObjectMeta: metav1.ObjectMeta{
			Name:      blueprintName,
			Namespace: namespace,
			Labels: map[string]string{
				"hash":   shortHash,
				"branch": req.Branch,
			},
			Annotations: annotations,
		},
		Spec: envv1alpha1.BlueprintSpec{
			DockerCompose: req.Compose,
			Hash:          fullHash,
		},
	}

	if err := h.k8sClient.CreateBlueprint(c.Request().Context(), blueprint); err != nil {
		logging.Logger.Error("Failed to create blueprint",
			zap.String("namespace", namespace),
			zap.String("name", blueprintName),
			zap.Error(err))
		return c.String(500, "Failed to create blueprint")
	}

	// Return 201 with scoped identifier
	identifier := h.nsManager.MustGenerateScopedID(namespace, blueprintName)
	return c.String(201, identifier)
}

// BlueprintResponse represents enriched blueprint data
type BlueprintResponse struct {
	ID      string                  `json:"id"`
	Title   string                  `json:"title"`
	Content compose.ServiceMetadata `json:"content"`
}

// FormattableBlueprint wraps a k8s Blueprint to implement common.Formattable
type FormattableBlueprint struct {
	K8sObj    *envv1alpha1.Blueprint
	NsManager *authz.NamespaceManager
}

func (f *FormattableBlueprint) ToDetailed() (common.DetailedResponse, error) {
	return common.NewDetailedResponse(f.K8sObj.ObjectMeta, f.K8sObj.Spec, f.NsManager)
}

func (f *FormattableBlueprint) ToStandard() interface{} {
	return extractBlueprintResponse(f.K8sObj, f.NsManager)
}

// extractBlueprintResponse extracts enriched data from blueprint annotations
func extractBlueprintResponse(bp *envv1alpha1.Blueprint, nsManager *authz.NamespaceManager) BlueprintResponse {
	identifier := nsManager.MustGenerateScopedID(bp.Namespace, bp.Name)

	// Extract title
	title := common.ExtractBlueprintTitle(bp, "")
	var services compose.ServiceMetadata

	if bp.Annotations != nil {
		if servicesJSON, ok := bp.Annotations["lissto.dev/services"]; ok && servicesJSON != "" {
			if parsedServices, err := compose.ServiceMetadataFromJSON(servicesJSON); err == nil {
				services = *parsedServices
			}
		}
	}

	// Ensure empty slices instead of nil
	if services.Services == nil {
		services.Services = []string{}
	}
	if services.Infra == nil {
		services.Infra = []string{}
	}

	return BlueprintResponse{
		ID:      identifier,
		Title:   title,
		Content: services,
	}
}

// GetBlueprints handles GET /blueprints
// Supports ?format=metadata for lightweight responses (metadata only, no spec)
func (h *Handler) GetBlueprints(c echo.Context) error {
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces
	allowedNS := h.authorizer.GetAllowedNamespaces(
		user.Role,
		authz.ActionList,
		authz.ResourceBlueprint,
		user.Name,
	)

	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Collect all blueprint items first
	var allItems []envv1alpha1.Blueprint

	if allowedNS[0] == "*" {
		// Admin: list from all namespaces
		bpList, err := h.k8sClient.ListBlueprints(c.Request().Context(), "")
		if err != nil {
			return c.String(500, "Failed to list blueprints")
		}
		allItems = append(allItems, bpList.Items...)
	} else {
		// List from each allowed namespace
		for _, ns := range allowedNS {
			bpList, err := h.k8sClient.ListBlueprints(c.Request().Context(), ns)
			if err != nil {
				continue
			}
			allItems = append(allItems, bpList.Items...)
		}
	}

	// Check format parameter
	format := c.QueryParam("format")

	if format == "metadata" {
		// Return []MetadataOnlyResponse - lightweight, no spec
		// Inspired by K8s PartialObjectMetadata pattern
		var metadataList []common.MetadataOnlyResponse
		for i := range allItems {
			metadata, err := common.ExtractDetailedMetadata(allItems[i].ObjectMeta, h.nsManager)
			if err != nil {
				// Skip items with metadata extraction errors
				continue
			}
			metadataList = append(metadataList, common.MetadataOnlyResponse{Metadata: metadata})
		}
		return c.JSON(200, metadataList)
	}

	// Default: return standard BlueprintResponse array
	var allBlueprints []BlueprintResponse
	for i := range allItems {
		allBlueprints = append(allBlueprints, extractBlueprintResponse(&allItems[i], h.nsManager))
	}

	return c.JSON(200, allBlueprints)
}

// GetBlueprint handles GET /blueprints/:id
func (h *Handler) GetBlueprint(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces for authorization
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionRead, authz.ResourceBlueprint, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to find the blueprint
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	blueprint, found := h.findBlueprint(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS)
	if !found {
		return c.String(404, fmt.Sprintf("Blueprint '%s' not found", idParam))
	}

	return common.HandleFormatResponse(c, &FormattableBlueprint{K8sObj: blueprint, NsManager: h.nsManager})
}

// findBlueprint searches for a blueprint in the appropriate namespace(s)
func (h *Handler) findBlueprint(c echo.Context, targetNS, name string, searchAll bool, userNS, globalNS string, allowedNS []string) (*envv1alpha1.Blueprint, bool) {
	ctx := c.Request().Context()

	// Get ordered list of namespaces to search
	namespaces := namespace.ResolveNamespacesToSearch(targetNS, userNS, globalNS, searchAll, allowedNS)

	// Try each namespace in order
	for _, ns := range namespaces {
		if bp, err := h.k8sClient.GetBlueprint(ctx, ns, name); err == nil {
			return bp, true
		}
	}

	return nil, false
}

// DeleteBlueprint handles DELETE /blueprints/:id
func (h *Handler) DeleteBlueprint(c echo.Context) error {
	idParam := c.Param("id")
	user, _ := middleware.GetUserFromContext(c)

	// Get allowed namespaces for authorization
	allowedNS := h.authorizer.GetAllowedNamespaces(user.Role, authz.ActionDelete, authz.ResourceBlueprint, user.Name)
	if len(allowedNS) == 0 {
		return c.String(403, "Permission denied: no accessible namespaces")
	}

	// Resolve namespace from ID
	targetNamespace, name, searchAll := h.nsManager.ResolveNamespaceFromID(idParam, allowedNS)

	// Try to delete the blueprint
	userNS := h.nsManager.GetDeveloperNamespace(user.Name)
	globalNS := h.nsManager.GetGlobalNamespace()
	if h.deleteBlueprint(c, targetNamespace, name, searchAll, userNS, globalNS, allowedNS) {
		return c.NoContent(204)
	}

	return c.String(404, fmt.Sprintf("Blueprint '%s' not found", idParam))
}

// deleteBlueprint searches for and deletes a blueprint in the appropriate namespace(s)
func (h *Handler) deleteBlueprint(c echo.Context, targetNS, name string, searchAll bool, userNS, globalNS string, allowedNS []string) bool {
	ctx := c.Request().Context()

	// Get ordered list of namespaces to search
	namespaces := namespace.ResolveNamespacesToSearch(targetNS, userNS, globalNS, searchAll, allowedNS)

	// Try to delete from each namespace in order
	for _, ns := range namespaces {
		if h.k8sClient.DeleteBlueprint(ctx, ns, name) == nil {
			return true
		}
	}

	return false
}
