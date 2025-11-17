package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/go-playground/validator/v10"
	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	"go.uber.org/zap"

	internalMiddleware "github.com/lissto-dev/api/internal/middleware"
	"github.com/lissto-dev/api/internal/server"
	"github.com/lissto-dev/api/pkg/authz"
	"github.com/lissto-dev/api/pkg/config"
	"github.com/lissto-dev/api/pkg/k8s"
	"github.com/lissto-dev/api/pkg/logging"
	pkgServer "github.com/lissto-dev/api/pkg/server"
	operatorConfig "github.com/lissto-dev/controller/pkg/config"
)

// CustomValidator wraps the validator
type CustomValidator struct {
	validator *validator.Validate
}

// Validate validates the struct
func (cv *CustomValidator) Validate(i interface{}) error {
	return cv.validator.Struct(i)
}

func main() {
	// Parse flags
	var configPath string
	var kubeconfig string
	var inCluster bool

	flag.StringVar(&configPath, "config-path", "config.local.yaml", "Path to configuration file")
	flag.StringVar(&kubeconfig, "kubeconfig", "", "Path to kubeconfig file (optional for out-of-cluster)")
	flag.BoolVar(&inCluster, "in-cluster", false, "Use in-cluster Kubernetes configuration")
	flag.Parse()

	// Load shared operator configuration
	cfg, err := operatorConfig.LoadConfig(configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}
	log.Printf("Configuration loaded from %s", configPath)

	// Get API namespace from environment (defaults to lissto-system)
	apiNamespace := os.Getenv("POD_NAMESPACE")
	if apiNamespace == "" {
		apiNamespace = "lissto-system"
		log.Printf("POD_NAMESPACE not set, defaulting to: %s", apiNamespace)
	} else {
		log.Printf("API namespace from POD_NAMESPACE: %s", apiNamespace)
	}

	// Initialize structured logging
	if err := logging.InitLogger(cfg.Logging.Level, "json"); err != nil {
		logging.Logger.Fatal("Failed to initialize logging", zap.Error(err))
	}
	logging.Logger.Info("Structured logging initialized",
		zap.String("level", cfg.Logging.Level),
		zap.String("format", "json"))

	// Initialize Kubernetes client
	k8sClient, err := k8s.NewClient(inCluster, kubeconfig)
	if err != nil {
		logging.Logger.Fatal("Failed to create Kubernetes client", zap.Error(err))
	}
	logging.Logger.Info("Kubernetes client initialized")

	// Initialize authorization components
	nsManager := authz.NewNamespaceManager(cfg)
	authorizer := authz.NewAuthorizer(nsManager)
	logging.Logger.Info("Authorization initialized")

	// Ensure global namespace exists on startup
	ctx := context.Background()
	if err := k8sClient.EnsureNamespace(ctx, cfg.Namespaces.Global); err != nil {
		logging.Logger.Fatal("Failed to ensure global namespace",
			zap.String("namespace", cfg.Namespaces.Global),
			zap.Error(err))
	}
	logging.Logger.Info("Global namespace ready", zap.String("namespace", cfg.Namespaces.Global))

	// Load API keys from Kubernetes secret (in API's own namespace)
	apiKeys, err := config.LoadAPIKeysFromSecret(ctx, k8sClient, apiNamespace, config.SecretName)
	if err != nil {
		logging.Logger.Fatal("Failed to load API keys from secret",
			zap.String("namespace", apiNamespace),
			zap.Error(err))
	}

	// Ensure admin key exists, generate if not
	var adminKeyGenerated bool
	apiKeys, adminKeyGenerated, err = config.EnsureAdminKey(apiKeys)
	if err != nil {
		logging.Logger.Fatal("Failed to ensure admin key", zap.Error(err))
	}

	// If admin key was generated, save it to the secret (in API's own namespace)
	if adminKeyGenerated {
		if err := config.SaveAPIKeysToSecret(ctx, k8sClient, apiNamespace, config.SecretName, apiKeys); err != nil {
			logging.Logger.Fatal("Failed to save admin key to secret",
				zap.String("namespace", apiNamespace),
				zap.Error(err))
		}
		logging.Logger.Info("Admin key generated and saved to secret",
			zap.String("namespace", apiNamespace),
			zap.String("secret", config.SecretName))
	}

	logging.Logger.Info("API keys loaded",
		zap.Int("count", len(apiKeys)),
		zap.String("namespace", apiNamespace))

	// Get or create API instance ID (stored in the same secret as API keys)
	instanceID, err := pkgServer.GetOrCreateInstanceID(ctx, k8sClient, apiNamespace, config.SecretName)
	if err != nil {
		logging.Logger.Fatal("Failed to get or create instance ID", zap.Error(err))
	}
	logging.Logger.Info("API instance ID initialized", zap.String("id", instanceID))

	// Load public URL from config or environment variable
	// Priority: config file > environment variable
	publicURL := cfg.API.Server.PublicURL
	if publicURL == "" {
		publicURL = os.Getenv("LISSTO_PUBLIC_URL")
		if publicURL != "" {
			logging.Logger.Info("Loaded public URL from environment", zap.String("url", publicURL))
		}
	} else {
		logging.Logger.Info("Loaded public URL from config", zap.String("url", publicURL))
	}
	if publicURL == "" {
		logging.Logger.Info("No public URL configured")
	}

	// Create Echo instance
	e := echo.New()
	e.HideBanner = true

	// Add validator
	e.Validator = &CustomValidator{validator: validator.New()}

	// Add global middleware (including API ID header)
	e.Use(middleware.Logger())
	e.Use(middleware.Recover())
	e.Use(middleware.CORS())
	e.Use(internalMiddleware.APIIDMiddleware(instanceID))

	// Initialize and start server
	srv := server.New(e, apiKeys, cfg, k8sClient, authorizer, nsManager, apiNamespace, instanceID, publicURL)
	logging.Logger.Info("Server initialized")

	if err := srv.Start(); err != nil {
		logging.Logger.Fatal("Server error", zap.Error(err))
	}
}
