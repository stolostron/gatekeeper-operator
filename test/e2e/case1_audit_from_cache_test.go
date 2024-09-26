package e2e

import (
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"golang.org/x/exp/slices"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"

	gv1alpha1 "github.com/stolostron/gatekeeper-operator/api/v1alpha1"
	. "github.com/stolostron/gatekeeper-operator/test/e2e/util"
)

var _ = Describe("Test auditFromCache", Ordered, func() {
	const (
		case1GatekeeperYaml             string = "../resources/case1_audit_from_cache/gatekeeper.yaml"
		case1TemplateYaml               string = "../resources/case1_audit_from_cache/template.yaml"
		case1ConstraintPodYaml          string = "../resources/case1_audit_from_cache/constraint-pod.yaml"
		case1ConstraintPod2Yaml         string = "../resources/case1_audit_from_cache/constraint-pod-2.yaml"
		case1ConstraintIngressYaml      string = "../resources/case1_audit_from_cache/constraint-ingress.yaml"
		case1ConstraintStorageclassYaml string = "../resources/case1_audit_from_cache/constraint-storageclass.yaml"
		case1PodYaml                    string = "../resources/case1_audit_from_cache/pod.yaml"
		allowNamespace                  string = "case1-allow"
		denyNamespace                   string = "case1-deny"
		case1ConstraintUpdateYaml       string = "../resources/case1_audit_from_cache/constraint-update.yaml"
		case1ConstraintUpdateChangeYaml string = "../resources/case1_audit_from_cache/constraint-update-change.yaml"
		case1ConstraintWrongYaml        string = "../resources/case1_audit_from_cache/constraint-wrong.yaml"
	)

	var (
		constraintGVR = schema.GroupVersionResource{
			Group:    "constraints.gatekeeper.sh",
			Version:  "v1beta1",
			Resource: "case1template",
		}

		templateGVR = schema.GroupVersionResource{
			Group:    "templates.gatekeeper.sh",
			Version:  "v1",
			Resource: "constrainttemplates",
		}
	)

	BeforeAll(func() {
		if !useExistingCluster() {
			Skip("Test requires existing cluster. Set environment variable USE_EXISTING_CLUSTER=true and try again.")
		}

		By("Create namespaces to compare")
		Kubectl("create", "ns", allowNamespace)
		Kubectl("create", "ns", denyNamespace)

		By("Create a gatekeeper resource")
		_, err := KubectlWithOutput("apply", "-f", case1GatekeeperYaml)
		Expect(err).ShouldNot(HaveOccurred())
		// Need enough time until gatekeeper is up
		ctlDeployment := GetWithTimeout(clientHubDynamic, deploymentGVR,
			"gatekeeper-controller-manager", gatekeeperNamespace, true, 150)
		Expect(ctlDeployment).NotTo(BeNil())

		Eventually(func(g Gomega) {
			auditDeployment := GetWithTimeout(clientHubDynamic, deploymentGVR,
				"gatekeeper-audit", gatekeeperNamespace, true, 60)
			g.Expect(auditDeployment).NotTo(BeNil())

			availableReplicas, _, err := unstructured.NestedInt64(auditDeployment.Object, "status", "availableReplicas")
			g.Expect(err).ShouldNot(HaveOccurred())
			g.Expect(availableReplicas).Should(BeNumerically(">", 0))
		}, 2*time.Minute, 2*time.Second).Should(Succeed())

		_, err = KubectlWithOutput("apply", "-f", case1TemplateYaml)
		Expect(err).ShouldNot(HaveOccurred())
		template := GetWithTimeout(clientHubDynamic, templateGVR, "case1template", "", true, 60)
		Expect(template).NotTo(BeNil())

		Eventually(func() error {
			_, err = KubectlWithOutput("apply", "-f", case1ConstraintStorageclassYaml)

			return err
		}, timeout).ShouldNot(HaveOccurred())
		storageclass := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-storageclass-deny", "", true, 60)
		Expect(storageclass).NotTo(BeNil())

		Eventually(func() error {
			_, err = KubectlWithOutput("apply", "-f", case1ConstraintPodYaml)

			return err
		}, timeout).ShouldNot(HaveOccurred())
		pod := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-pod-deny", "", true, 60)
		Expect(pod).NotTo(BeNil())

		Eventually(func() error {
			_, err = KubectlWithOutput("apply", "-f", case1ConstraintPod2Yaml)

			return err
		}, timeout).ShouldNot(HaveOccurred())
		pod2 := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-pod-deny-2", "", true, 60)
		Expect(pod2).NotTo(BeNil())

		Eventually(func() error {
			_, err = KubectlWithOutput("apply", "-f", case1ConstraintIngressYaml)

			return err
		}, timeout).ShouldNot(HaveOccurred())
		ingress := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-ingress-deny", "", true, 60)
		Expect(ingress).NotTo(BeNil())
	})

	Describe("Gatekeeper with auditFromCache=Automatic create syncOnly config", Ordered, func() {
		It("should create config resource with syncOnly includes pod, ingress, storageclass", func(ctx SpecContext) {
			config := &v1alpha1.Config{}

			By("config syncOnly should have 3 elements, duplicates should be omitted")
			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return config.Spec.Sync.SyncOnly
			}, timeout).Should(HaveLen(3))

			expectedSyncOnly := map[string]v1alpha1.SyncOnlyEntry{
				"Ingress": {
					Group:   "networking.k8s.io",
					Kind:    "Ingress",
					Version: "v1",
				},
				"Pod": {
					Kind:    "Pod",
					Version: "v1",
				},
				"StorageClass": {
					Group:   "storage.k8s.io",
					Version: "v1",
					Kind:    "StorageClass",
				},
			}
			for key, val := range expectedSyncOnly {
				foundSyncOnly := slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Kind == key
				})
				Expect(foundSyncOnly).ShouldNot(Equal(-1))
				Expect(config.Spec.Sync.SyncOnly[foundSyncOnly]).Should(BeEquivalentTo(val))
			}
		})
		It("Should have an error message with the cached pod list", func() {
			_, err := KubectlWithOutput("apply", "-f", case1PodYaml, "-n", allowNamespace)
			Expect(err).ShouldNot(HaveOccurred())
			output, err := KubectlWithOutput("apply", "-f", case1PodYaml, "-n", denyNamespace)
			Expect(err).Should(HaveOccurred())
			Expect(output).
				Should(ContainSubstring("cached data: {\"case1-pod\": {\"apiVersion\": \"v1\", \"kind\": \"Pod\""))
		})
	})

	Describe("Gatekeeper with auditFromCache=Automatic delete syncOnly config", Ordered, func() {
		It("Should have 3 syncOnly elements in config", func(ctx SpecContext) {
			config := &v1alpha1.Config{}
			By("Config syncOnly should have 3 elements")

			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return config.Spec.Sync.SyncOnly
			}, timeout).Should(HaveLen(3))
		})
		It("Should have 2 syncOnly elements in config", func(ctx SpecContext) {
			Kubectl("delete", "-f", case1ConstraintIngressYaml, "--ignore-not-found")

			config := &v1alpha1.Config{}

			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return config.Spec.Sync.SyncOnly
			}, timeout).Should(HaveLen(2))

			By("Ingress should not be in SyncOnly")
			Expect(slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
				return s.Kind == "Ingress"
			})).Should(Equal(-1))
		})
		It("Should have 1 syncOnly elements in config", func(ctx SpecContext) {
			Kubectl("delete", "-f", case1ConstraintStorageclassYaml, "--ignore-not-found")

			config := &v1alpha1.Config{}

			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return config.Spec.Sync.SyncOnly
			}, timeout).Should(HaveLen(1))

			By("StorageClass should not be in SyncOnly")
			Expect(slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
				return s.Kind == "StorageClass"
			})).Should(Equal(-1))
		})
		It("Should still have 1 syncOnly elements in config when Pod constraint is deleted", func(ctx SpecContext) {
			Kubectl("delete", "-f", case1ConstraintPodYaml, "--ignore-not-found")
			config := &v1alpha1.Config{}

			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return config.Spec.Sync.SyncOnly
			}, timeout).Should(HaveLen(1))

			By("Pod should exist in SyncOnly because case1ConstraintPod2 yet exist")
			Expect(slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
				return s.Kind == "Pod"
			})).ShouldNot(Equal(-1))
		})
		It("Should have 0 syncOnly elements in config ", func(ctx SpecContext) {
			Kubectl("delete", "-f", case1ConstraintPod2Yaml, "--ignore-not-found")
			config := &v1alpha1.Config{}

			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return config.Spec.Sync.SyncOnly
			}, timeout).Should(BeNil())
		})
	})

	Describe("Updating constraint should apply to config resource", Ordered, func() {
		It("Should update the config resource", func(ctx SpecContext) {
			config := &v1alpha1.Config{}

			By("Add a new constraint")
			Kubectl("apply", "-f", case1ConstraintUpdateYaml)

			By("Group name 'apps.StatefulSet' should exist in SyncOnly")
			Eventually(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "apps" && s.Kind == "StatefulSet"
				})
			}, timeout).ShouldNot(Equal(-1))

			By("Update the config")
			Kubectl("apply", "-f", case1ConstraintUpdateChangeYaml)

			By("Group name 'batch.CronJob' should exist in SyncOnly")
			Eventually(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "batch" && s.Kind == "CronJob"
				})
			}, timeout).ShouldNot(Equal(-1))

			By("Group name 'events.k8s.io.Event' should exist in SyncOnly")
			Eventually(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "events.k8s.io" && s.Kind == "Event"
				})
			}, timeout).ShouldNot(Equal(-1))
		})
	})

	Describe("Add wrong match kinds", Ordered, func() {
		It("Should not add not founded matches", func(ctx SpecContext) {
			config := &v1alpha1.Config{}

			By("Apply constraint")
			Kubectl("apply", "-f", case1ConstraintWrongYaml)

			Eventually(func() error {
				return K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
			}, timeout).ShouldNot(HaveOccurred())

			By("Group name 'ohmyhappy.sad.io' should not exist in SyncOnly")
			Consistently(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "ohmyhappy.sad.io" && s.Kind == "alien"
				})
			}, 10).Should(Equal(-1))

			By("Group name 'apps.StatefulSet' should still exist in SyncOnly")
			Eventually(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "apps" && s.Kind == "StatefulSet"
				})
			}, timeout).ShouldNot(Equal(-1))

			By("Group name 'batch.CronJob' should still exist in SyncOnly")
			Eventually(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "batch" && s.Kind == "CronJob"
				})
			}, timeout).ShouldNot(Equal(-1))

			By("Group name 'events.k8s.io.Event' should still exist in SyncOnly")
			Eventually(func(g Gomega) int {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())

				return slices.IndexFunc(config.Spec.Sync.SyncOnly, func(s v1alpha1.SyncOnlyEntry) bool {
					return s.Group == "events.k8s.io" && s.Kind == "Event"
				})
			}, timeout).ShouldNot(Equal(-1))
		})
	})

	AfterAll(func(ctx SpecContext) {
		Kubectl("delete", "ns", allowNamespace, "--ignore-not-found", "--grace-period=1")
		Kubectl("delete", "ns", denyNamespace, "--ignore-not-found", "--grace-period=1")
		Kubectl("delete", "-f", case1ConstraintPodYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintIngressYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintStorageclassYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintPod2Yaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintUpdateYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintWrongYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1GatekeeperYaml, "--ignore-not-found")

		Eventually(func() bool {
			err := K8sClient.Get(ctx, gatekeeperName, &gv1alpha1.Gatekeeper{})
			if err == nil {
				return false
			}

			return apierrors.IsNotFound(err)
		}, deleteTimeout, pollInterval).Should(BeTrue())

		Eventually(func() bool {
			err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace},
				&v1alpha1.Config{})
			if err == nil {
				return false
			}

			return apierrors.IsNotFound(err)
		}, deleteTimeout, pollInterval).Should(BeTrue())

		ctlDeployment := GetWithTimeout(clientHubDynamic, deploymentGVR,
			"gatekeeper-controller-manager", gatekeeperNamespace, false, 60)
		Expect(ctlDeployment).Should(BeNil())
		auditDeployment := GetWithTimeout(clientHubDynamic, deploymentGVR,
			"gatekeeper-audit", gatekeeperNamespace, false, 60)
		Expect(auditDeployment).Should(BeNil())
	})
})
