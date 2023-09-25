package e2e

import (
	. "github.com/gatekeeper/gatekeeper-operator/test/e2e/util"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"golang.org/x/exp/slices"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
)

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
)

var constraintGVR = schema.GroupVersionResource{
	Group:    "constraints.gatekeeper.sh",
	Version:  "v1beta1",
	Resource: "case1template",
}

var templateGVR = schema.GroupVersionResource{
	Group:    "templates.gatekeeper.sh",
	Version:  "v1",
	Resource: "constrainttemplates",
}

var _ = FDescribe("Test auditFromCache", Label("auditFromCache"), Ordered, func() {
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
		ctlDeployment := GetWithTimeout(clientHubDynamic, deploymentGVR,
			"gatekeeper-controller-manager", gatekeeperNamespace, true, 60)
		Expect(ctlDeployment).NotTo(BeNil())
		auditDeployment := GetWithTimeout(clientHubDynamic, deploymentGVR,
			"gatekeeper-audit", gatekeeperNamespace, true, 60)
		Expect(auditDeployment).NotTo(BeNil())

		Kubectl("apply", "-f", case1TemplateYaml)
		Kubectl("apply", "-f", case1ConstraintPodYaml)
		Kubectl("apply", "-f", case1ConstraintPod2Yaml)
		Kubectl("apply", "-f", case1ConstraintIngressYaml)
		Kubectl("apply", "-f", case1ConstraintStorageclassYaml)

		template := GetWithTimeout(clientHubDynamic, templateGVR, "case1template", "", true, 2)
		Expect(template).NotTo(BeNil())
		storageclass := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-storageclass-deny", "", true, 2)
		Expect(storageclass).NotTo(BeNil())
		pod := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-pod-deny", "", true, 2)
		Expect(pod).NotTo(BeNil())
		pod2 := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-pod-deny-2", "", true, 2)
		Expect(pod2).NotTo(BeNil())
		ingress := GetWithTimeout(clientHubDynamic, constraintGVR, "case1-ingress-deny", "", true, 2)
		Expect(ingress).NotTo(BeNil())
	})
	Describe("Gatekeeper with auditFromCache=Automatic create syncOnly config", func() {
		It("should create config resource with syncOnly includes pod, ingress, storageclass", func() {
			config := &v1alpha1.Config{}

			By("config syncOnly should have 3 elements, duplicated should be deleted")
			Eventually(func(g Gomega) []v1alpha1.SyncOnlyEntry {
				err := K8sClient.Get(ctx, types.NamespacedName{Name: "config", Namespace: gatekeeperNamespace}, config)
				g.Expect(err).ShouldNot(HaveOccurred())
				return config.Spec.Sync.SyncOnly
			}, 30).Should(HaveLen(3))

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
				Expect(config.Spec.Sync.SyncOnly[foundSyncOnly]).Should(BeEquivalentTo(val))
			}
		})
		It("Should error message shows cached pod list", func() {
			_, err := KubectlWithOutput("apply", "-f", case1PodYaml, "-n", allowNamespace)
			Expect(err).ShouldNot(HaveOccurred())
			output, err := KubectlWithOutput("apply", "-f", case1PodYaml, "-n", denyNamespace)
			Expect(err).Should(HaveOccurred())
			Expect(output).
				Should(ContainSubstring("cached data: {\"case1-pod\": {\"apiVersion\": \"v1\", \"kind\": \"Pod\""))
		})
	})
	AfterAll(func() {
		Kubectl("delete", "ns", allowNamespace, "--ignore-not-found", "--grace-period=1")
		Kubectl("delete", "ns", denyNamespace, "--ignore-not-found", "--grace-period=1")
		Kubectl("delete", "-f", case1ConstraintPodYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintIngressYaml, "--ignore-not-found")
		Kubectl("delete", "-f", case1ConstraintStorageclassYaml, "--ignore-not-found")
	})
})
