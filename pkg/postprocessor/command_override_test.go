package postprocessor_test

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/lissto-dev/api/pkg/logging"
	"github.com/lissto-dev/api/pkg/postprocessor"
)

func TestCommandOverride(t *testing.T) {
	// Initialize logger for tests
	logging.InitLogger("info", "console")
	
	RegisterFailHandler(Fail)
	RunSpecs(t, "CommandOverride Postprocessor Suite")
}

var _ = Describe("CommandOverrider", func() {
	var (
		overrider *postprocessor.CommandOverrider
	)

	BeforeEach(func() {
		overrider = postprocessor.NewCommandOverrider()
	})

	Describe("OverrideCommands", func() {
		Context("with lissto.dev/command label (space-separated format)", func() {
			It("should preserve Kubernetes env var syntax $(VAR)", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "test-pod",
										Image: "kubectl",
										Args:  []string{"get", "pod/"},  // Broken by Kompose
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"test-pod": {
						"lissto.dev/command": "get pod/$(POD_NAMESPACE)",
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"get", "pod/$(POD_NAMESPACE)"}))
			})
		})

		Context("with lissto.dev/command label (JSON format)", func() {
			It("should preserve Kubernetes env var syntax $(VAR) in JSON array", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "test-pod",
										Image: "kubectl",
										Args:  []string{"get", "pod/"},
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"test-pod": {
						"lissto.dev/command": `["get", "pod/$(POD_NAMESPACE)"]`,
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"get", "pod/$(POD_NAMESPACE)"}))
			})

			It("should handle multiple env vars in JSON array", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "logger",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "logger",
										Image: "busybox",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"logger": {
						"lissto.dev/command": `["echo", "Pod: $(POD_NAME) in $(POD_NAMESPACE)"]`,
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"echo", "Pod: $(POD_NAME) in $(POD_NAMESPACE)"}))
			})
		})

		Context("with lissto.dev/entrypoint label", func() {
			It("should override container command (K8s command = Docker entrypoint)", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"web": {
						"lissto.dev/entrypoint": `["sh", "-c"]`,
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"sh", "-c"}))
			})

			It("should handle space-separated format", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"web": {
						"lissto.dev/entrypoint": "sh -c",
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"sh", "-c"}))
			})
		})

		Context("with both entrypoint and command labels", func() {
			It("should override both command and args", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "worker",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "worker",
										Image: "celery",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"worker": {
						"lissto.dev/entrypoint": `["python"]`,
						"lissto.dev/command":    `["-m", "celery", "worker", "-l", "debug"]`,
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Command).To(Equal([]string{"python"}))
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"-m", "celery", "worker", "-l", "debug"}))
			})
		})

		Context("with StatefulSet", func() {
			It("should override commands in StatefulSet", func() {
				statefulset := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "database",
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "database",
										Image: "postgres",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"database": {
						"lissto.dev/command": "postgres -c log_statement=all",
					},
				}

				objects := []runtime.Object{statefulset}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedStatefulSet := result[0].(*appsv1.StatefulSet)
				Expect(updatedStatefulSet.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"postgres", "-c", "log_statement=all"}))
			})
		})

		Context("with Pod", func() {
			It("should override commands in Pod by name", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-pod",
								Image: "kubectl",
								Args:  []string{"get", "pod/"},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"test-pod": {
						"lissto.dev/command": "get pod/$(POD_NAMESPACE)",
					},
				}

				objects := []runtime.Object{pod}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedPod := result[0].(*corev1.Pod)
				Expect(updatedPod.Spec.Containers[0].Args).To(Equal([]string{"get", "pod/$(POD_NAMESPACE)"}))
			})

			It("should match Pod by io.kompose.service label", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "test-pod-xyz123",
						Labels: map[string]string{
							"io.kompose.service": "test-pod",
						},
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "test-pod",
								Image: "kubectl",
								Args:  []string{"get", "pod/"},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"test-pod": {
						"lissto.dev/command": "get pod/$(POD_NAMESPACE)",
					},
				}

				objects := []runtime.Object{pod}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedPod := result[0].(*corev1.Pod)
				Expect(updatedPod.Spec.Containers[0].Args).To(Equal([]string{"get", "pod/$(POD_NAMESPACE)"}))
			})
		})

		Context("with no matching labels", func() {
			It("should not modify containers", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
										Args:  []string{"original", "args"},
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"other-service": {
						"lissto.dev/command": "should not apply",
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"original", "args"}))
			})
		})

		Context("with empty serviceLabelMap", func() {
			It("should return objects unchanged", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{}
				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				Expect(result).To(HaveLen(1))
				Expect(result[0]).To(Equal(deployment))
			})
		})

		Context("with empty label values", func() {
			It("should not override when label is empty string", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
										Args:  []string{"original"},
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"web": {
						"lissto.dev/command": "",
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"original"}))
			})
		})

		Context("with multiple whitespace in space-separated format", func() {
			It("should handle multiple spaces correctly", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"web": {
						"lissto.dev/command": "python    app.py    --debug",  // Multiple spaces
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"python", "app.py", "--debug"}))
			})
		})

		Context("format detection", func() {
			It("should detect JSON array format and not split by spaces", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"web": {
						// Arguments with spaces - JSON array preserves them
						"lissto.dev/command": `["echo", "Hello World", "from container"]`,
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"echo", "Hello World", "from container"}))
			})

			It("should fall back to space-separated when JSON is invalid", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							Spec: corev1.PodSpec{
								Containers: []corev1.Container{
									{
										Name:  "web",
										Image: "nginx",
									},
								},
							},
						},
					},
				}

				serviceLabelMap := map[string]map[string]string{
					"web": {
						"lissto.dev/command": `[invalid json but works as space-separated]`,
					},
				}

				objects := []runtime.Object{deployment}
				result := overrider.OverrideCommands(objects, serviceLabelMap)

				updatedDeployment := result[0].(*appsv1.Deployment)
				// Invalid JSON falls back to space-separated
				Expect(updatedDeployment.Spec.Template.Spec.Containers[0].Args).To(Equal([]string{"[invalid", "json", "but", "works", "as", "space-separated]"}))
			})
		})
	})
})

