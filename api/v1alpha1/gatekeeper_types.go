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

// GatekeeperSpec defines the desired state of Gatekeeper
type GatekeeperSpec struct {
	// INSERT ADDITIONAL SPEC FIELDS - desired state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Image Configuration"
	// +operator-sdk:csv:customresourcedefinitions:type=spec,xDescriptors={"urn:alm:descriptor:com.tectonic.ui:hidden"}
	// +optional
	Image *ImageConfig `json:"image,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Audit Configuration"
	// +optional
	Audit *AuditConfig `json:"audit,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Validating Webhook"
	// +optional
	ValidatingWebhook *Mode `json:"validatingWebhook,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Mutating Webhook"
	// +optional
	MutatingWebhook *Mode `json:"mutatingWebhook,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Webhook Config"
	// +optional
	Webhook *WebhookConfig `json:"webhook,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Node Selector"
	// +optional
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Affinity"
	// +optional
	Affinity *corev1.Affinity `json:"affinity,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Tolerations"
	// +optional
	Tolerations []corev1.Toleration `json:"tolerations,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Pod Annotations"
	// +optional
	PodAnnotations map[string]string `json:"podAnnotations,omitempty"`

	// +operator-sdk:csv:customresourcedefinitions:type=spec,displayName="Config Configuration"
	// +optional
	Config *ConfigConfig `json:"config,omitempty"`
}

type ConfigConfig struct {
	// +optional
	// This field is the same type as the config spec.match. These will be appended to config.spec.match
	Matches []v1alpha1.MatchEntry `json:"matches,omitempty"`
	// +optional
	// Default is false. Setting this to true disables appending the default exempt namespaces to spec.config.matches.
	DisableDefaultMatches bool `json:"disableDefaultMatches,omitempty"`
}

type ImageConfig struct {
	// Deprecated: Image is deprecated. Its continued use will be honored by
	// the operator with a warning and removed in a future release. Instead,
	// the operator will rely on the environment variable set in its manifest
	// at deployment time and will be the default behavior after this field is
	// removed.
	// Image to pull including registry (optional), repository, name, and tag
	// e.g. quay.io/gatekeeper/gatekeeper-operator:latest
	// +optional
	Image *string `json:"image,omitempty"`
	// +optional
	ImagePullPolicy *corev1.PullPolicy `json:"imagePullPolicy,omitempty"`
}

type AuditConfig struct {
	// +kubebuilder:validation:Minimum:=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	AuditInterval *metav1.Duration `json:"auditInterval,omitempty"`
	// +kubebuilder:validation:Minimum:=0
	// +optional
	ConstraintViolationLimit *uint64 `json:"constraintViolationLimit,omitempty"`
	// +optional
	// Setting Automatic lets the Gatekeeper operator manage syncOnly in the config resource.
	// It is not recommended to use Automatic when using referential constraints since those are not detected.
	AuditFromCache *AuditFromCacheMode `json:"auditFromCache,omitempty"`
	// +kubebuilder:validation:Minimum:=0
	// +optional
	AuditChunkSize *uint64 `json:"auditChunkSize,omitempty"`
	// +optional
	LogLevel *LogLevelMode `json:"logLevel,omitempty"`
	// +optional
	EmitAuditEvents *Mode `json:"emitAuditEvents,omitempty"`
	// +optional
	AuditEventsInvolvedNamespace *Mode `json:"auditEventsInvolvedNamespace,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
}

type WebhookConfig struct {
	// +kubebuilder:validation:Minimum:=0
	// +optional
	Replicas *int32 `json:"replicas,omitempty"`
	// +optional
	LogLevel *LogLevelMode `json:"logLevel,omitempty"`
	// +optional
	EmitAdmissionEvents *Mode `json:"emitAdmissionEvents,omitempty"`
	// +optional
	AdmissionEventsInvolvedNamespace *Mode `json:"admissionEventsInvolvedNamespace,omitempty"`
	// +optional
	FailurePolicy *admregv1.FailurePolicyType `json:"failurePolicy,omitempty"`
	// +optional
	NamespaceSelector *metav1.LabelSelector `json:"namespaceSelector,omitempty"`
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// +optional
	Operations []OperationType `json:"operations,omitempty"`
	// +optional
	DisabledBuiltins []string `json:"disabledBuiltins,omitempty"`
	// +optional
	// Sets the --log-mutations flag which enables logging of mutation events and errors. This defaults to Disabled.
	LogMutations *Mode `json:"logMutations,omitempty"`
	// +optional
	// Sets the --mutation-annotations flag which adds the gatekeeper.sh/mutation-id and gatekeeper.sh/mutations
	// annotations on mutated objects. This defaults to Disabled.
	MutationAnnotations *Mode `json:"mutationAnnotations,omitempty"`
}

// +kubebuilder:validation:Enum:=CONNECT;CREATE;UPDATE;DELETE;*
type OperationType admregv1.OperationType

// +kubebuilder:validation:Enum:=DEBUG;INFO;WARNING;ERROR
type LogLevelMode string

const (
	LogLevelDEBUG   LogLevelMode = "DEBUG"
	LogLevelInfo    LogLevelMode = "INFO"
	LogLevelWarning LogLevelMode = "WARNING"
	LogLevelError   LogLevelMode = "ERROR"
)

// +kubebuilder:validation:Enum:=Enabled;Disabled;Automatic
type AuditFromCacheMode string

const (
	AuditFromCacheEnabled   AuditFromCacheMode = "Enabled"
	AuditFromCacheDisabled  AuditFromCacheMode = "Disabled"
	AuditFromCacheAutomatic AuditFromCacheMode = "Automatic"
)

// GatekeeperStatus defines the observed state of Gatekeeper
type GatekeeperStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file

	// ObservedGeneration is the generation as observed by the operator consuming this API.
	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Observed Generation"
	ObservedGeneration int64 `json:"observedGeneration"`

	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Audit Conditions"
	AuditConditions []StatusCondition `json:"auditConditions"`

	// +operator-sdk:csv:customresourcedefinitions:type=status,displayName="Webhook Conditions"
	WebhookConditions []StatusCondition `json:"webhookConditions"`
}

// StatusCondition describes the current state of a component.
type StatusCondition struct {
	// Type of status condition.
	Type StatusConditionType `json:"type"`
	// Status of the condition, one of True, False, Unknown.
	Status corev1.ConditionStatus `json:"status"`
	// Last time the condition was checked.
	// +optional
	LastProbeTime metav1.Time `json:"lastProbeTime,omitempty"`
	// Last time the condition transit from one status to another.
	// +optional
	LastTransitionTime metav1.Time `json:"lastTransitionTime,omitempty"`
	// (brief) reason for the condition's last transition.
	// +optional
	Reason string `json:"reason,omitempty"`
	// Human readable message indicating details about last transition.
	// +optional
	Message string `json:"message,omitempty"`
}

// +kubebuilder:validation:Enum:=Ready;Not Ready
type StatusConditionType string

const (
	StatusReady    StatusConditionType = "Ready"
	StatusNotReady StatusConditionType = "Not Ready"
)

//nolint:lll
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:path=gatekeepers,scope=Cluster
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`
// +operator-sdk:csv:customresourcedefinitions:displayName="Gatekeeper",resources={{Deployment,v1,gatekeeper-deployment}}

// Gatekeeper is the Schema for the gatekeepers API
type Gatekeeper struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   GatekeeperSpec   `json:"spec,omitempty"`
	Status GatekeeperStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// GatekeeperList contains a list of Gatekeeper
type GatekeeperList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Gatekeeper `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Gatekeeper{}, &GatekeeperList{})
}
