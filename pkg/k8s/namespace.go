package k8s

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EnsureNamespace creates namespace if it doesn't exist
func (c *Client) EnsureNamespace(ctx context.Context, name string) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
	}

	err := c.Create(ctx, ns)
	if err == nil {
		return nil // Successfully created
	}

	if errors.IsAlreadyExists(err) {
		return nil // Already exists, OK
	}

	return err // Real error
}
