//go:build !ignore_autogenerated

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

// Code generated by controller-gen. DO NOT EDIT.

package v1alpha1

import (
	configv1alpha1 "github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1"
	runtime "k8s.io/apimachinery/pkg/runtime"
)

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *AuditConfig) DeepCopyInto(out *AuditConfig) {
	*out = *in
	if in.AuditInterval != nil {
		in, out := &in.AuditInterval, &out.AuditInterval
		*out = new(v1.Duration)
		**out = **in
	}
	if in.ConstraintViolationLimit != nil {
		in, out := &in.ConstraintViolationLimit, &out.ConstraintViolationLimit
		*out = new(uint64)
		**out = **in
	}
	if in.AuditFromCache != nil {
		in, out := &in.AuditFromCache, &out.AuditFromCache
		*out = new(AuditFromCacheMode)
		**out = **in
	}
	if in.AuditChunkSize != nil {
		in, out := &in.AuditChunkSize, &out.AuditChunkSize
		*out = new(uint64)
		**out = **in
	}
	if in.EmitAuditEvents != nil {
		in, out := &in.EmitAuditEvents, &out.EmitAuditEvents
		*out = new(Mode)
		**out = **in
	}
	if in.AuditEventsInvolvedNamespace != nil {
		in, out := &in.AuditEventsInvolvedNamespace, &out.AuditEventsInvolvedNamespace
		*out = new(Mode)
		**out = **in
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.LogLevel != nil {
		in, out := &in.LogLevel, &out.LogLevel
		*out = new(LogLevelMode)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new AuditConfig.
func (in *AuditConfig) DeepCopy() *AuditConfig {
	if in == nil {
		return nil
	}
	out := new(AuditConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ConfigConfig) DeepCopyInto(out *ConfigConfig) {
	*out = *in
	if in.Matches != nil {
		in, out := &in.Matches, &out.Matches
		*out = make([]configv1alpha1.MatchEntry, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ConfigConfig.
func (in *ConfigConfig) DeepCopy() *ConfigConfig {
	if in == nil {
		return nil
	}
	out := new(ConfigConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *Gatekeeper) DeepCopyInto(out *Gatekeeper) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	out.Status = in.Status
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new Gatekeeper.
func (in *Gatekeeper) DeepCopy() *Gatekeeper {
	if in == nil {
		return nil
	}
	out := new(Gatekeeper)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *Gatekeeper) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatekeeperList) DeepCopyInto(out *GatekeeperList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		in, out := &in.Items, &out.Items
		*out = make([]Gatekeeper, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatekeeperList.
func (in *GatekeeperList) DeepCopy() *GatekeeperList {
	if in == nil {
		return nil
	}
	out := new(GatekeeperList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject is an autogenerated deepcopy function, copying the receiver, creating a new runtime.Object.
func (in *GatekeeperList) DeepCopyObject() runtime.Object {
	if c := in.DeepCopy(); c != nil {
		return c
	}
	return nil
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatekeeperSpec) DeepCopyInto(out *GatekeeperSpec) {
	*out = *in
	if in.Audit != nil {
		in, out := &in.Audit, &out.Audit
		*out = new(AuditConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.ValidatingWebhook != nil {
		in, out := &in.ValidatingWebhook, &out.ValidatingWebhook
		*out = new(Mode)
		**out = **in
	}
	if in.MutatingWebhook != nil {
		in, out := &in.MutatingWebhook, &out.MutatingWebhook
		*out = new(Mode)
		**out = **in
	}
	if in.Webhook != nil {
		in, out := &in.Webhook, &out.Webhook
		*out = new(WebhookConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Config != nil {
		in, out := &in.Config, &out.Config
		*out = new(ConfigConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.Image != nil {
		in, out := &in.Image, &out.Image
		*out = new(ImageConfig)
		(*in).DeepCopyInto(*out)
	}
	if in.NodeSelector != nil {
		in, out := &in.NodeSelector, &out.NodeSelector
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
	if in.Affinity != nil {
		in, out := &in.Affinity, &out.Affinity
		*out = new(corev1.Affinity)
		(*in).DeepCopyInto(*out)
	}
	if in.Tolerations != nil {
		in, out := &in.Tolerations, &out.Tolerations
		*out = make([]corev1.Toleration, len(*in))
		for i := range *in {
			(*in)[i].DeepCopyInto(&(*out)[i])
		}
	}
	if in.PodAnnotations != nil {
		in, out := &in.PodAnnotations, &out.PodAnnotations
		*out = make(map[string]string, len(*in))
		for key, val := range *in {
			(*out)[key] = val
		}
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatekeeperSpec.
func (in *GatekeeperSpec) DeepCopy() *GatekeeperSpec {
	if in == nil {
		return nil
	}
	out := new(GatekeeperSpec)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *GatekeeperStatus) DeepCopyInto(out *GatekeeperStatus) {
	*out = *in
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new GatekeeperStatus.
func (in *GatekeeperStatus) DeepCopy() *GatekeeperStatus {
	if in == nil {
		return nil
	}
	out := new(GatekeeperStatus)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *ImageConfig) DeepCopyInto(out *ImageConfig) {
	*out = *in
	if in.Image != nil {
		in, out := &in.Image, &out.Image
		*out = new(string)
		**out = **in
	}
	if in.ImagePullPolicy != nil {
		in, out := &in.ImagePullPolicy, &out.ImagePullPolicy
		*out = new(corev1.PullPolicy)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new ImageConfig.
func (in *ImageConfig) DeepCopy() *ImageConfig {
	if in == nil {
		return nil
	}
	out := new(ImageConfig)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyInto is an autogenerated deepcopy function, copying the receiver, writing into out. in must be non-nil.
func (in *WebhookConfig) DeepCopyInto(out *WebhookConfig) {
	*out = *in
	if in.EmitAdmissionEvents != nil {
		in, out := &in.EmitAdmissionEvents, &out.EmitAdmissionEvents
		*out = new(Mode)
		**out = **in
	}
	if in.AdmissionEventsInvolvedNamespace != nil {
		in, out := &in.AdmissionEventsInvolvedNamespace, &out.AdmissionEventsInvolvedNamespace
		*out = new(Mode)
		**out = **in
	}
	if in.FailurePolicy != nil {
		in, out := &in.FailurePolicy, &out.FailurePolicy
		*out = new(admissionregistrationv1.FailurePolicyType)
		**out = **in
	}
	if in.NamespaceSelector != nil {
		in, out := &in.NamespaceSelector, &out.NamespaceSelector
		*out = new(v1.LabelSelector)
		(*in).DeepCopyInto(*out)
	}
	if in.Operations != nil {
		in, out := &in.Operations, &out.Operations
		*out = make([]OperationType, len(*in))
		copy(*out, *in)
	}
	if in.DisabledBuiltins != nil {
		in, out := &in.DisabledBuiltins, &out.DisabledBuiltins
		*out = make([]string, len(*in))
		copy(*out, *in)
	}
	if in.LogMutations != nil {
		in, out := &in.LogMutations, &out.LogMutations
		*out = new(Mode)
		**out = **in
	}
	if in.MutationAnnotations != nil {
		in, out := &in.MutationAnnotations, &out.MutationAnnotations
		*out = new(Mode)
		**out = **in
	}
	if in.Replicas != nil {
		in, out := &in.Replicas, &out.Replicas
		*out = new(int32)
		**out = **in
	}
	if in.Resources != nil {
		in, out := &in.Resources, &out.Resources
		*out = new(corev1.ResourceRequirements)
		(*in).DeepCopyInto(*out)
	}
	if in.LogLevel != nil {
		in, out := &in.LogLevel, &out.LogLevel
		*out = new(LogLevelMode)
		**out = **in
	}
	if in.LogDenies != nil {
		in, out := &in.LogDenies, &out.LogDenies
		*out = new(Mode)
		**out = **in
	}
}

// DeepCopy is an autogenerated deepcopy function, copying the receiver, creating a new WebhookConfig.
func (in *WebhookConfig) DeepCopy() *WebhookConfig {
	if in == nil {
		return nil
	}
	out := new(WebhookConfig)
	in.DeepCopyInto(out)
	return out
}
