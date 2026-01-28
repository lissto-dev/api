package postprocessor_test

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/lissto-dev/api/pkg/kompose"
	"github.com/lissto-dev/api/pkg/postprocessor"
)

var _ = Describe("ResourceClassifier", func() {
	var classifier *postprocessor.ResourceClassifier

	BeforeEach(func() {
		classifier = postprocessor.NewResourceClassifier()
	})

	Describe("InjectClassAnnotations", func() {
		Context("with state resources", func() {
			It("should classify PersistentVolumeClaim as state", func() {
				pvc := &corev1.PersistentVolumeClaim{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
				}

				objects := []runtime.Object{pvc}
				result := classifier.InjectClassAnnotations(objects)

				updatedPVC := result[0].(*corev1.PersistentVolumeClaim)
				Expect(updatedPVC.Annotations).To(HaveKeyWithValue(
					postprocessor.ResourceClassAnnotation,
					postprocessor.ResourceClassState,
				))
			})

			It("should classify Ingress as state", func() {
				ingress := &networkingv1.Ingress{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
				}

				objects := []runtime.Object{ingress}
				result := classifier.InjectClassAnnotations(objects)

				updatedIngress := result[0].(*networkingv1.Ingress)
				Expect(updatedIngress.Annotations).To(HaveKeyWithValue(
					postprocessor.ResourceClassAnnotation,
					postprocessor.ResourceClassState,
				))
			})
		})

		Context("with workload resources", func() {
			It("should classify Deployment as workload", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
				}

				objects := []runtime.Object{deployment}
				result := classifier.InjectClassAnnotations(objects)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue(
					postprocessor.ResourceClassAnnotation,
					postprocessor.ResourceClassWorkload,
				))
			})

			It("should classify Service as workload", func() {
				service := &corev1.Service{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
				}

				objects := []runtime.Object{service}
				result := classifier.InjectClassAnnotations(objects)

				updatedService := result[0].(*corev1.Service)
				Expect(updatedService.Annotations).To(HaveKeyWithValue(
					postprocessor.ResourceClassAnnotation,
					postprocessor.ResourceClassWorkload,
				))
			})
		})

		Context("with nil annotations", func() {
			It("should create annotations map", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
					},
				}
				Expect(deployment.Annotations).To(BeNil())

				objects := []runtime.Object{deployment}
				result := classifier.InjectClassAnnotations(objects)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Annotations).NotTo(BeNil())
				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue(
					postprocessor.ResourceClassAnnotation,
					postprocessor.ResourceClassWorkload,
				))
			})
		})

		Context("with existing annotations", func() {
			It("should preserve existing annotations", func() {
				deployment := &appsv1.Deployment{
					ObjectMeta: metav1.ObjectMeta{
						Name: "web",
						Annotations: map[string]string{
							"existing-key": "existing-value",
						},
					},
				}

				objects := []runtime.Object{deployment}
				result := classifier.InjectClassAnnotations(objects)

				updatedDeployment := result[0].(*appsv1.Deployment)
				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue("existing-key", "existing-value"))
				Expect(updatedDeployment.Annotations).To(HaveKeyWithValue(
					postprocessor.ResourceClassAnnotation,
					postprocessor.ResourceClassWorkload,
				))
			})
		})

		Context("with real Kompose output (integration)", func() {
			It("should classify all resources from Kompose conversion", func() {
				// Simple compose with web service and volume
				composeYAML := `
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
    volumes:
      - data:/var/www/html

volumes:
  data:
`
				converter := kompose.NewConverter("test-ns")
				objects, err := converter.ConvertToObjects(composeYAML)
				Expect(err).NotTo(HaveOccurred())
				Expect(len(objects)).To(BeNumerically(">", 0))

				// Apply the classifier
				result := classifier.InjectClassAnnotations(objects)

				// All resources should have the annotation
				for _, obj := range result {
					switch resource := obj.(type) {
					case *appsv1.Deployment:
						Expect(resource.Annotations).To(HaveKeyWithValue(
							postprocessor.ResourceClassAnnotation,
							postprocessor.ResourceClassWorkload,
						), "Deployment should have workload class")
					case *corev1.Service:
						Expect(resource.Annotations).To(HaveKeyWithValue(
							postprocessor.ResourceClassAnnotation,
							postprocessor.ResourceClassWorkload,
						), "Service should have workload class")
					case *corev1.PersistentVolumeClaim:
						Expect(resource.Annotations).To(HaveKeyWithValue(
							postprocessor.ResourceClassAnnotation,
							postprocessor.ResourceClassState,
						), "PVC should have state class")
					}
				}
			})

			It("should preserve annotations through YAML serialization", func() {
				// Simple compose with web service
				composeYAML := `
services:
  web:
    image: nginx:latest
    ports:
      - "80:80"
`
				converter := kompose.NewConverter("test-ns")
				objects, err := converter.ConvertToObjects(composeYAML)
				Expect(err).NotTo(HaveOccurred())

				// Apply the classifier
				result := classifier.InjectClassAnnotations(objects)

				// Serialize to YAML
				yamlOutput, err := converter.SerializeToYAML(result)
				Expect(err).NotTo(HaveOccurred())

				// Check YAML contains the annotation
				Expect(yamlOutput).To(ContainSubstring("lissto.dev/class"))
				GinkgoWriter.Printf("YAML output:\n%s\n", yamlOutput)
			})
		})
	})
})
