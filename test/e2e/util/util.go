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

package util

import (
	"context"
	"fmt"
	"os/exec"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type defaultConfig struct {
	AuditReplicas     int32
	WebhookReplicas   int32
	Affinity          *corev1.Affinity
	NodeSelector      map[string]string
	PodAnnotations    map[string]string
	WebResources      *corev1.ResourceRequirements
	AuditResources    *corev1.ResourceRequirements
	FailurePolicy     admregv1.FailurePolicyType
	NamespaceSelector *metav1.LabelSelector
}

// DefaultDeployment is the expected default configuration to be deployed
var DefaultDeployment = defaultConfig{
	AuditReplicas:   int32(1),
	WebhookReplicas: int32(3),
	Affinity: &corev1.Affinity{
		PodAntiAffinity: &corev1.PodAntiAffinity{
			PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
				{
					Weight: 100,
					PodAffinityTerm: corev1.PodAffinityTerm{
						LabelSelector: &metav1.LabelSelector{
							MatchExpressions: []metav1.LabelSelectorRequirement{
								{
									Key:      "gatekeeper.sh/operation",
									Operator: metav1.LabelSelectorOpIn,
									Values: []string{
										"webhook",
									},
								},
							},
						},
						TopologyKey: "kubernetes.io/hostname",
					},
				},
			},
		},
	},
	NodeSelector: map[string]string{
		"kubernetes.io/os": "linux",
	},
	AuditResources: &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("1Gi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	},
	WebResources: &corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1000m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),
			corev1.ResourceMemory: resource.MustParse("512Mi"),
		},
	},
	FailurePolicy: admregv1.Ignore,
	NamespaceSelector: &metav1.LabelSelector{
		MatchExpressions: []metav1.LabelSelectorRequirement{
			{
				Key:      "admission.gatekeeper.sh/ignore",
				Operator: metav1.LabelSelectorOpDoesNotExist,
			},
			{
				Key:      "kubernetes.io/metadata.name",
				Operator: metav1.LabelSelectorOpNotIn,
				Values:   []string{"gatekeeper-system"},
			},
		},
	},
}

// GetWithTimeout keeps polling to get the object for timeout seconds until wantFound is met
// (true for found, false for not found)
func GetWithTimeout(
	clientHubDynamic dynamic.Interface,
	gvr schema.GroupVersionResource,
	name, namespace string,
	wantFound bool,
	timeout int,
) *unstructured.Unstructured {
	if timeout < 1 {
		timeout = 1
	}

	var obj *unstructured.Unstructured

	EventuallyWithOffset(1, func() error {
		var err error
		namespace := clientHubDynamic.Resource(gvr).Namespace(namespace)

		obj, err = namespace.Get(context.TODO(), name, metav1.GetOptions{})
		if wantFound && err != nil {
			return err
		}

		if !wantFound && err == nil {
			return fmt.Errorf("expected to return IsNotFound error")
		}

		if !wantFound && err != nil && !errors.IsNotFound(err) {
			return err
		}

		return nil
	}, timeout, 1).Should(BeNil())

	if wantFound {
		return obj
	}

	return nil
}

// Kubectl execute kubectl cli
func Kubectl(args ...string) {
	cmd := exec.Command("kubectl", args...)

	err := cmd.Start()
	if err != nil {
		Fail(fmt.Sprintf("Error: %v", err))
	}
}

// KubectlWithOutput execute kubectl cli and return output and error
func KubectlWithOutput(args ...string) (string, error) {
	kubectlCmd := exec.Command("kubectl", args...)

	output, err := kubectlCmd.CombinedOutput()
	if err != nil {
		// Reformat error to include kubectl command and stderr output
		err = fmt.Errorf(
			"error running command '%s':\n %s: %s",
			strings.Join(kubectlCmd.Args, " "),
			output,
			err.Error(),
		)
	}

	return string(output), err
}
