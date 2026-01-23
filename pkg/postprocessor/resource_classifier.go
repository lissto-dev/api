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
		var annotations map[string]string
		var class string

		switch resource := obj.(type) {
		case *corev1.PersistentVolumeClaim:
			class = ResourceClassState
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *networkingv1.Ingress:
			class = ResourceClassState
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *appsv1.Deployment:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *appsv1.StatefulSet:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *appsv1.DaemonSet:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *batchv1.Job:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *batchv1.CronJob:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *corev1.Service:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *corev1.ConfigMap:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *corev1.Secret:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		case *corev1.Pod:
			class = ResourceClassWorkload
			if resource.Annotations == nil {
				resource.Annotations = make(map[string]string)
			}
			resource.Annotations[ResourceClassAnnotation] = class
			objects[i] = resource

		default:
			// For any unknown resource types, default to workload
			// This ensures all resources have the annotation
			if metaObj, ok := obj.(interface{ GetAnnotations() map[string]string }); ok {
				annotations = metaObj.GetAnnotations()
				if annotations == nil {
					annotations = make(map[string]string)
				}
				annotations[ResourceClassAnnotation] = ResourceClassWorkload
				if setter, ok := obj.(interface{ SetAnnotations(map[string]string) }); ok {
					setter.SetAnnotations(annotations)
					objects[i] = obj
				}
			}
		}
	}

	return objects
}
