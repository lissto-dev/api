package blueprint_test

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/lissto-dev/api/internal/api/blueprint"
	"github.com/lissto-dev/api/pkg/authz"
	envv1alpha1 "github.com/lissto-dev/controller/api/v1alpha1"
	operatorConfig "github.com/lissto-dev/controller/pkg/config"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ = Describe("FormattableBlueprint", func() {
	Describe("ToDetailed", func() {
		Context("with global namespace", func() {
			It("should normalize global namespace and expose all metadata", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				createdAt := time.Now()
				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "test-blueprint",
						Namespace: "lissto-global",
						Labels: map[string]string{
							"hash":   "abc123",
							"branch": "main",
						},
						Annotations: map[string]string{
							"lissto.dev/title":      "Test Blueprint",
							"lissto.dev/repository": "https://github.com/test/repo",
							"lissto.dev/services":   `{"services":["web","api"],"infra":["db"]}`,
						},
						CreationTimestamp: metav1.Time{Time: createdAt},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'\nservices:\n  web:\n    image: nginx",
						Hash:          "abc12345",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				Expect(err).ToNot(HaveOccurred())
				Expect(detailed.Metadata.Name).To(Equal("test-blueprint"))
				Expect(detailed.Metadata.Namespace).To(Equal("global"))
				Expect(detailed.Metadata.Labels["hash"]).To(Equal("abc123"))
				Expect(detailed.Metadata.Labels["branch"]).To(Equal("main"))
				Expect(detailed.Metadata.Annotations["lissto.dev/title"]).To(Equal("Test Blueprint"))
				Expect(detailed.Metadata.Annotations["lissto.dev/repository"]).To(Equal("https://github.com/test/repo"))
				Expect(detailed.Metadata.Annotations["lissto.dev/services"]).To(Equal(`{"services":["web","api"],"infra":["db"]}`))
				Expect(detailed.Metadata.CreatedAt).To(Equal(createdAt.Format(time.RFC3339)))
				Expect(detailed.Spec.(envv1alpha1.BlueprintSpec).DockerCompose).To(Equal("version: '3'\nservices:\n  web:\n    image: nginx"))
			})
		})

		Context("with developer namespace", func() {
			It("should normalize developer namespace correctly", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				createdAt := time.Now()
				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "dev-blueprint",
						Namespace: "lissto-daniel",
						Labels: map[string]string{
							"owner": "daniel",
						},
						Annotations: map[string]string{
							"lissto.dev/title": "Dev Blueprint",
						},
						CreationTimestamp: metav1.Time{Time: createdAt},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'\nservices:\n  app:\n    image: alpine",
						Hash:          "def67890",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				Expect(err).ToNot(HaveOccurred())
				Expect(detailed.Metadata.Name).To(Equal("dev-blueprint"))
				Expect(detailed.Metadata.Namespace).To(Equal("daniel"))
				Expect(detailed.Metadata.Labels["owner"]).To(Equal("daniel"))
				Expect(detailed.Metadata.Annotations["lissto.dev/title"]).To(Equal("Dev Blueprint"))
				Expect(detailed.Metadata.CreatedAt).To(Equal(createdAt.Format(time.RFC3339)))
			})
		})

		Context("with unknown namespace", func() {
			It("should return error for unknown namespace type", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "unknown-blueprint",
						Namespace:         "unknown-namespace",
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'",
						Hash:          "xyz",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				_, err := formattable.ToDetailed()

				Expect(err).To(HaveOccurred())
				Expect(err.Error()).To(ContainSubstring("unknown namespace type"))
			})
		})

		Context("with all annotations", func() {
			It("should expose all annotations including custom ones", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "full-annotations",
						Namespace: "lissto-global",
						Annotations: map[string]string{
							"lissto.dev/title":       "Full Test",
							"lissto.dev/repository":  "https://github.com/test/full",
							"lissto.dev/services":    `{"services":["a","b"]}`,
							"custom-annotation":      "custom-value",
							"another-annotation":     "another-value",
							"lissto.dev/extra-field": "extra",
						},
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'",
						Hash:          "full",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				Expect(err).ToNot(HaveOccurred())
				Expect(detailed.Metadata.Annotations).To(HaveLen(6))
				Expect(detailed.Metadata.Annotations["lissto.dev/title"]).To(Equal("Full Test"))
				Expect(detailed.Metadata.Annotations["lissto.dev/repository"]).To(Equal("https://github.com/test/full"))
				Expect(detailed.Metadata.Annotations["lissto.dev/services"]).To(Equal(`{"services":["a","b"]}`))
				Expect(detailed.Metadata.Annotations["custom-annotation"]).To(Equal("custom-value"))
				Expect(detailed.Metadata.Annotations["another-annotation"]).To(Equal("another-value"))
				Expect(detailed.Metadata.Annotations["lissto.dev/extra-field"]).To(Equal("extra"))
			})
		})

		Context("with all labels", func() {
			It("should expose all labels including custom ones", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:      "full-labels",
						Namespace: "lissto-global",
						Labels: map[string]string{
							"hash":         "abc",
							"branch":       "main",
							"environment":  "production",
							"custom-label": "custom-value",
						},
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'",
						Hash:          "labels",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				Expect(err).ToNot(HaveOccurred())
				Expect(detailed.Metadata.Labels).To(HaveLen(4))
				Expect(detailed.Metadata.Labels["hash"]).To(Equal("abc"))
				Expect(detailed.Metadata.Labels["branch"]).To(Equal("main"))
				Expect(detailed.Metadata.Labels["environment"]).To(Equal("production"))
				Expect(detailed.Metadata.Labels["custom-label"]).To(Equal("custom-value"))
			})
		})

		Context("with empty annotations and labels", func() {
			It("should handle nil metadata gracefully", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "empty-metadata",
						Namespace:         "lissto-global",
						Labels:            nil,
						Annotations:       nil,
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'",
						Hash:          "empty",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				Expect(err).ToNot(HaveOccurred())
				Expect(detailed.Metadata.Labels).To(BeNil())
				Expect(detailed.Metadata.Annotations).To(BeNil())
			})
		})

		Context("with RFC3339 timestamp formatting", func() {
			It("should format CreatedAt as RFC3339", func() {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				specificTime := time.Date(2024, 3, 15, 14, 30, 45, 0, time.UTC)
				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "time-test",
						Namespace:         "lissto-global",
						CreationTimestamp: metav1.Time{Time: specificTime},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'",
						Hash:          "time",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				Expect(err).ToNot(HaveOccurred())
				Expect(detailed.Metadata.CreatedAt).To(Equal("2024-03-15T14:30:45Z"))

				// Verify it's valid RFC3339
				_, parseErr := time.Parse(time.RFC3339, detailed.Metadata.CreatedAt)
				Expect(parseErr).ToNot(HaveOccurred())
			})
		})

		DescribeTable("namespace normalization integration tests",
			func(k8sNamespace, expectedNormalized string, shouldError bool) {
				config := &operatorConfig.Config{}
				config.Namespaces.Global = "lissto-global"
				config.Namespaces.DeveloperPrefix = "lissto-"

				nsManager := authz.NewNamespaceManager(config)

				bp := &envv1alpha1.Blueprint{
					ObjectMeta: metav1.ObjectMeta{
						Name:              "test",
						Namespace:         k8sNamespace,
						CreationTimestamp: metav1.Time{Time: time.Now()},
					},
					Spec: envv1alpha1.BlueprintSpec{
						DockerCompose: "version: '3'",
						Hash:          "test",
					},
				}

				formattable := &blueprint.FormattableBlueprint{
					K8sObj:    bp,
					NsManager: nsManager,
				}

				detailed, err := formattable.ToDetailed()

				if shouldError {
					Expect(err).To(HaveOccurred())
				} else {
					Expect(err).ToNot(HaveOccurred())
					Expect(detailed.Metadata.Namespace).To(Equal(expectedNormalized))
				}
			},
			Entry("global namespace", "lissto-global", "global", false),
			Entry("developer namespace - alice", "lissto-alice", "alice", false),
			Entry("developer namespace - bob", "lissto-bob", "bob", false),
			Entry("unknown namespace", "random-namespace", "", true),
			Entry("invalid developer namespace", "not-lissto-prefix", "", true),
		)
	})

	Describe("ToStandard", func() {
		It("should return BlueprintResponse with scoped ID", func() {
			config := &operatorConfig.Config{}
			config.Namespaces.Global = "lissto-global"
			config.Namespaces.DeveloperPrefix = "lissto-"

			nsManager := authz.NewNamespaceManager(config)

			bp := &envv1alpha1.Blueprint{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "standard-blueprint",
					Namespace: "lissto-global",
					Annotations: map[string]string{
						"lissto.dev/title":      "Standard Test",
						"lissto.dev/repository": "https://github.com/test/standard",
						"lissto.dev/services":   `{"services":["web"],"infra":[]}`,
					},
				},
				Spec: envv1alpha1.BlueprintSpec{
					DockerCompose: "version: '3'",
					Hash:          "std",
				},
			}

			formattable := &blueprint.FormattableBlueprint{
				K8sObj:    bp,
				NsManager: nsManager,
			}

			standard := formattable.ToStandard()

			response, ok := standard.(blueprint.BlueprintResponse)
			Expect(ok).To(BeTrue())
			Expect(response.ID).To(Equal("global/standard-blueprint"))
			Expect(response.Title).To(Equal("Standard Test"))
			Expect(response.Content.Services).To(Equal([]string{"web"}))
			Expect(response.Content.Infra).To(Equal([]string{}))
		})
	})
})
