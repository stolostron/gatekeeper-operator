/*


Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package e2e

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	gkv1alpha1 "github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/pkg/wildcard"
	admregv1 "k8s.io/api/admissionregistration/v1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	extv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/yaml"

	"github.com/stolostron/gatekeeper-operator/api/v1alpha1"
	"github.com/stolostron/gatekeeper-operator/controllers"
	"github.com/stolostron/gatekeeper-operator/pkg/util"
	test "github.com/stolostron/gatekeeper-operator/test/e2e/util"
)

var _ = Describe("Gatekeeper", func() {
	const (
		gatekeeperWithAllValuesFile = "gatekeeper_with_all_values.yaml"
	)

	BeforeEach(func() {
		if !useExistingCluster() {
			Skip("Test requires existing cluster. Set environment variable USE_EXISTING_CLUSTER=true and try again.")
		}
	})

	JustAfterEach(func(ctx SpecContext) {
		if CurrentSpecReport().Failed() {
			DebugDump()
		}
	})

	AfterEach(func(ctx SpecContext) {
		By("Clean gatekeeper")
		_, err := test.KubectlWithOutput("delete", "gatekeeper", "gatekeeper", "--ignore-not-found")
		Expect(err).ShouldNot(HaveOccurred())

		// Once this succeeds, clean up has happened for all owned resources.
		Eventually(func() bool {
			err := K8sClient.Get(ctx, gatekeeperName, &v1alpha1.Gatekeeper{})
			if err == nil {
				return false
			}

			return apierrors.IsNotFound(err)
		}, deleteTimeout, pollInterval).Should(BeTrue(), "Gatekeeper "+gatekeeperName.Name+" should be deleted.")

		Eventually(func() bool {
			err := K8sClient.Get(ctx, auditName, &appsv1.Deployment{})
			if err == nil {
				return false
			}

			return apierrors.IsNotFound(err)
		}, deleteTimeout, pollInterval).Should(BeTrue(), "Deployment "+auditName.Name+" should be deleted.")

		Eventually(func() bool {
			err := K8sClient.Get(ctx, controllerManagerName, &appsv1.Deployment{})
			if err == nil {
				return false
			}

			return apierrors.IsNotFound(err)
		}, deleteTimeout, pollInterval).Should(BeTrue(), "Deployment "+controllerManagerName.Name+" should be deleted.")

		By("Clean Config", func() {
			_, _ = test.KubectlWithOutput("delete", "config", "config", "-n", gatekeeperNamespace, "--ignore-not-found")
			Eventually(func() bool {
				err := K8sClient.Get(ctx, types.NamespacedName{
					Name:      "config",
					Namespace: gatekeeperNamespace,
				}, &gkv1alpha1.Config{})
				if err == nil {
					return false
				}

				return apierrors.IsNotFound(err) || meta.IsNoMatchError(err)
			}, deleteTimeout, pollInterval).Should(BeTrue(), "Gatekeeper Config config should be deleted.")
		})
	})

	Describe("Test config resource", Ordered, func() {
		// The default exempt namespaces
		defaultNamespaces := []wildcard.Wildcard{
			"kube-*", "multicluster-engine",
			"hypershift", "hive", "rhacs-operator", "open-cluster-*", "openshift-*",
		}

		AfterEach(func(ctx SpecContext) {
			By("Clean gatekeeper")
			_, err := test.KubectlWithOutput("delete", "gatekeeper", "gatekeeper", "--ignore-not-found")
			Expect(err).ShouldNot(HaveOccurred())

			By("Wait for config to be deleted", func() {
				Eventually(func() bool {
					err := K8sClient.Get(ctx, types.NamespacedName{
						Name:      "config",
						Namespace: gatekeeperNamespace,
					}, &gkv1alpha1.Config{})
					if err == nil {
						return false
					}

					return apierrors.IsNotFound(err)
				}, deleteTimeout, pollInterval).Should(BeTrue())

				Consistently(func() bool {
					err := K8sClient.Get(ctx, types.NamespacedName{
						Name:      "config",
						Namespace: gatekeeperNamespace,
					}, &gkv1alpha1.Config{})
					if err == nil {
						return false
					}

					return apierrors.IsNotFound(err)
				}, 5, pollInterval).Should(BeTrue())
			})
		})

		It("Should update config when the gatekeeper.config.match is not nil", func(ctx SpecContext) {
			var originalNs wildcard.Wildcard = "mynamespace"

			gatekeeper := &v1alpha1.Gatekeeper{
				ObjectMeta: metav1.ObjectMeta{
					Name: gkName,
				},
				Spec: v1alpha1.GatekeeperSpec{
					Config: &v1alpha1.ConfigConfig{
						Matches: []gkv1alpha1.MatchEntry{
							{
								ExcludedNamespaces: []wildcard.Wildcard{
									originalNs,
								},
								Processes: []string{
									"audit", "webhook", "sync",
								},
							},
						},
					},
				},
			}

			By("Creating Gatekeeper resource", func() {
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			})

			config := &gkv1alpha1.Config{}

			By("The config include the default namespaces and matches")
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, types.NamespacedName{Namespace: gatekeeperNamespace, Name: "config"},
					config)
				g.Expect(err).ShouldNot(HaveOccurred())

				g.Expect(config.Spec.Match).Should(HaveLen(2))
				Expect(config.Spec.Match[0].ExcludedNamespaces).Should(ContainElements(defaultNamespaces))
				g.Expect(config.Spec.Match[1].ExcludedNamespaces[0]).Should(Equal(originalNs))
			}, 60, pollInterval).Should(Succeed())
		})

		It("Should not attach the default ns when the DisableDefaultMatches is true", func(ctx SpecContext) {
			disableDefaultMatches := true
			gatekeeper := &v1alpha1.Gatekeeper{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gkName,
					Namespace: gatekeeperNamespace,
				},
				Spec: v1alpha1.GatekeeperSpec{
					Config: &v1alpha1.ConfigConfig{
						Matches: []gkv1alpha1.MatchEntry{
							{
								ExcludedNamespaces: []wildcard.Wildcard{
									"cat-ns", "dog-ns",
								},
								Processes: []string{
									"audit", "webhook", "sync",
								},
							},
						},
						DisableDefaultMatches: disableDefaultMatches,
					},
				},
			}

			By("Creating Gatekeeper resource", func() {
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			})

			config := &gkv1alpha1.Config{}

			By("The config has only the gatekeeper.spec.config.matches")
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, types.NamespacedName{Namespace: gatekeeperNamespace, Name: "config"},
					config)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(config.Spec.Match[0]).Should(Equal(gatekeeper.Spec.Config.Matches[0]))
			}, 120, pollInterval).Should(Succeed())
		})

		It("Should keep config.spec.match when config is updated", func(ctx SpecContext) {
			gatekeeper := &v1alpha1.Gatekeeper{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gkName,
					Namespace: gatekeeperNamespace,
				},
				Spec: v1alpha1.GatekeeperSpec{
					Config: &v1alpha1.ConfigConfig{
						Matches: []gkv1alpha1.MatchEntry{
							{
								ExcludedNamespaces: []wildcard.Wildcard{
									"tiger-ns", "lion-ns",
								},
								Processes: []string{
									"audit", "webhook", "sync",
								},
							},
						},
					},
				},
			}

			By("Creating Gatekeeper resource", func() {
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			})

			By("Getting gatekeeper")
			Eventually(func() error {
				return K8sClient.Get(ctx, types.NamespacedName{Name: "gatekeeper"}, gatekeeper)
			}, timeout, pollInterval).Should(Succeed())

			By("Getting Config")
			config := &gkv1alpha1.Config{}
			Eventually(func(g Gomega) []gkv1alpha1.MatchEntry {
				g.Expect(
					K8sClient.Get(ctx, types.NamespacedName{Namespace: gatekeeperNamespace, Name: "config"}, config),
				).Should(Succeed())

				return config.Spec.Match
			}, 150, 5).Should(HaveLen(2))

			By("Apply Config with 'shouldnotexist' namespace")
			config.Spec.Match = []gkv1alpha1.MatchEntry{
				{
					ExcludedNamespaces: []wildcard.Wildcard{
						"shouldnotexist",
					},
					Processes: []string{
						"webhook", "sync",
					},
				},
			}
			Expect(K8sClient.Update(ctx, config)).Should(Succeed())

			By("The config has the default exempt namespaces and gatekeeper.config.matches")
			Eventually(func(g Gomega) []gkv1alpha1.MatchEntry {
				g.Expect(
					K8sClient.Get(ctx, types.NamespacedName{Namespace: gatekeeperNamespace, Name: "config"}, config),
				).Should(Succeed())

				return config.Spec.Match
			}, 150, 5).Should(HaveLen(2))

			Expect(config.Spec.Match[0].ExcludedNamespaces).Should(ContainElements(defaultNamespaces))
			Expect(config.Spec.Match[1]).Should(BeComparableTo(gatekeeper.Spec.Config.Matches[0]))

			By("The config should not include 'shouldnotexist' namespace")
			Expect(config.Spec.Match[0].ExcludedNamespaces).ShouldNot(ContainElement("shouldnotexist"))
			Expect(config.Spec.Match[1].ExcludedNamespaces).ShouldNot(ContainElement("shouldnotexist"))
		})

		It("Should update the config after gatekeeper is updated", func(ctx SpecContext) {
			gatekeeper := &v1alpha1.Gatekeeper{
				ObjectMeta: metav1.ObjectMeta{
					Name:      gkName,
					Namespace: gatekeeperNamespace,
				},
				Spec: v1alpha1.GatekeeperSpec{
					Config: &v1alpha1.ConfigConfig{
						Matches: []gkv1alpha1.MatchEntry{
							{
								ExcludedNamespaces: []wildcard.Wildcard{
									"cat-ns", "dog-ns",
								},
								Processes: []string{
									"audit", "webhook", "sync",
								},
							},
						},
						DisableDefaultMatches: false,
					},
				},
			}

			By("Creating Gatekeeper resource", func() {
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			})

			gatekeeper = &v1alpha1.Gatekeeper{}

			By("The config should be updated")
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, types.NamespacedName{Namespace: gatekeeperNamespace, Name: "gatekeeper"},
					gatekeeper)
				g.Expect(err).ShouldNot(HaveOccurred())

				gatekeeper.Spec = v1alpha1.GatekeeperSpec{
					Config: &v1alpha1.ConfigConfig{
						Matches: []gkv1alpha1.MatchEntry{
							{
								ExcludedNamespaces: []wildcard.Wildcard{
									"cat-ns", "dog-ns",
								},
								Processes: []string{
									"audit", "webhook", "sync",
								},
							},
						},
						DisableDefaultMatches: true,
					},
				}

				err = K8sClient.Update(ctx, gatekeeper)
				g.Expect(err).ShouldNot(HaveOccurred())

				config := &gkv1alpha1.Config{}
				err = K8sClient.Get(ctx, types.NamespacedName{Namespace: gatekeeperNamespace, Name: "config"},
					config)
				g.Expect(err).ShouldNot(HaveOccurred())

				// Disable adding default namespaces
				g.Expect(config.Spec.Match).Should(HaveLen(1))
				g.Expect(config.Spec.Match[0]).Should(BeComparableTo(gatekeeper.Spec.Config.Matches[0]))
			}, 120, pollInterval).Should(Succeed())
		})
	})
	Describe("Overriding CR", Ordered, func() {
		It("Creating an empty gatekeeper contains default values", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			err := loadGatekeeperFromFile(gatekeeper, "gatekeeper_empty.yaml")
			Expect(err).ToNot(HaveOccurred())

			By("Creating Gatekeeper resource", func() {
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			})

			auditDeployment, webhookDeployment := gatekeeperDeployments(ctx)
			auditTemplate := auditDeployment.Spec.Template
			auditContainer := auditTemplate.Spec.Containers[0]
			webhookTemplate := webhookDeployment.Spec.Template
			webhookContainer := webhookTemplate.Spec.Containers[0]

			By("Checking the first element of deployment.volume is about certification", func() {
				Expect(auditDeployment.Spec.Template.Spec.Volumes[0].Secret.SecretName).NotTo(Equal(""))
				Expect(webhookDeployment.Spec.Template.Spec.Volumes[0].Secret.SecretName).NotTo(Equal(""))
			})

			By("Checking default replicas", func() {
				Expect(auditDeployment.Spec.Replicas).NotTo(BeNil())
				Expect(*auditDeployment.Spec.Replicas).To(Equal(test.DefaultDeployment.AuditReplicas))
				Expect(webhookDeployment.Spec.Replicas).NotTo(BeNil())
				Expect(*webhookDeployment.Spec.Replicas).To(Equal(test.DefaultDeployment.WebhookReplicas))
			})

			By("Checking gatekeeper-controller-manager readiness", func() {
				gkDeployment := &appsv1.Deployment{}
				Eventually(func() (int32, error) {
					return getDeploymentReadyReplicas(ctx, controllerManagerName, gkDeployment)
				}, timeout, pollInterval).Should(Equal(test.DefaultDeployment.WebhookReplicas))
			})

			By("Checking gatekeeper-audit readiness", func() {
				gkDeployment := &appsv1.Deployment{}
				Eventually(func() (int32, error) {
					return getDeploymentReadyReplicas(ctx, auditName, gkDeployment)
				}, timeout, pollInterval).Should(Equal(test.DefaultDeployment.AuditReplicas))
			})

			By("Checking validatingWebhookConfiguration is deployed", func() {
				validatingWebhookConfiguration := &admregv1.ValidatingWebhookConfiguration{}
				Eventually(func() error {
					return K8sClient.Get(ctx, validatingWebhookName, validatingWebhookConfiguration)
				}, timeout, pollInterval).ShouldNot(HaveOccurred())
				Expect(validatingWebhookConfiguration.OwnerReferences).To(HaveLen(1))
				Expect(validatingWebhookConfiguration.OwnerReferences[0].Kind).To(Equal("Gatekeeper"))
				Expect(validatingWebhookConfiguration.OwnerReferences[0].Name).To(Equal(gkName))
			})

			By("Checking mutatingWebhookConfiguration is deployed", func() {
				mutatingWebhookConfiguration := &admregv1.MutatingWebhookConfiguration{}
				Eventually(func() error {
					return K8sClient.Get(ctx, mutatingWebhookName, mutatingWebhookConfiguration)
				}, timeout, pollInterval).ShouldNot(HaveOccurred())
				Expect(mutatingWebhookConfiguration.OwnerReferences).To(HaveLen(1))
				Expect(mutatingWebhookConfiguration.OwnerReferences[0].Kind).To(Equal("Gatekeeper"))
				Expect(mutatingWebhookConfiguration.OwnerReferences[0].Name).To(Equal(gkName))
			})

			By("Checking default pod affinity", func() {
				Expect(auditTemplate.Spec.Affinity).To(BeNil())
				Expect(webhookTemplate.Spec.Affinity).To(BeEquivalentTo(test.DefaultDeployment.Affinity))
			})

			By("Checking default node selector", func() {
				Expect(auditTemplate.Spec.NodeSelector).To(BeEquivalentTo(test.DefaultDeployment.NodeSelector))
				Expect(webhookTemplate.Spec.NodeSelector).To(BeEquivalentTo(test.DefaultDeployment.NodeSelector))
			})

			By("Checking default pod annotations", func() {
				Expect(auditTemplate.Annotations).To(BeEquivalentTo(test.DefaultDeployment.PodAnnotations))
				Expect(webhookTemplate.Annotations).To(BeEquivalentTo(test.DefaultDeployment.PodAnnotations))
			})

			By("Checking default tolerations", func() {
				Expect(auditTemplate.Spec.Tolerations).To(BeNil())
				Expect(webhookTemplate.Spec.Tolerations).To(BeNil())
			})

			By("Checking default resource limits and requests", func() {
				assertResources(*test.DefaultDeployment.AuditResources, auditContainer.Resources)
				assertResources(*test.DefaultDeployment.WebResources, webhookContainer.Resources)
			})

			By("Checking default image", func() {
				auditImage, auditImagePullPolicy, err := getDefaultImage(controllers.AuditFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(auditContainer.Image).To(Equal(auditImage))
				Expect(auditContainer.ImagePullPolicy).To(Equal(auditImagePullPolicy))
				webhookImage, webhookImagePullPolicy, err := getDefaultImage(controllers.WebhookFile)
				Expect(err).NotTo(HaveOccurred())
				Expect(webhookContainer.Image).To(Equal(webhookImage))
				Expect(webhookContainer.ImagePullPolicy).To(Equal(webhookImagePullPolicy))
			})

			byCheckingFailurePolicy(ctx, &validatingWebhookName, "default",
				util.ValidatingWebhookConfigurationKind,
				controllers.ValidationGatekeeperWebhook,
				&test.DefaultDeployment.FailurePolicy)

			byCheckingNamespaceSelector(ctx, &validatingWebhookName, "default",
				util.ValidatingWebhookConfigurationKind,
				controllers.ValidationGatekeeperWebhook,
				test.DefaultDeployment.NamespaceSelector)

			byCheckingFailurePolicy(ctx, &mutatingWebhookName, "default",
				util.MutatingWebhookConfigurationKind,
				controllers.MutationGatekeeperWebhook,
				&test.DefaultDeployment.FailurePolicy)

			byCheckingNamespaceSelector(ctx, &mutatingWebhookName, "default",
				util.MutatingWebhookConfigurationKind,
				controllers.MutationGatekeeperWebhook,
				test.DefaultDeployment.NamespaceSelector)

			By("Checking default audit interval", func() {
				_, found := getContainerArg(auditContainer.Args, controllers.AuditIntervalArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default audit log level", func() {
				_, found := getContainerArg(auditContainer.Args, controllers.LogLevelArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default audit constraint violation limit", func() {
				_, found := getContainerArg(auditContainer.Args, controllers.ConstraintViolationLimitArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default audit chunk size", func() {
				_, found := getContainerArg(auditContainer.Args, controllers.AuditChunkSizeArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default audit from cache", func() {
				_, found := getContainerArg(auditContainer.Args, controllers.AuditFromCacheArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default emit audit events", func() {
				_, found := getContainerArg(auditContainer.Args, controllers.EmitAuditEventsArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default emit admission events", func() {
				_, found := getContainerArg(webhookContainer.Args, controllers.EmitAdmissionEventsArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default webhook log level", func() {
				_, found := getContainerArg(webhookContainer.Args, controllers.LogLevelArg)
				Expect(found).To(BeFalse())
			})

			By("Checking default disabled builtins", func() {
				_, found := getContainerArg(webhookContainer.Args, controllers.DisabledBuiltinArg)
				Expect(found).To(BeTrue())
			})

			By("Checking default logDenies", func() {
				_, found := getContainerArg(webhookContainer.Args, controllers.LogDenies)
				Expect(found).To(BeFalse())
			})
		})

		It("Contains the configured values", func(ctx SpecContext) {
			gatekeeper := &v1alpha1.Gatekeeper{}
			gatekeeper.Namespace = gatekeeperNamespace
			err := loadGatekeeperFromFile(gatekeeper, gatekeeperWithAllValuesFile)
			Expect(err).ToNot(HaveOccurred())
			Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			auditDeployment, webhookDeployment := gatekeeperDeployments(ctx)
			auditTemplate := auditDeployment.Spec.Template
			auditContainer := auditTemplate.Spec.Containers[0]
			webhookTemplate := webhookDeployment.Spec.Template
			webhookContainer := webhookTemplate.Spec.Containers[0]

			By("Checking expected replicas", func() {
				Expect(auditDeployment.Spec.Replicas).NotTo(BeNil())
				Expect(auditDeployment.Spec.Replicas).To(Equal(gatekeeper.Spec.Audit.Replicas))
				Expect(webhookDeployment.Spec.Replicas).NotTo(BeNil())
				Expect(webhookDeployment.Spec.Replicas).To(Equal(gatekeeper.Spec.Webhook.Replicas))
			})

			By("Checking expected pod affinity", func() {
				Expect(auditTemplate.Spec.Affinity).To(BeEquivalentTo(gatekeeper.Spec.Affinity))
				Expect(webhookTemplate.Spec.Affinity).To(BeEquivalentTo(gatekeeper.Spec.Affinity))
			})

			By("Checking expected node selector", func() {
				Expect(auditTemplate.Spec.NodeSelector).To(BeEquivalentTo(gatekeeper.Spec.NodeSelector))
				Expect(webhookTemplate.Spec.NodeSelector).To(BeEquivalentTo(gatekeeper.Spec.NodeSelector))
			})

			By("Checking expected pod annotations", func() {
				Expect(auditTemplate.Annotations).To(BeEquivalentTo(gatekeeper.Spec.PodAnnotations))
				Expect(webhookTemplate.Annotations).To(BeEquivalentTo(gatekeeper.Spec.PodAnnotations))
			})

			By("Checking expected tolerations", func() {
				Expect(auditTemplate.Spec.Tolerations).To(BeEquivalentTo(gatekeeper.Spec.Tolerations))
				Expect(webhookTemplate.Spec.Tolerations).To(BeEquivalentTo(gatekeeper.Spec.Tolerations))
			})

			By("Checking expected resource limits and requests", func() {
				assertResources(*gatekeeper.Spec.Audit.Resources, auditContainer.Resources)
				assertResources(*gatekeeper.Spec.Webhook.Resources, webhookContainer.Resources)
			})

			By("Checking expected image", func() {
				Expect(auditContainer.Image).ToNot(Equal(*gatekeeper.Spec.Image.Image)) //nolint:staticcheck
				Expect(auditContainer.ImagePullPolicy).To(Equal(*gatekeeper.Spec.Image.ImagePullPolicy))
				Expect(webhookContainer.Image).ToNot(Equal(*gatekeeper.Spec.Image.Image)) //nolint:staticcheck
				Expect(webhookContainer.ImagePullPolicy).To(Equal(*gatekeeper.Spec.Image.ImagePullPolicy))
			})

			By("Checking ready replicas", func() {
				gkDeployment := &appsv1.Deployment{}
				Eventually(func() (int32, error) {
					return getDeploymentReadyReplicas(ctx, controllerManagerName, gkDeployment)
				}, timeout, pollInterval).Should(Equal(*gatekeeper.Spec.Webhook.Replicas))
			})

			By("Checking webhook is available", func() {
				byCheckingValidation(ctx, v1alpha1.Enabled)
			})

			byCheckingFailurePolicy(ctx, &validatingWebhookName, "expected",
				util.ValidatingWebhookConfigurationKind,
				controllers.ValidationGatekeeperWebhook,
				gatekeeper.Spec.Webhook.FailurePolicy)

			byCheckingNamespaceSelector(ctx, &validatingWebhookName, "expected",
				util.ValidatingWebhookConfigurationKind,
				controllers.ValidationGatekeeperWebhook,
				gatekeeper.Spec.Webhook.NamespaceSelector)

			By("Checking expected audit interval", func() {
				value, found := getContainerArg(auditContainer.Args, controllers.AuditIntervalArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.AuditIntervalArg, "10")))
			})

			By("Checking expected audit log level", func() {
				value, found := getContainerArg(auditContainer.Args, controllers.LogLevelArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.LogLevelArg, "DEBUG")))
			})

			By("Checking expected audit constraint violation limit", func() {
				value, found := getContainerArg(auditContainer.Args, controllers.ConstraintViolationLimitArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.ConstraintViolationLimitArg, "55")))
			})

			By("Checking expected audit chunk size", func() {
				value, found := getContainerArg(auditContainer.Args, controllers.AuditChunkSizeArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.AuditChunkSizeArg, "66")))
			})

			By("Checking expected audit from cache", func() {
				value, found := getContainerArg(auditContainer.Args, controllers.AuditFromCacheArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.AuditFromCacheArg, "true")))
			})

			By("Checking expected emit audit events", func() {
				value, found := getContainerArg(auditContainer.Args, controllers.EmitAuditEventsArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.EmitAuditEventsArg, "true")))
			})

			By("Checking expected emit admission events", func() {
				value, found := getContainerArg(webhookContainer.Args, controllers.EmitAdmissionEventsArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.EmitAdmissionEventsArg, "true")))
			})

			By("Checking expected webhook log level", func() {
				value, found := getContainerArg(webhookContainer.Args, controllers.LogLevelArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.LogLevelArg, "ERROR")))
			})

			By("Checking expected disabled builtins", func() {
				value, found := getContainerArg(webhookContainer.Args, controllers.DisabledBuiltinArg)
				Expect(found).To(BeTrue())
				Expect(value).To(Equal(util.ToArg(controllers.DisabledBuiltinArg, "{http.send}")))
			})

			By("Checking default logDenies", func() {
				_, found := getContainerArg(webhookContainer.Args, controllers.LogDenies)
				Expect(found).To(BeTrue())
			})
		})

		It("Disables the ValidatingWebhookConfiguration", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			By("Create Gatekeeper CR with validation disabled", func() {
				webhookMode := v1alpha1.Disabled
				gatekeeper.Spec.ValidatingWebhook = &webhookMode
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
			})

			gatekeeperDeployments(ctx)
			byCheckingValidation(ctx, v1alpha1.Disabled)
		})

		It("Enables then disables the ValidatingWebhookConfiguration", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			By("First creating Gatekeeper CR with validation enabled", func() {
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
				gatekeeperDeployments(ctx)
				byCheckingValidation(ctx, v1alpha1.Enabled)
			})

			By("Getting Gatekeeper CR for updating", func() {
				err := K8sClient.Get(ctx, gatekeeperName, gatekeeper)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Updating Gatekeeper CR with validation disabled", func() {
				webhookMode := v1alpha1.Disabled
				gatekeeper.Spec.ValidatingWebhook = &webhookMode
				Expect(K8sClient.Update(ctx, gatekeeper)).Should(Succeed())
				byCheckingValidation(ctx, webhookMode)
			})
		})

		It("Enables Gatekeeper mutation with default values", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			webhookMode := v1alpha1.Enabled
			gatekeeper.Spec.MutatingWebhook = &webhookMode
			Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())

			byCheckingMutation(ctx, webhookMode)

			byCheckingFailurePolicy(ctx, &mutatingWebhookName, "default",
				util.MutatingWebhookConfigurationKind,
				controllers.MutationGatekeeperWebhook,
				&test.DefaultDeployment.FailurePolicy)

			byCheckingNamespaceSelector(ctx, &mutatingWebhookName, "default",
				util.MutatingWebhookConfigurationKind,
				controllers.MutationGatekeeperWebhook,
				test.DefaultDeployment.NamespaceSelector)
		})

		It("Enables Gatekeeper mutation with configured values", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			err := loadGatekeeperFromFile(gatekeeper, gatekeeperWithAllValuesFile)
			Expect(err).ToNot(HaveOccurred())
			webhookMode := v1alpha1.Enabled
			gatekeeper.Spec.MutatingWebhook = &webhookMode
			Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())

			byCheckingMutation(ctx, webhookMode)

			byCheckingFailurePolicy(ctx, &mutatingWebhookName, "expected",
				util.MutatingWebhookConfigurationKind,
				controllers.MutationGatekeeperWebhook,
				gatekeeper.Spec.Webhook.FailurePolicy)

			byCheckingNamespaceSelector(ctx, &mutatingWebhookName, "expected",
				util.MutatingWebhookConfigurationKind,
				controllers.MutationGatekeeperWebhook,
				gatekeeper.Spec.Webhook.NamespaceSelector)
		})

		It("Disables Gatekeeper mutation", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			By("Create Gatekeeper CR with mutation disabled", func() {
				webhookMode := v1alpha1.Disabled
				gatekeeper.Spec.MutatingWebhook = &webhookMode
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
				byCheckingMutation(ctx, webhookMode)
			})
		})

		It("Enables then disables Gatekeeper mutation", func(ctx SpecContext) {
			gatekeeper := emptyGatekeeper()
			By("First creating Gatekeeper CR with mutation enabled", func() {
				webhookMode := v1alpha1.Enabled
				gatekeeper.Spec.MutatingWebhook = &webhookMode
				Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
				byCheckingMutation(ctx, webhookMode)
			})

			By("Getting Gatekeeper CR for updating", func() {
				err := K8sClient.Get(ctx, gatekeeperName, gatekeeper)
				Expect(err).ToNot(HaveOccurred())
			})

			By("Updating Gatekeeper CR with mutation disabled", func() {
				webhookMode := v1alpha1.Disabled
				gatekeeper.Spec.MutatingWebhook = &webhookMode
				Expect(K8sClient.Update(ctx, gatekeeper)).Should(Succeed())
				byCheckingMutation(ctx, webhookMode)
			})
		})

		It("Override Webhook operations with Create, Update, Delete, Connect", func(ctx SpecContext) {
			gatekeeper := &v1alpha1.Gatekeeper{
				ObjectMeta: metav1.ObjectMeta{
					Namespace: gatekeeperNamespace,
					Name:      "gatekeeper",
				},
				Spec: v1alpha1.GatekeeperSpec{
					Webhook: &v1alpha1.WebhookConfig{
						Operations: []v1alpha1.OperationType{
							"CREATE", "UPDATE", "CONNECT", "DELETE",
						},
					},
				},
			}
			Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())

			By("Wait until new Deployments loaded")
			gatekeeperDeployments(ctx)

			By("ValidatingWebhookConfiguration Rules should have 4 operations")
			validatingWebhookConfiguration := &admregv1.ValidatingWebhookConfiguration{}
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, validatingWebhookName, validatingWebhookConfiguration)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(validatingWebhookConfiguration.Webhooks[0].Rules[0].Operations).Should(HaveLen(4))
				g.Expect(validatingWebhookConfiguration.Webhooks[1].Rules[0].Operations).Should(HaveLen(4))
			}, timeout, pollInterval).Should(Succeed())

			By("MutatingWebhookConfiguration Rules should have 4 operations")
			mutatingWebhookConfiguration := &admregv1.MutatingWebhookConfiguration{}
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, mutatingWebhookName, mutatingWebhookConfiguration)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(mutatingWebhookConfiguration.Webhooks[0].Rules[0].Operations).Should(HaveLen(4))
			}, timeout, pollInterval).Should(Succeed())

			gatekeeper.Spec.Webhook.Operations = []v1alpha1.OperationType{"*"}
			Expect(K8sClient.Update(ctx, gatekeeper)).Should(Succeed())

			By("ValidatingWebhookConfiguration Rules should have 1 operations")
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, validatingWebhookName, validatingWebhookConfiguration)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(validatingWebhookConfiguration.Webhooks[0].Rules[0].Operations).Should(HaveLen(1))
				g.Expect(validatingWebhookConfiguration.Webhooks[0].Rules[0].Operations[0]).Should(BeEquivalentTo("*"))
				g.Expect(validatingWebhookConfiguration.Webhooks[1].Rules[0].Operations).Should(HaveLen(1))
				g.Expect(validatingWebhookConfiguration.Webhooks[1].Rules[0].Operations[0]).Should(BeEquivalentTo("*"))
			}, timeout*2, pollInterval).Should(Succeed())

			By("MutatingWebhookConfiguration Rules should have 1 operations")
			Eventually(func(g Gomega) {
				err := K8sClient.Get(ctx, mutatingWebhookName, mutatingWebhookConfiguration)
				g.Expect(err).ShouldNot(HaveOccurred())
				g.Expect(mutatingWebhookConfiguration.Webhooks[0].Rules[0].Operations).Should(HaveLen(1))
				g.Expect(mutatingWebhookConfiguration.Webhooks[0].Rules[0].Operations[0]).Should(BeEquivalentTo("*"))
			}, timeout, pollInterval).Should(Succeed())
		})
	})

	Describe("Test in Openshift Env", Label("openshift"), Ordered, Serial, func() {
		const openshiftNamespace = "openshift-gatekeeper-system"

		Describe("Test Gatekeeper Certification", Label("openshift"), func() {
			It("Should have Openshift Cert Annotation in the Service Resource"+
				" and not have Cert Secret in the Gatekeeper Namespace", func(ctx SpecContext) {
				gatekeeper := emptyGatekeeper()
				By("First creating Gatekeeper CR", func() {
					Expect(K8sClient.Create(ctx, gatekeeper)).Should(Succeed())
				})

				By("All Deployment resources should have a --disable-cert-rotation arg" +
					" and cert secret name for openshift")
				Eventually(func(g Gomega) {
					audit := &appsv1.Deployment{}
					err := K8sClient.Get(ctx, types.NamespacedName{
						Namespace: openshiftNamespace,
						Name:      "gatekeeper-audit",
					}, audit)
					g.Expect(err).ShouldNot(HaveOccurred())

					g.Expect(audit.Spec.Template.Spec.Containers[0].Args).
						Should(ContainElement("--disable-cert-rotation"), "Audit should have disabled cert arg")
					g.Expect(audit.Spec.Template.Spec.Volumes[0].Secret.SecretName).
						Should(Equal(controllers.OpenshiftSecretName),
							"Audit deployment certificate volume mount should have the expected name")
				}, timeout, pollInterval).Should(Succeed())

				Eventually(func(g Gomega) {
					webhook := &appsv1.Deployment{}
					err := K8sClient.Get(ctx, types.NamespacedName{
						Namespace: openshiftNamespace,
						Name:      "gatekeeper-controller-manager",
					}, webhook)
					g.Expect(err).ShouldNot(HaveOccurred())

					g.Expect(webhook.Spec.Template.Spec.Containers[0].Args).
						Should(ContainElement("--disable-cert-rotation"), "Webhook should have disabled cert arg")
					g.Expect(webhook.Spec.Template.Spec.Volumes[0].Secret.SecretName).
						Should(Equal(controllers.OpenshiftSecretName),
							"Webhook deployment certificate volume mount should have the expected name")
				}, timeout, pollInterval).Should(Succeed())

				By("Service resource should have a service.beta.openshift.io/serving-cert-secret-name annotation")
				Eventually(func(g Gomega) {
					service := corev1.Service{}
					err := K8sClient.Get(ctx,
						types.NamespacedName{
							Name:      "gatekeeper-webhook-service",
							Namespace: openshiftNamespace,
						}, &service)
					g.Expect(err).Should(Succeed())

					annotations := service.Annotations

					v, ok := annotations["service.beta.openshift.io/serving-cert-secret-name"]
					g.Expect(ok).Should(BeTrue(),
						"Should have service.beta.openshift.io/serving-cert-secret-name annotation")
					g.Expect(v).Should(Equal(controllers.OpenshiftSecretName),
						"service.beta.openshift.io/serving-cert-secret-name should have the expected value")
				}, timeout, pollInterval).Should(Succeed())

				By("ValidatingWebhookConfiguration should have a service.beta.openshift.io/inject-cabundle annotation")
				Eventually(func(g Gomega) {
					validatingWebhookConfiguration := &admregv1.ValidatingWebhookConfiguration{}

					err := K8sClient.Get(ctx, validatingWebhookName, validatingWebhookConfiguration)
					g.Expect(err).Should(Succeed())

					annotations := validatingWebhookConfiguration.Annotations
					v, ok := annotations["service.beta.openshift.io/inject-cabundle"]
					g.Expect(ok).Should(BeTrue(),
						"Should have service.beta.openshift.io/inject-cabundle annotation")
					g.Expect(v).Should(Equal("true"),
						"Should be true")
				}, timeout, pollInterval).Should(Succeed())

				By("MutatingWebhookConfiguration should have a service.beta.openshift.io/inject-cabundle annotation")
				Eventually(func(g Gomega) {
					mutatingWebhookConfiguration := &admregv1.MutatingWebhookConfiguration{}
					err := K8sClient.Get(ctx, mutatingWebhookName, mutatingWebhookConfiguration)
					g.Expect(err).Should(Succeed())

					annotations := mutatingWebhookConfiguration.Annotations
					v, ok := annotations["service.beta.openshift.io/inject-cabundle"]
					g.Expect(ok).Should(BeTrue(),
						"Should have service.beta.openshift.io/inject-cabundle annotation")
					g.Expect(v).Should(Equal("true"),
						"Should be true")
				}, timeout, pollInterval).Should(Succeed())

				// In this test, gatekeeper-webhook-server-cert does not exist but
				// on OpenShift platform, service-ca will create gatekeeper-webhook-server-cert secret
				By("Cert Secret should not be in the Namespace")
				Consistently(func(g Gomega) bool {
					service := corev1.Secret{}
					err := K8sClient.Get(ctx,
						types.NamespacedName{
							Name:      "gatekeeper-webhook-server-cert",
							Namespace: openshiftNamespace,
						}, &service)

					return apierrors.IsNotFound(err)
				}, 5, pollInterval).Should(BeTrue())
			})
		})
	})
})

func gatekeeperDeployments(ctx SpecContext) (auditDeployment, webhookDeployment *appsv1.Deployment) {
	return gatekeeperAuditDeployment(ctx), gatekeeperWebhookDeployment(ctx)
}

func gatekeeperAuditDeployment(ctx SpecContext) (auditDeployment *appsv1.Deployment) {
	auditDeployment = &appsv1.Deployment{}

	Eventually(func() error {
		return K8sClient.Get(ctx, auditName, auditDeployment)
	}, timeout, pollInterval).ShouldNot(HaveOccurred())

	return
}

func gatekeeperWebhookDeployment(ctx SpecContext) (webhookDeployment *appsv1.Deployment) {
	webhookDeployment = &appsv1.Deployment{}

	Eventually(func() error {
		return K8sClient.Get(ctx, controllerManagerName, webhookDeployment)
	}, timeout, pollInterval).ShouldNot(HaveOccurred())

	return
}

func assertResources(expected, current corev1.ResourceRequirements) {
	Expect(expected.Limits.Cpu().Cmp(*current.Limits.Cpu())).To(BeZero())
	Expect(expected.Limits.Memory().Cmp(*current.Limits.Memory())).To(BeZero())
	Expect(expected.Requests.Cpu().Cmp(*current.Requests.Cpu())).To(BeZero())
	Expect(expected.Requests.Memory().Cmp(*current.Requests.Memory())).To(BeZero())
}

func byCheckingValidation(ctx SpecContext, mode v1alpha1.Mode) {
	By("Checking validation is "+string(mode), func() {
		validatingWebhookConfiguration := &admregv1.ValidatingWebhookConfiguration{}
		Eventually(func() error {
			err := K8sClient.Get(ctx, validatingWebhookName, validatingWebhookConfiguration)
			if !mode.ToBool() && apierrors.IsNotFound(err) {
				return nil
			}

			return err
		}, timeout, pollInterval).ShouldNot(HaveOccurred(), "Validation should be "+string(mode))
	})
}

type getCRDFunc func(types.NamespacedName, *extv1.CustomResourceDefinition)

func byCheckingMutation(ctx SpecContext, mode v1alpha1.Mode) {
	msgNegation := ""

	if !mode.ToBool() {
		msgNegation = "not "
	}

	By(fmt.Sprintf(
		"Checking %s=%s argument is %sset",
		controllers.OperationArg, controllers.OperationMutationWebhook, msgNegation,
	), func() {
		Eventually(func() bool {
			webhookDeployment := gatekeeperWebhookDeployment(ctx)

			return findContainerArgValue(
				webhookDeployment.Spec.Template.Spec.Containers[0].Args,
				controllers.OperationMutationWebhook,
			)
		},
			timeout, pollInterval).Should(Equal(mode.ToBool()), fmt.Sprintf(
			"Argument %s=%s should %sbe set",
			controllers.OperationArg, controllers.OperationMutationWebhook, msgNegation,
		))
	})

	By(fmt.Sprintf(
		"Checking %s=%s argument is %sset",
		controllers.OperationArg, controllers.OperationMutationStatus, msgNegation,
	), func() {
		Eventually(func() bool {
			auditDeployment := gatekeeperAuditDeployment(ctx)

			return findContainerArgValue(auditDeployment.Spec.Template.Spec.Containers[0].Args,
				controllers.OperationMutationStatus)
		},
			timeout, pollInterval).Should(Equal(mode.ToBool()), fmt.Sprintf(
			"Argument %s=%s should %sbe set",
			controllers.OperationArg, controllers.OperationMutationStatus, msgNegation,
		))
	})

	By("Checking MutatingWebhookConfiguration is "+msgNegation+"deployed", func() {
		mutatingWebhookConfiguration := &admregv1.MutatingWebhookConfiguration{}
		Eventually(func() error {
			err := K8sClient.Get(ctx, mutatingWebhookName, mutatingWebhookConfiguration)
			if !mode.ToBool() && apierrors.IsNotFound(err) {
				return nil
			}

			return err
		}, timeout, pollInterval).ShouldNot(HaveOccurred(),
			"MutatingWebhookConfiguration should "+msgNegation+"be deployed")
	})

	crdFn := func(crdName types.NamespacedName, mutatingCRD *extv1.CustomResourceDefinition) {
		Eventually(func() error {
			err := K8sClient.Get(ctx, crdName, mutatingCRD)
			if !mode.ToBool() && apierrors.IsNotFound(err) {
				return nil
			}

			return err
		}, timeout, pollInterval).Should(Succeed())
	}
	byCheckingMutatingCRDs(msgNegation+"deployed", crdFn)
}

func byCheckingMutatingCRDs(deployMsg string, f getCRDFunc) {
	for _, asset := range controllers.MutatingCRDs {
		obj, err := util.GetManifestObject(asset)
		Expect(err).ToNot(HaveOccurred())

		crdNamespacedName := types.NamespacedName{
			Name: obj.GetName(),
		}

		By(fmt.Sprintf("Checking %s Mutating CRD is %s", obj.GetName(), deployMsg), func() {
			mutatingAssignCRD := &extv1.CustomResourceDefinition{}
			f(crdNamespacedName, mutatingAssignCRD)
		})
	}
}

func byCheckingFailurePolicy(ctx SpecContext, webhookNamespacedName *types.NamespacedName,
	testName, kind, webhookName string, failurePolicy *admregv1.FailurePolicyType,
) {
	By(fmt.Sprintf("Checking %s failure policy", testName), func() {
		webhookConfiguration := &unstructured.Unstructured{}
		webhookConfiguration.SetAPIVersion(admregv1.SchemeGroupVersion.String())
		webhookConfiguration.SetKind(kind)
		Eventually(func() error {
			return K8sClient.Get(ctx, *webhookNamespacedName, webhookConfiguration)
		}, timeout, pollInterval).ShouldNot(HaveOccurred())
		assertFailurePolicy(webhookConfiguration, webhookName, failurePolicy)
	})
}

func assertFailurePolicy(obj *unstructured.Unstructured, webhookName string, expected *admregv1.FailurePolicyType) {
	assertWebhook(obj, webhookName, func(webhook map[string]interface{}) {
		Expect(webhook["failurePolicy"]).To(BeEquivalentTo(string(*expected)))
	})
}

func byCheckingNamespaceSelector(ctx SpecContext, webhookNamespacedName *types.NamespacedName,
	testName, kind, webhookName string, namespaceSelector *metav1.LabelSelector,
) {
	By(fmt.Sprintf("Checking %s namespace selector", testName), func() {
		webhookConfiguration := &unstructured.Unstructured{}
		webhookConfiguration.SetAPIVersion(admregv1.SchemeGroupVersion.String())
		webhookConfiguration.SetKind(kind)
		Eventually(func() error {
			return K8sClient.Get(ctx, *webhookNamespacedName, webhookConfiguration)
		}, timeout, pollInterval).ShouldNot(HaveOccurred())
		assertNamespaceSelector(webhookConfiguration, webhookName, namespaceSelector)
	})
}

func assertNamespaceSelector(obj *unstructured.Unstructured, webhookName string, expected *metav1.LabelSelector) {
	assertWebhook(obj, webhookName, func(webhook map[string]interface{}) {
		nsSelector, found, err := unstructured.NestedFieldNoCopy(webhook, "namespaceSelector")
		Expect(err).NotTo(HaveOccurred())
		Expect(found).To(BeTrue())

		nsSelectorBytes, err := json.Marshal(nsSelector)
		Expect(err).NotTo(HaveOccurred())

		nsSelectorTyped := &metav1.LabelSelector{}
		err = json.Unmarshal(nsSelectorBytes, nsSelectorTyped)
		Expect(err).NotTo(HaveOccurred())

		Expect(nsSelectorTyped).To(BeEquivalentTo(expected))
	})
}

func assertWebhook(obj *unstructured.Unstructured, webhookName string, webhookFn func(map[string]interface{})) {
	webhooks, found, err := unstructured.NestedSlice(obj.Object, "webhooks")
	Expect(err).NotTo(HaveOccurred())
	Expect(found).To(BeTrue())

	for _, webhook := range webhooks {
		w, ok := webhook.(map[string]interface{})
		Expect(ok).To(BeTrue())

		if w["name"] == webhookName {
			webhookFn(w)
		}
	}
}

func getContainerArg(args []string, argPrefix string) (arg string, found bool) {
	for _, arg := range args {
		if strings.HasPrefix(arg, argPrefix) {
			return arg, true
		}
	}

	return "", false
}

func findContainerArgValue(args []string, argValue string) bool {
	argKeyValue := fmt.Sprintf("%s=%s", controllers.OperationArg, argValue)
	for _, arg := range args {
		if strings.Compare(arg, argKeyValue) == 0 {
			return true
		}
	}

	return false
}

func loadGatekeeperFromFile(gatekeeper *v1alpha1.Gatekeeper, fileName string) error {
	f, err := os.Open(fmt.Sprintf("../../config/samples/%s", fileName))
	if err != nil {
		return err
	}
	defer f.Close()

	return decodeYAML(f, gatekeeper)
}

func decodeYAML(r io.Reader, obj interface{}) error {
	decoder := yaml.NewYAMLToJSONDecoder(r)

	return decoder.Decode(obj)
}

func useExistingCluster() bool {
	return strings.ToLower(os.Getenv("USE_EXISTING_CLUSTER")) == "true"
}

func getDeploymentReadyReplicas(ctx SpecContext, name types.NamespacedName,
	deploy *appsv1.Deployment,
) (int32, error) {
	err := K8sClient.Get(ctx, name, deploy)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return 0, nil
		}

		return 0, err
	}

	return deploy.Status.ReadyReplicas, nil
}

func emptyGatekeeper() *v1alpha1.Gatekeeper {
	return &v1alpha1.Gatekeeper{
		ObjectMeta: metav1.ObjectMeta{
			Name:      gkName,
			Namespace: gatekeeperNamespace,
		},
	}
}

func getDefaultImage(file string) (image string, imagePullPolicy corev1.PullPolicy, err error) {
	obj, err := util.GetManifestObject(file)
	if err != nil {
		return "", "", err
	}

	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil {
		return "", "", err
	}

	if !found {
		return "", "", fmt.Errorf("Containers not found")
	}

	image, found, err = unstructured.NestedString(containers[0].(map[string]interface{}), "image")
	if err != nil {
		return "", "", err
	}

	if !found {
		return "", "", fmt.Errorf("Image not found")
	}

	policy, found, err := unstructured.NestedString(containers[0].(map[string]interface{}), "imagePullPolicy")
	if err != nil {
		return "", "", err
	}

	if !found {
		return "", "", fmt.Errorf("ImagePullPolicy not found")
	}

	return image, corev1.PullPolicy(policy), nil
}
