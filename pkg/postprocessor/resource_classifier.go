package postprocessor

import (
	appsv1 "k8s.io/api/apps/v1"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

const (
	// ResourceClassAnnotation is the annotation key for resource classification
	ResourceClassAnnotation = "lissto.dev/class"

	// ResourceClassState marks resources preserved during suspension (PVCs, Ingress)
	ResourceClassState = "state"

	// ResourceClassWorkload marks resources deleted during suspension (everything else)
	ResourceClassWorkload = "workload"
)

// ResourceClassifier injects lissto.dev/class annotation on all Kubernetes resources
// This annotation is required by the controller to determine which resources to preserve
// during stack suspension and which to delete.
type ResourceClassifier struct{}

func NewResourceClassifier() *ResourceClassifier {
	return &ResourceClassifier{}
}

// InjectClassAnnotations adds lissto.dev/class annotation to all resources
// Classification rules:
//   - state: PersistentVolumeClaim, Ingress (preserved during suspension)
//   - workload: Everything else (deleted during suspension)
func (r *ResourceClassifier) InjectClassAnnotations(objects []runtime.Object) []runtime.Object {
	for i, obj := range objects {
		class := r.getResourceClass(obj)
		r.setClassAnnotation(obj, class)
		objects[i] = obj
	}
	return objects
}

// getResourceClass determines the classification for a given resource type
func (r *ResourceClassifier) getResourceClass(obj runtime.Object) string {
	switch obj.(type) {
	case *corev1.PersistentVolumeClaim, *networkingv1.Ingress:
		return ResourceClassState
	default:
		return ResourceClassWorkload
	}
}

// setClassAnnotation sets the lissto.dev/class annotation on a resource
func (r *ResourceClassifier) setClassAnnotation(obj runtime.Object, class string) {
	// Try to access annotations via metav1.Object interface
	if metaObj, ok := obj.(interface {
		GetAnnotations() map[string]string
		SetAnnotations(map[string]string)
	}); ok {
		annotations := metaObj.GetAnnotations()
		if annotations == nil {
			annotations = make(map[string]string)
		}
		annotations[ResourceClassAnnotation] = class
		metaObj.SetAnnotations(annotations)
	}
}
