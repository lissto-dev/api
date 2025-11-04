package postprocessor

import (
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// StackLabelInjector injects stack-related labels into Kubernetes resources
type StackLabelInjector struct{}

func NewStackLabelInjector() *StackLabelInjector {
	return &StackLabelInjector{}
}

// InjectLabels adds lissto.dev/stack label to pod templates in workload resources
func (s *StackLabelInjector) InjectLabels(objects []runtime.Object, stackName string) []runtime.Object {
	if stackName == "" {
		return objects
	}

	for i, obj := range objects {
		switch resource := obj.(type) {
		case *appsv1.Deployment:
			s.injectToPodTemplate(&resource.Spec.Template, stackName)
			objects[i] = resource

		case *appsv1.StatefulSet:
			s.injectToPodTemplate(&resource.Spec.Template, stackName)
			objects[i] = resource
		}
	}
	return objects
}

// injectToPodTemplate adds label to a pod template
func (s *StackLabelInjector) injectToPodTemplate(template *corev1.PodTemplateSpec, stackName string) {
	if template.Labels == nil {
		template.Labels = make(map[string]string)
	}
	template.Labels["lissto.dev/stack"] = stackName
}
