package k8s

import (
	"context"
	"fmt"

	"github.com/lissto.dev/api/pkg/logging"
	"go.uber.org/zap"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"

	envv1alpha1 "github.com/lissto.dev/controller/api/v1alpha1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
)

// Client wraps controller-runtime client for managing CRDs
type Client struct {
	client.Client
	scheme *runtime.Scheme
}

// Scheme returns the runtime scheme for owner references
func (c *Client) Scheme() *runtime.Scheme {
	return c.scheme
}

// NewClient creates a new Kubernetes client
// If inCluster is true, uses in-cluster config. Otherwise, uses kubeconfig.
func NewClient(inCluster bool, kubeconfigPath string) (*Client, error) {
	// Set up controller-runtime logger
	log.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(false)))

	var config *rest.Config
	var err error

	if inCluster {
		config, err = rest.InClusterConfig()
		if err != nil {
			logging.Logger.Error("Failed to get in-cluster config", zap.Error(err))
			return nil, fmt.Errorf("failed to get in-cluster config: %w", err)
		}
	} else {
		config, err = clientcmd.BuildConfigFromFlags("", kubeconfigPath)
		if err != nil {
			logging.Logger.Error("Failed to build config from kubeconfig",
				zap.String("kubeconfig", kubeconfigPath),
				zap.Error(err))
			return nil, fmt.Errorf("failed to build config from kubeconfig: %w", err)
		}
	}

	// Create scheme and register our CRDs
	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		logging.Logger.Error("Failed to add client-go scheme", zap.Error(err))
		return nil, fmt.Errorf("failed to add client-go scheme: %w", err)
	}
	if err := envv1alpha1.AddToScheme(scheme); err != nil {
		logging.Logger.Error("Failed to add operator scheme", zap.Error(err))
		return nil, fmt.Errorf("failed to add operator scheme: %w", err)
	}

	// Create controller-runtime client
	k8sClient, err := client.New(config, client.Options{Scheme: scheme})
	if err != nil {
		logging.Logger.Error("Failed to create client", zap.Error(err))
		return nil, fmt.Errorf("failed to create client: %w", err)
	}

	return &Client{
		Client: k8sClient,
		scheme: scheme,
	}, nil
}

// CreateStack creates a Stack resource in the given namespace
func (c *Client) CreateStack(ctx context.Context, stack *envv1alpha1.Stack) error {
	return c.Create(ctx, stack)
}

// GetStack retrieves a Stack resource
func (c *Client) GetStack(ctx context.Context, namespace, name string) (*envv1alpha1.Stack, error) {
	stack := &envv1alpha1.Stack{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, stack); err != nil {
		return nil, err
	}
	return stack, nil
}

// ListStacks lists Stack resources in a namespace
func (c *Client) ListStacks(ctx context.Context, namespace string) (*envv1alpha1.StackList, error) {
	stackList := &envv1alpha1.StackList{}
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := c.List(ctx, stackList, opts...); err != nil {
		return nil, err
	}
	return stackList, nil
}

// UpdateStack updates a Stack resource
func (c *Client) UpdateStack(ctx context.Context, stack *envv1alpha1.Stack) error {
	return c.Update(ctx, stack)
}

// DeleteStack deletes a Stack resource
func (c *Client) DeleteStack(ctx context.Context, namespace, name string) error {
	stack := &envv1alpha1.Stack{}
	stack.Namespace = namespace
	stack.Name = name
	return c.Delete(ctx, stack)
}

// CreateBlueprint creates a Blueprint resource in the given namespace
func (c *Client) CreateBlueprint(ctx context.Context, blueprint *envv1alpha1.Blueprint) error {
	return c.Create(ctx, blueprint)
}

// GetBlueprint retrieves a Blueprint resource
func (c *Client) GetBlueprint(ctx context.Context, namespace, name string) (*envv1alpha1.Blueprint, error) {
	blueprint := &envv1alpha1.Blueprint{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, blueprint); err != nil {
		return nil, err
	}
	return blueprint, nil
}

// ListBlueprints lists Blueprint resources in a namespace
func (c *Client) ListBlueprints(ctx context.Context, namespace string) (*envv1alpha1.BlueprintList, error) {
	blueprintList := &envv1alpha1.BlueprintList{}
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := c.List(ctx, blueprintList, opts...); err != nil {
		return nil, err
	}
	return blueprintList, nil
}

// UpdateBlueprint updates a Blueprint resource
func (c *Client) UpdateBlueprint(ctx context.Context, blueprint *envv1alpha1.Blueprint) error {
	return c.Update(ctx, blueprint)
}

// DeleteBlueprint deletes a Blueprint resource
func (c *Client) DeleteBlueprint(ctx context.Context, namespace, name string) error {
	blueprint := &envv1alpha1.Blueprint{}
	blueprint.Namespace = namespace
	blueprint.Name = name
	return c.Delete(ctx, blueprint)
}

// CreateEnv creates an Env resource in the given namespace
func (c *Client) CreateEnv(ctx context.Context, env *envv1alpha1.Env) error {
	return c.Create(ctx, env)
}

// GetEnv retrieves an Env resource
func (c *Client) GetEnv(ctx context.Context, namespace, name string) (*envv1alpha1.Env, error) {
	env := &envv1alpha1.Env{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, env); err != nil {
		return nil, err
	}
	return env, nil
}

// ListEnvs lists Env resources in a namespace
func (c *Client) ListEnvs(ctx context.Context, namespace string) (*envv1alpha1.EnvList, error) {
	envList := &envv1alpha1.EnvList{}
	opts := []client.ListOption{}
	if namespace != "" {
		opts = append(opts, client.InNamespace(namespace))
	}
	if err := c.List(ctx, envList, opts...); err != nil {
		return nil, err
	}
	return envList, nil
}

// UpdateEnv updates an Env resource
func (c *Client) UpdateEnv(ctx context.Context, env *envv1alpha1.Env) error {
	return c.Update(ctx, env)
}

// DeleteEnv deletes an Env resource
func (c *Client) DeleteEnv(ctx context.Context, namespace, name string) error {
	env := &envv1alpha1.Env{}
	env.Namespace = namespace
	env.Name = name
	return c.Delete(ctx, env)
}

// CreateConfigMap creates a ConfigMap resource
func (c *Client) CreateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) error {
	return c.Create(ctx, configMap)
}

// GetConfigMap retrieves a ConfigMap resource
func (c *Client) GetConfigMap(ctx context.Context, namespace, name string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, configMap); err != nil {
		return nil, err
	}
	return configMap, nil
}

// UpdateConfigMap updates a ConfigMap resource
func (c *Client) UpdateConfigMap(ctx context.Context, configMap *corev1.ConfigMap) error {
	return c.Update(ctx, configMap)
}

// DeleteConfigMap deletes a ConfigMap resource
func (c *Client) DeleteConfigMap(ctx context.Context, namespace, name string) error {
	configMap := &corev1.ConfigMap{}
	configMap.Namespace = namespace
	configMap.Name = name
	return c.Delete(ctx, configMap)
}

// GetSecret retrieves a Secret resource
func (c *Client) GetSecret(ctx context.Context, namespace, name string) (*corev1.Secret, error) {
	secret := &corev1.Secret{}
	if err := c.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, secret); err != nil {
		return nil, err
	}
	return secret, nil
}

// CreateSecret creates a Secret resource
func (c *Client) CreateSecret(ctx context.Context, secret *corev1.Secret) error {
	return c.Create(ctx, secret)
}

// UpdateSecret updates a Secret resource
func (c *Client) UpdateSecret(ctx context.Context, secret *corev1.Secret) error {
	return c.Update(ctx, secret)
}
