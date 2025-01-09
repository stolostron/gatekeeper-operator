/*
Copyright 2021.

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

package v1alpha1

import (
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	admregv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:validation:Enum:=Enabled;Disabled
type Mode string

const (
	Enabled  Mode = "Enabled"
	Disabled Mode = "Disabled"
)

func (m Mode) ToBool() bool {
	return m == Enabled
}

func (m Mode) ToBoolString() string {
	if m.ToBool() {
		return "true"
	}

	return "false"
}

// LogLevel specifies the verbosity of the Pod logs. The supported parameter values are DEBUG, INFO,
// WARNING, or ERROR. The parameter value DEBUG produces the most logs, while ERROR produces the
// least. The default value is INFO.
//
// +kubebuilder:validation:Enum:=DEBUG;INFO;WARNING;ERROR
type LogLevelMode string

const (
	LogLevelDEBUG   LogLevelMode = "DEBUG"
	LogLevelInfo    LogLevelMode = "INFO"
	LogLevelWarning LogLevelMode = "WARNING"
	LogLevelError   LogLevelMode = "ERROR"
)

// Arg represents an argument for the container binary.
type Arg struct {
	// Name is the name of the container argument (i.e. the flag without the leading double dashes).
	//
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Value is the value of the container argument. An empty value is interpreted as `true` (i.e. the
	// flag will be passed with no value).
	//
	// +optional
	Value string `json:"value,omitempty"`
}

type CommonConfig struct {
	// Replicas is the number of desired pods.
	//
	// +optional
	// +kubebuilder:validation:Minimum:=0
	Replicas *int32 `json:"replicas,omitempty"`

	// Resources describes the compute resource requirements for the pod.
	//
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`

	// +optional
	LogLevel *LogLevelMode `json:"logLevel,omitempty"`

	// ContainerArguments is a list of argument names and values to pass to the container. Arguments
	// provided are ignored if the flag is set previously by configurations from other fields.
	// Furthermore, this list of flags is deny listed and aren't currently supported:
	// • port
	// • prometheus-port
	// • health-addr
	// • validating-webhook-configuration-name
	// • mutating-webhook-configuration-name
	// • disable-cert-rotation
	// • client-cert-name
	// • tls-min-version
	//
	// +optional
	ContainerArguments []Arg `json:"containerArguments,omitempty"`
}

// +kubebuilder:validation:Enum:=Enabled;Disabled;Automatic
type AuditFromCacheMode string

const (
	AuditFromCacheEnabled   AuditFromCacheMode = "Enabled"
	AuditFromCacheDisabled  AuditFromCacheMode = "Disabled"
	AuditFromCacheAutomatic AuditFromCacheMode = "Automatic"
)

type AuditConfig struct {
	CommonConfig `json:",inline"`

	// AuditInterval configures how often an audit is run on the cluster. The default value is 60s.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/performance-tuning/#audit-interval.
	//
	// +optional
	AuditInterval *metav1.Duration `json:"auditInterval,omitempty"`

	// ConstraintViolationLimit configures how many violations are reported in a given Constraint
	// resource. The default value is 20.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/performance-tuning/#constraint-violations-limit.
	//
	// +optional
	// +kubebuilder:validation:Minimum:=0
	ConstraintViolationLimit *uint64 `json:"constraintViolationLimit,omitempty"`

	// AuditFromCache sets whether a cache is used for auditing. The parameter value options are
	// Enabled, Disabled, and Automatic. If you set the parameter to Automatic, the Gatekeeper
	// operator manages the syncOnly field in the Config resource. It is not recommended to use
	// Automatic when using referential constraints since those are not detected.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/sync/#replicating-data-with-config.
	//
	// +optional
	AuditFromCache *AuditFromCacheMode `json:"auditFromCache,omitempty"`

	// AuditChunkSize causes audit list requests against the Kubernetes API to be paginated, reducing
	// memory usage. Setting to zero disables pagination. The default value is 500.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/performance-tuning/#audit-chunk-size.
	//
	// +optional
	// +kubebuilder:validation:Minimum:=0
	AuditChunkSize *uint64 `json:"auditChunkSize,omitempty"`

	// EmitAuditEvents enables the emission of all admission violations as Kubernetes events. The
	// default value is Disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup/#alpha-emit-admission-and-audit-events.
	//
	// +optional
	//nolint:lll
	EmitAuditEvents *Mode `json:"emitAuditEvents,omitempty"`

	// AuditEventsInvolvedNamespace controls in which namespace the audit events are created. When you
	// set it to Enabled, audit events are created in the namespace of the object violating the
	// constraint. If you set the parameter to Disabled, it causes all audit events to be created in
	// the Gatekeeper namespace. The default value is Disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup/#alpha-emit-admission-and-audit-events.
	//
	// +optional
	//nolint:lll
	AuditEventsInvolvedNamespace *Mode `json:"auditEventsInvolvedNamespace,omitempty"`
}

// +kubebuilder:validation:Enum:=CONNECT;CREATE;UPDATE;DELETE;*
type OperationType admregv1.OperationType

type WebhookConfig struct {
	CommonConfig `json:",inline"`

	// EmitAdmissionEvents enables the emission of all admission violations as Kubernetes events.
	// The default value is Disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup/#alpha-emit-admission-and-audit-events.
	//
	// +optional
	//nolint:lll
	EmitAdmissionEvents *Mode `json:"emitAdmissionEvents,omitempty"`

	// AdmissionEventsInvolvedNamespace controls in which namespace admission events are created. When
	// set to true, admission events are created in the namespace of the object violating the
	// constraint. If you set the parameter to Disabled, it causes all admission events to be created
	// in the Gatekeeper namespace. The default value is Disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup/#alpha-emit-admission-and-audit-events.
	//
	// +optional
	//nolint:lll
	AdmissionEventsInvolvedNamespace *Mode `json:"admissionEventsInvolvedNamespace,omitempty"`

	// +optional
	// +kubebuilder:validation:Enum:=Ignore;Fail
	FailurePolicy *admregv1.FailurePolicyType `json:"failurePolicy,omitempty"`

	// NamespaceSelector is a label selector to define which namespaces should be handled by the
	// admission webhook.
	//
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`

	// Operations specifies a list of API operations to be checked by the admission webhook. The
	// default value is ["CREATE","UPDATE"].
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-admission/#enable-validation-of-delete-operations.
	//
	// +optional
	//nolint:lll
	Operations []OperationType `json:"operations,omitempty"`

	// DisabledBuiltins is a list of specific OPA built-in functions to disable. By default, http.send
	// is disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup#disable-opa-built-in-functions.
	//
	// +optional
	DisabledBuiltins []string `json:"disabledBuiltins,omitempty"`

	// LogMutations enables the logging of mutation events and errors. The default value is Disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup#beta-enable-mutation-logging-and-annotations.
	//
	// +optional
	//nolint:lll
	LogMutations *Mode `json:"logMutations,omitempty"`

	// MutationAnnotations adds the gatekeeper.sh/mutation-id and gatekeeper.sh/mutations annotations
	// to mutated objects. The default value is Enabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/customize-startup#beta-enable-mutation-logging-and-annotations.
	//
	// +optional
	//nolint:lll
	MutationAnnotations *Mode `json:"mutationAnnotations,omitempty"`

	// LogDenies enables the logging of all deny, dry run, and warn failures. The default value is
	// Disabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/violations/#log-denies
	// +optional
	LogDenies *Mode `json:"logDenies,omitempty"`
}

type ImageConfig struct {
	// Deprecated: Image is deprecated. If you decide to use Image, the operator displays a warning
	// and message about removal in a future release. To address this, the operator relies on the
	// environment variable set in its manifest at deployment time and is the expected behavior after
	// this field is removed.
	// Image to pull including registry (optional), repository, name, and tag
	// e.g. quay.io/gatekeeper/gatekeeper-operator:latest
	//
	// +optional
	Image *string `json:"image,omitempty"`

	// +optional
	ImagePullPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

type ConfigConfig struct {
	// Matches is a list of exempt namespaces for specified processes. If you specify the namespaces,
	// the Matches are set on the existing Config spec.match.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/exempt-namespaces.
	//
	// +optional
	Matches []v1alpha1.MatchEntry `json:"matches,omitempty"`

	// DisableDefaultMatches is a boolean parameter to disable appending default exempt namespaces of
	// the Gatekeeper Operator to spec.config.matches. The default value is false to enable appending
	// default namespaces of the operator.
	//
	// +optional
	DisableDefaultMatches bool `json:"disableDefaultMatches,omitempty"`
}

// GatekeeperSpec specifies how the operator should deploy Gatekeeper to the cluster.
type GatekeeperSpec struct {
	// Audit specifies the configuration for the Gatekeeper auditing function.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/audit.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Audit Configuration"
	Audit *AuditConfig `json:"audit,omitempty"`

	// ValidatingWebhook specifies whether the Gatekeeper validating admission webhook is enabled.
	// The default value is Enabled.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Validating Webhook"
	ValidatingWebhook *Mode `json:"validatingWebhook,omitempty"`

	// MutatingWebhook specifies whether the Gatekeeper mutating admission webhook is enabled.
	// The default value is Enabled.
	// See https://open-policy-agent.github.io/gatekeeper/website/docs/mutation.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Mutating Webhook"
	MutatingWebhook *Mode `json:"mutatingWebhook,omitempty"`

	// Webhook specifies the configuration for the Gatekeeper admission webhook.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Webhook Config"
	Webhook *WebhookConfig `json:"webhook,omitempty"`

	// Config specifies configurations for the configs.config.gatekeeper.sh API, allowing
	// high-level configuration of Gatekeeper.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Config Configuration"
	Config *ConfigConfig `json:"config,omitempty"`

	// Image specifies the configuration for handling Gatekeeper deployment images.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image Configuration"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:hidden"}
	Image *ImageConfig `json:"image,omitempty"`

	// NodeSelector is a map of node selectors to add to the Gatekeeper deployment Pods.
	// See https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Node Selector"
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// Affinity is a node affinity specification to add to the Gatekeeper deployment Pods.
	// See https://kubernetes.io/docs/concepts/scheduling-eviction/assign-pod-node/.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Affinity"
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// Tolerations is an array of tolerations to add to the Gatekeeper deployment Pods.
	// See https://kubernetes.io/docs/concepts/scheduling-eviction/taint-and-toleration/.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tolerations"
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// PodAnnotations is a map of additional annotations to be added to the Gatekeeper deployment
	// Pods.
	//
	// +optional
	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Pod Annotations"
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`
}

// GatekeeperStatus defines the observed state of Gatekeeper
type GatekeeperStatus struct{}

// Gatekeeper is the schema for the gatekeepers API. Gatekeeper contains configurations for the Gatekeeper operator,
// which deploys Open Policy Agent Gatekeeper, a policy engine that enforces policies through admission controller
// webhooks.
// See https://github.com/open-policy-agent/gatekeeper.
//
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=gatekeepers,scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +operator-sdk:csv:customresourcedefinitions:displayName="Gatekeeper",resources={{Deployment,v1,gatekeeper-deployment}}
//
//nolint:lll
type Gatekeeper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatekeeperSpec   `json:"spec,omitempty"`
	Status GatekeeperStatus `json:"status,omitempty"`
}

// GatekeeperList contains a list of gatekeeper operator configurations.
//
// +kubebuilder:object:root=true
type GatekeeperList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Gatekeeper `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gatekeeper{}, &GatekeeperList{})
}
