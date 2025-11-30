package postprocessor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/lissto-dev/api/pkg/postprocessor"
)

var _ = Describe("StackLabelInjector", func() {
	var (
		injector  *postprocessor.StackLabelInjector
		stackName string
	)

	BeforeEach(func() {
		injector = postprocessor.NewStackLabelInjector()
		stackName = "test-stack-123"
	})

	Describe("InjectLabels", func() {
		Context("with Deployment", func() {
			It("should inject lissto.dev/stack label to pod template", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "web",
								},
							},
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

				objects := []runtime.Object{deployment}
				result := injector.InjectLabels(objects, stackName)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))
				Expect(updatedDeployment.Spec.Template.Labels).To(HaveKeyWithValue("app", "web"))
			})

			It("should create labels map if it doesn't exist", func() {
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

				objects := []runtime.Object{deployment}
				result := injector.InjectLabels(objects, stackName)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))
			})
		})

		Context("with StatefulSet", func() {
			It("should inject lissto.dev/stack label to pod template", func() {
				statefulset := &appsv1.StatefulSet{
					ObjectMeta: metav1.ObjectMeta{
						Name: "database",
					},
					Spec: appsv1.StatefulSetSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "database",
								},
							},
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

				objects := []runtime.Object{statefulset}
				result := injector.InjectLabels(objects, stackName)

				updatedStatefulSet := result[0].(*appsv1.StatefulSet)
				Expect(updatedStatefulSet.Spec.Template.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))
				Expect(updatedStatefulSet.Spec.Template.Labels).To(HaveKeyWithValue("app", "database"))
			})
		})

		Context("with Pod", func() {
			It("should inject lissto.dev/stack label to pod metadata", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "migrate",
						Labels: map[string]string{
							"io.kompose.service": "migrate",
						},
					},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{
							{
								Name:  "migrate",
								Image: "migrate:latest",
							},
						},
					},
				}

				objects := []runtime.Object{pod}
				result := injector.InjectLabels(objects, stackName)

				updatedPod := result[0].(*corev1.Pod)
				Expect(updatedPod.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))
				Expect(updatedPod.Labels).To(HaveKeyWithValue("io.kompose.service", "migrate"))
			})

			It("should create labels map if it doesn't exist", func() {
				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "migrate",
					},
					Spec: corev1.PodSpec{
						RestartPolicy: corev1.RestartPolicyNever,
						Containers: []corev1.Container{
							{
								Name:  "migrate",
								Image: "migrate:latest",
							},
						},
					},
				}

				objects := []runtime.Object{pod}
				result := injector.InjectLabels(objects, stackName)

				updatedPod := result[0].(*corev1.Pod)
				Expect(updatedPod.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))
			})
		})

		Context("with multiple resources", func() {
			It("should inject labels to all supported resources", func() {
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

				pod := &corev1.Pod{
					ObjectMeta: metav1.ObjectMeta{
						Name: "migrate",
					},
					Spec: corev1.PodSpec{
						Containers: []corev1.Container{
							{
								Name:  "migrate",
								Image: "migrate:latest",
							},
						},
					},
				}

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

				objects := []runtime.Object{deployment, pod, statefulset}
				result := injector.InjectLabels(objects, stackName)

				Expect(result).To(HaveLen(3))

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))

				updatedPod := result[1].(*corev1.Pod)
				Expect(updatedPod.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))

				updatedStatefulSet := result[2].(*appsv1.StatefulSet)
				Expect(updatedStatefulSet.Spec.Template.Labels).To(HaveKeyWithValue("lissto.dev/stack", stackName))
			})
		})

		Context("with empty stack name", func() {
			It("should not modify resources", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: appsv1.DeploymentSpec{
						Template: corev1.PodTemplateSpec{
							ObjectMeta: metav1.ObjectMeta{
								Labels: map[string]string{
									"app": "web",
								},
							},
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

				objects := []runtime.Object{deployment}
				result := injector.InjectLabels(objects, "")

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Spec.Template.Labels).NotTo(HaveKey("lissto.dev/stack"))
				Expect(updatedDeployment.Spec.Template.Labels).To(HaveKeyWithValue("app", "web"))
			})
		})

		Context("with unsupported resource types", func() {
			It("should not modify Service resources", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
					Spec: corev1.ServiceSpec{
						Ports: []corev1.ServicePort{
							{
								Port: 80,
							},
						},
					},
				}

				objects := []runtime.Object{service}
				result := injector.InjectLabels(objects, stackName)

				updatedService := result[0].(*corev1.Service)
				Expect(updatedService.Labels).NotTo(HaveKey("lissto.dev/stack"))
			})
		})
	})
})
