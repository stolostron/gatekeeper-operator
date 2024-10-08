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

package controllers

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strconv"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/pkg/errors"
	admregv1 "k8s.io/api/admissionregistration/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/controller-runtime/pkg/source"

	operatorv1alpha1 "github.com/stolostron/gatekeeper-operator/api/v1alpha1"
	"github.com/stolostron/gatekeeper-operator/controllers/merge"
	"github.com/stolostron/gatekeeper-operator/pkg/platform"
	"github.com/stolostron/gatekeeper-operator/pkg/util"
)

const (
	defaultGatekeeperCrName             = "gatekeeper"
	GatekeeperImageEnvVar               = "RELATED_IMAGE_GATEKEEPER"
	NamespaceFile                       = "v1_namespace_gatekeeper-system.yaml"
	crdFilePrefix                       = "apiextensions.k8s.io_v1_customresourcedefinition_"
	AssignCRDFile                       = crdFilePrefix + "assign.mutations.gatekeeper.sh.yaml"
	AssignMetadataCRDFile               = crdFilePrefix + "assignmetadata.mutations.gatekeeper.sh.yaml"
	MutatorPodStatusCRDFile             = crdFilePrefix + "mutatorpodstatuses.status.gatekeeper.sh.yaml"
	ModifySetCRDFile                    = crdFilePrefix + "modifyset.mutations.gatekeeper.sh.yaml"
	ProviderCRDFile                     = crdFilePrefix + "providers.externaldata.gatekeeper.sh.yaml"
	AuditFile                           = "apps_v1_deployment_gatekeeper-audit.yaml"
	WebhookFile                         = "apps_v1_deployment_gatekeeper-controller-manager.yaml"
	rbacFilePrefix                      = "rbac.authorization.k8s.io_v1_"
	ClusterRoleFile                     = rbacFilePrefix + "clusterrole_gatekeeper-manager-role.yaml"
	ClusterRoleBindingFile              = rbacFilePrefix + "clusterrolebinding_gatekeeper-manager-rolebinding.yaml"
	RoleFile                            = rbacFilePrefix + "role_gatekeeper-manager-role.yaml"
	RoleBindingFile                     = rbacFilePrefix + "rolebinding_gatekeeper-manager-rolebinding.yaml"
	ServerCertFile                      = "v1_secret_gatekeeper-webhook-server-cert.yaml"
	ServiceFile                         = "v1_service_gatekeeper-webhook-service.yaml"
	ValidationGatekeeperWebhook         = "validation.gatekeeper.sh"
	CheckIgnoreLabelGatekeeperWebhook   = "check-ignore-label.gatekeeper.sh"
	MutationGatekeeperWebhook           = "mutation.gatekeeper.sh"
	AuditDeploymentName                 = "gatekeeper-audit"
	WebhookDeploymentName               = "gatekeeper-controller-manager"
	managerContainer                    = "manager"
	LogLevelArg                         = "--log-level"
	AuditIntervalArg                    = "--audit-interval"
	ConstraintViolationLimitArg         = "--constraint-violations-limit"
	AuditFromCacheArg                   = "--audit-from-cache"
	AuditChunkSizeArg                   = "--audit-chunk-size"
	EmitAuditEventsArg                  = "--emit-audit-events"
	EmitAdmissionEventsArg              = "--emit-admission-events"
	AdmissionEventsInvolvedNamespaceArg = "--admission-events-involved-namespace"
	AuditEventsInvolvedNamespaceArg     = "--audit-events-involved-namespace"
	ExemptNamespaceArg                  = "--exempt-namespace"
	OperationArg                        = "--operation"
	OperationMutationStatus             = "mutation-status"
	OperationMutationWebhook            = "mutation-webhook"
	DisabledBuiltinArg                  = "--disable-opa-builtin"
	LogMutationsArg                     = "--log-mutations"
	MutationAnnotationsArg              = "--mutation-annotations"
	LogDeniesArg                        = "--log-denies"
	admissionFileFmt                    = "admissionregistration.k8s.io_v1_%[1]swebhookconfiguration" +
		"_gatekeeper-%[1]s-webhook-configuration.yaml"
	OpenshiftSecretName = "gatekeeper-webhook-server-cert-ocp"
)

var (
	ValidatingWebhookConfiguration = fmt.Sprintf(admissionFileFmt, "validating")
	MutatingWebhookConfiguration   = fmt.Sprintf(admissionFileFmt, "mutating")
	orderedStaticAssets            = []string{
		NamespaceFile,
		"v1_resourcequota_gatekeeper-critical-pods.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_configs.config.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_constrainttemplates.templates.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_constrainttemplatepodstatuses.status.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_constraintpodstatuses.status.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_expansiontemplate.expansion.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_expansiontemplatepodstatuses.status.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_assignimage.mutations.gatekeeper.sh.yaml",
		"apiextensions.k8s.io_v1_customresourcedefinition_syncsets.syncset.gatekeeper.sh.yaml",
		ModifySetCRDFile,
		ProviderCRDFile,
		AssignCRDFile,
		AssignMetadataCRDFile,
		MutatorPodStatusCRDFile,
		"v1_serviceaccount_gatekeeper-admin.yaml",
		"policy_v1_poddisruptionbudget_gatekeeper-controller-manager.yaml",
		ClusterRoleFile,
		ClusterRoleBindingFile,
		RoleFile,
		RoleBindingFile,
		AuditFile,
		WebhookFile,
		ServiceFile,
		// ServerCertFile will be added when it is not openshift platform
	}
	webhookStaticAssets = []string{
		ValidatingWebhookConfiguration,
		MutatingWebhookConfiguration,
	}
	MutatingCRDs = []string{
		AssignCRDFile,
		AssignMetadataCRDFile,
		MutatorPodStatusCRDFile,
	}
)

// GatekeeperReconciler reconciles a Gatekeeper object
type GatekeeperReconciler struct {
	client.Client
	Log                    logr.Logger
	Scheme                 *runtime.Scheme
	Namespace              string
	PlatformInfo           platform.PlatformInfo
	DiscoveryStorage       *DiscoveryStorage
	ManualReconcileTrigger chan event.GenericEvent
	isCPSCtrlRunning       bool
	cpsCtrlCtxCancel       context.CancelFunc
	isConfigCtrlRunning    bool
	configCtrlCtxCancel    context.CancelFunc
	subControllerWait      sync.WaitGroup
	DynamicClient          *dynamic.DynamicClient
	KubeConfig             *rest.Config
	EnableLeaderElection   bool
}

type crudOperation uint32

const (
	applyCrud  crudOperation = iota
	deleteCrud crudOperation = iota
)

// Gatekeeper Operator RBAC permissions to manage Gatekeeper custom resource
// +kubebuilder:rbac:groups=operator.gatekeeper.sh,resources=gatekeepers,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=operator.gatekeeper.sh,resources=gatekeepers/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=operator.gatekeeper.sh,resources=gatekeepers/finalizers,verbs=delete;get;update;patch

// Gatekeeper Operator RBAC permissions to deploy Gatekeeper. Many of these
// RBAC permissions are needed because the operator must have the permissions
// to grant Gatekeeper its required RBAC permissions.

// Cluster Scoped
// +kubebuilder:rbac:groups=*,resources=*,verbs=get;list;watch
// +kubebuilder:rbac:groups=core,resources=namespaces;events,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apiextensions.k8s.io,resources=customresourcedefinitions,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.gatekeeper.sh,resources=configs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=config.gatekeeper.sh,resources=configs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=constraints.gatekeeper.sh,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=mutations.gatekeeper.sh,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,resources=podsecuritypolicies,verbs=create;delete;update;use
// +kubebuilder:rbac:groups=status.gatekeeper.sh,resources=*,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=templates.gatekeeper.sh,resources=constrainttemplates,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=templates.gatekeeper.sh,resources=constrainttemplates/finalizers,verbs=get;update;patch;delete
// +kubebuilder:rbac:groups=templates.gatekeeper.sh,resources=constrainttemplates/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,resources=clusterroles;clusterrolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,verbs=create
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingwebhookconfigurations,resourceNames=gatekeeper-validating-webhook-configuration,verbs=get;list;watch;update;patch;delete
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,verbs=create
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=mutatingwebhookconfigurations,resourceNames=gatekeeper-mutating-webhook-configuration,verbs=delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=admissionregistration.k8s.io,resources=validatingadmissionpolicies;validatingadmissionpolicybindings,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=externaldata.gatekeeper.sh,resources=providers,verbs=create;delete;get;list;patch;update;watch
// +kubebuilder:rbac:groups=expansion.gatekeeper.sh,resources=*,verbs=create;delete;get;list;patch;update;watch

// Namespace Scoped
// +kubebuilder:rbac:groups=core,namespace="system",resources=secrets;serviceaccounts;services;resourcequotas,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=rbac.authorization.k8s.io,namespace="system",resources=roles;rolebindings,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=apps,namespace="system",resources=deployments,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=policy,namespace="system",resources=poddisruptionbudgets,verbs=create;delete;update;use

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.9.2/pkg/reconcile
func (r *GatekeeperReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := r.Log.WithValues("gatekeeper", req.NamespacedName)
	logger.Info("Reconciling Gatekeeper")

	var requeueTime time.Duration
	var resultErr error

	if req.Name != defaultGatekeeperCrName {
		err := fmt.Errorf("Gatekeeper resource name must be '%s'", defaultGatekeeperCrName)
		logger.Error(err, "Invalid Gatekeeper resource name")

		// Return success to avoid requeue
		return ctrl.Result{}, nil
	}

	gatekeeper := &operatorv1alpha1.Gatekeeper{}

	err := r.Get(ctx, req.NamespacedName, gatekeeper)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("The Gatekeeper resource is not found.")

			r.StopSubControllers()

			return ctrl.Result{}, nil
		}

		logger.Error(err, "Failed to get Gatekeeper resource. Requeuing.")

		return ctrl.Result{}, err
	}

	//nolint:staticcheck
	if gatekeeper.Spec.Image != nil && gatekeeper.Spec.Image.Image != nil {
		logger.Info("WARNING: operator.gatekeeper.sh/v1alpha1 Gatekeeper spec.image.image field "+
			"is no longer supported and will be removed in a future release.",
			"spec.image.image", gatekeeper.Spec.Image.Image)
	}

	err, requeue := r.deployGatekeeperResources(ctx, gatekeeper)
	if err != nil {
		return ctrl.Result{}, errors.Wrap(err, "Unable to deploy Gatekeeper resources")
	} else if requeue {
		return ctrl.Result{RequeueAfter: 5 * time.Second}, nil
	}

	if err := r.handleConfigController(ctx); err != nil {
		if errors.Is(err, errCrdNotReady) {
			// Config CRD is not ready, wait for the CRD is ready
			requeueTime = time.Second * 3
		} else {
			resultErr = err
		}
	}

	if err := r.handleCPSController(ctx, gatekeeper); err != nil {
		if errors.Is(err, errCrdNotReady) {
			// ConstraintPodStatus CRD is not ready, wait for the CRD is ready
			requeueTime = time.Second * 3
		} else {
			resultErr = err
		}
	}

	err = r.initConfig(ctx, gatekeeper)
	if err != nil {
		logger.Error(err, "Fail to set the default Config")

		return ctrl.Result{}, err
	}

	return reconcile.Result{RequeueAfter: requeueTime}, resultErr
}

// SetupWithManager sets up the controller with the Manager.
func (r *GatekeeperReconciler) SetupWithManager(mgr ctrl.Manager, fromCPSMgrSource source.Source) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: int(1)}).
		For(&operatorv1alpha1.Gatekeeper{}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldGeneration := e.ObjectOld.GetGeneration()
				newGeneration := e.ObjectNew.GetGeneration()

				return oldGeneration != newGeneration
			},
		}).
		WatchesRawSource(fromCPSMgrSource).
		Complete(r)
}

func (r *GatekeeperReconciler) StopSubControllers() {
	log := r.Log.WithName("cleanup")

	if r.cpsCtrlCtxCancel != nil {
		log.Info("Stopping the ConstraintPodStatus controller manager.")
		r.cpsCtrlCtxCancel()
	}

	if r.configCtrlCtxCancel != nil {
		log.Info("Stopping the Config controller manager.")
		r.configCtrlCtxCancel()
	}

	log.Info("The resource controllers are shutting down.")
	r.subControllerWait.Wait()
}

func (r *GatekeeperReconciler) deployGatekeeperResources(
	ctx context.Context, gatekeeper *operatorv1alpha1.Gatekeeper,
) (error, bool) {
	deleteWebhookAssets,
		applyOrderedAssets,
		applyWebhookAssets,
		deleteCRDAssets := getStaticAssets(gatekeeper, r.isOpenShift())

	if err := r.deleteAssets(ctx, deleteWebhookAssets, gatekeeper); err != nil {
		return err, false
	}

	// Checking for deployment before deploying assets or deleting CRDs to
	// avoid transient errors e.g. cert rotator errors, removing required CRD
	// resources, etc.
	err, requeue := r.validateWebhookDeployment(ctx)
	if err != nil {
		return err, false
	}

	if err := r.applyAssets(ctx, applyOrderedAssets, gatekeeper, false); err != nil {
		return err, false
	}

	if err := r.applyAssets(ctx, applyWebhookAssets, gatekeeper, requeue); err != nil {
		return err, false
	}

	if err := r.deleteAssets(ctx, deleteCRDAssets, gatekeeper); err != nil {
		return err, false
	}

	return nil, requeue
}

func (r *GatekeeperReconciler) deleteAssets(
	ctx context.Context, assets []string, gatekeeper *operatorv1alpha1.Gatekeeper,
) error {
	for _, a := range assets {
		obj, err := util.GetManifestObject(a)
		if err != nil {
			return err
		}

		if err = r.crudResource(ctx, obj, gatekeeper, deleteCrud); err != nil {
			return err
		}
	}

	return nil
}

func (r *GatekeeperReconciler) applyAssets(
	ctx context.Context, assets []string, gatekeeper *operatorv1alpha1.Gatekeeper, controllerDeploymentPending bool,
) error {
	for _, a := range assets {
		err := r.applyAsset(ctx, gatekeeper, a, controllerDeploymentPending)
		if err != nil {
			return err
		}
	}

	return nil
}

func (r *GatekeeperReconciler) applyAsset(
	ctx context.Context, gatekeeper *operatorv1alpha1.Gatekeeper, asset string, controllerDeploymentPending bool,
) error {
	obj, err := util.GetManifestObject(asset)
	if err != nil {
		return err
	}

	err = crOverrides(r.Log, gatekeeper, asset, obj, r.Namespace, r.isOpenShift(), controllerDeploymentPending)
	if err != nil {
		return err
	}

	if err = r.crudResource(ctx, obj, gatekeeper, applyCrud); err != nil {
		return err
	}

	return nil
}

func (r *GatekeeperReconciler) validateWebhookDeployment(ctx context.Context) (error, bool) {
	r.Log.Info(fmt.Sprintf("Validating %s deployment status", WebhookDeploymentName))

	obj, err := util.GetManifestObject(WebhookFile)
	if err != nil {
		return err, false
	}

	deployment := &unstructured.Unstructured{}
	deployment.SetAPIVersion(obj.GetAPIVersion())
	deployment.SetKind(obj.GetKind())
	namespacedName := types.NamespacedName{
		Namespace: r.Namespace,
		Name:      obj.GetName(),
	}

	err = r.Get(ctx, namespacedName, deployment)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("Deployment not found, will set webhook failure policy to ignore and requeue...")

			return nil, true
		}

		return err, false
	}

	r.Log.Info("Deployment found, checking replicas ...")

	replicas, _, err := unstructured.NestedInt64(deployment.Object, "status", "replicas")
	if err != nil {
		return err, false
	}

	readyReplicas, ok, err := unstructured.NestedInt64(deployment.Object, "status", "readyReplicas")
	if err != nil {
		return err, false
	}

	if !ok {
		r.Log.Info(
			"Deployment status.readyReplicas not found or populated yet, " +
				"will set webhook failure policy to ignore and requeue.",
		)

		return nil, true
	}

	if replicas == readyReplicas {
		r.Log.Info("Deployment validation successful, all replicas ready",
			"replicas", replicas,
			"readyReplicas", readyReplicas,
		)

		return nil, false
	}

	r.Log.Info("Deployment replicas not ready, will set webhook failure policy to ignore and requeue.",
		"replicas", replicas,
		"readyReplicas", readyReplicas,
	)

	return nil, true
}

func getStaticAssets(
	gatekeeper *operatorv1alpha1.Gatekeeper, isOpenshift bool,
) (deleteWebhookAssets, applyOrderedAssets, applyWebhookAssets, deleteCRDAssets []string) {
	validatingWebhookEnabled := gatekeeper.Spec.ValidatingWebhook == nil || gatekeeper.Spec.ValidatingWebhook.ToBool()
	mutatingWebhookEnabled := mutatingWebhookEnabled(gatekeeper.Spec.MutatingWebhook)

	deleteWebhookAssets = make([]string, 0)
	applyOrderedAssets = make([]string, 0)
	applyWebhookAssets = make([]string, 0)
	deleteCRDAssets = make([]string, 0)
	// Copy over our set of ordered static assets so we maintain its
	// immutability.
	applyOrderedAssets = append(applyOrderedAssets, orderedStaticAssets...)
	applyWebhookAssets = append(applyWebhookAssets, webhookStaticAssets...)

	if !validatingWebhookEnabled {
		// Remove and apply ValidatingWebhookConfiguration resource
		deleteWebhookAssets = append(deleteWebhookAssets, ValidatingWebhookConfiguration)
		applyWebhookAssets = getSubsetOfAssets(applyWebhookAssets, ValidatingWebhookConfiguration)
	}

	if !mutatingWebhookEnabled {
		// Remove and apply mutating resources
		deleteWebhookAssets = append(deleteWebhookAssets, MutatingWebhookConfiguration)
		applyOrderedAssets = getSubsetOfAssets(applyOrderedAssets, MutatingCRDs...)
		applyWebhookAssets = getSubsetOfAssets(applyWebhookAssets, MutatingWebhookConfiguration)

		deleteCRDAssets = append(deleteCRDAssets, MutatingCRDs...)
	}

	if !isOpenshift {
		applyOrderedAssets = append(applyOrderedAssets, ServerCertFile)
	}

	return
}

func mutatingWebhookEnabled(mode *operatorv1alpha1.Mode) bool {
	return mode == nil || mode.ToBool()
}

func getSubsetOfAssets(inputAssets []string, assetsToRemove ...string) []string {
	outputAssets := make([]string, 0)

	for _, i := range inputAssets {
		addAsset := true

		for _, j := range assetsToRemove {
			if i == j {
				addAsset = false
			}
		}

		if addAsset {
			outputAssets = append(outputAssets, i)
		}
	}

	return outputAssets
}

func (r *GatekeeperReconciler) crudResource(
	ctx context.Context,
	obj *unstructured.Unstructured,
	gatekeeper *operatorv1alpha1.Gatekeeper,
	operation crudOperation,
) error {
	clusterObj := &unstructured.Unstructured{}
	clusterObj.SetAPIVersion(obj.GetAPIVersion())
	clusterObj.SetKind(obj.GetKind())

	namespacedName := types.NamespacedName{
		Namespace: obj.GetNamespace(),
		Name:      obj.GetName(),
	}

	logger := r.Log.WithValues("Gatekeeper resource", namespacedName)

	// Skip adding a controller reference for the namespace so that a deletion
	// of the Gatekeeper CR does not also delete the namespace and everything
	// within it.
	if obj.GetKind() != util.NamespaceKind {
		err := ctrl.SetControllerReference(gatekeeper, obj, r.Scheme)
		if err != nil {
			return errors.Wrapf(err, "Unable to set controller reference for %s", namespacedName)
		}
	}

	err := r.Get(ctx, namespacedName, clusterObj)
	if err != nil && !apierrors.IsNotFound(err) {
		return errors.Wrapf(err, "Error attempting to get resource %s", namespacedName)
	}

	switch operation {
	case applyCrud:
		if apierrors.IsNotFound(err) {
			if err = r.Create(ctx, obj); err != nil {
				return errors.Wrapf(err, "Error attempting to create resource %s", namespacedName)
			}

			logger.Info("Created Gatekeeper resource")
		} else {
			err = merge.RetainClusterObjectFields(obj, clusterObj)
			if err != nil {
				return errors.Wrapf(err, "Unable to retain cluster object fields from %s", namespacedName)
			}

			if err = r.Update(ctx, obj); err != nil {
				return errors.Wrapf(err, "Error attempting to update resource %s", namespacedName)
			}

			logger.Info("Updated Gatekeeper resource")
		}
	case deleteCrud:
		if err = r.Delete(ctx, obj); err != nil {
			if !apierrors.IsNotFound(err) {
				return errors.Wrapf(err, "Error attempting to delete resource %s", namespacedName)
			}
		}

		logger.Info("Deleted Gatekeeper resource")
	}

	return nil
}

func (r *GatekeeperReconciler) isOpenShift() bool {
	return r.PlatformInfo.IsOpenShift()
}

var commonSpecOverridesFn = []func(*unstructured.Unstructured, operatorv1alpha1.GatekeeperSpec) error{
	setAffinity,
	setNodeSelector,
	setPodAnnotations,
	setTolerations,
	containerOverrides,
	setEnableMutation,
}

var commonContainerOverridesFn = []func(map[string]interface{}, operatorv1alpha1.GatekeeperSpec) error{
	setImage,
}

// crOverrides
func crOverrides(
	log logr.Logger,
	gatekeeper *operatorv1alpha1.Gatekeeper,
	asset string,
	obj *unstructured.Unstructured,
	namespace string,
	isOpenshift bool,
	controllerDeploymentPending bool,
) error {
	if asset == NamespaceFile {
		obj.SetName(namespace)

		return nil
	}
	// set resource's namespace
	if err := setNamespace(obj, asset, namespace); err != nil {
		return err
	}

	switch asset {
	// audit overrides
	case AuditFile:
		if err := commonOverrides(obj, gatekeeper.Spec); err != nil {
			return err
		}

		if err := auditOverrides(log, obj, gatekeeper.Spec.Audit); err != nil {
			return err
		}

		if isOpenshift {
			if err := removeAnnotations(obj); err != nil {
				return err
			}

			if err := openShiftDeploymentOverrides(obj); err != nil {
				return err
			}
		}
	// webhook overrides
	case WebhookFile:
		if err := commonOverrides(obj, gatekeeper.Spec); err != nil {
			return err
		}

		if err := webhookOverrides(log, obj, gatekeeper.Spec.Webhook); err != nil {
			return err
		}

		if isOpenshift {
			if err := removeAnnotations(obj); err != nil {
				return err
			}

			if err := openShiftDeploymentOverrides(obj); err != nil {
				return err
			}
		}
	// ValidatingWebhookConfiguration overrides
	case ValidatingWebhookConfiguration:
		if err := webhookConfigurationOverrides(
			obj, gatekeeper.Spec.Webhook, namespace, ValidationGatekeeperWebhook, true, controllerDeploymentPending,
		); err != nil {
			return err
		}

		if err := webhookConfigurationOverrides(
			obj,
			gatekeeper.Spec.Webhook,
			namespace,
			CheckIgnoreLabelGatekeeperWebhook,
			false,
			controllerDeploymentPending,
		); err != nil {
			return err
		}

		if isOpenshift {
			setOpenshiftCertInjectAnnotation(obj)
		}
	// MutatingWebhookConfiguration overrides
	case MutatingWebhookConfiguration:
		if err := webhookConfigurationOverrides(
			obj,
			gatekeeper.Spec.Webhook,
			namespace,
			MutationGatekeeperWebhook,
			true,
			controllerDeploymentPending,
		); err != nil {
			return err
		}

		if isOpenshift {
			setOpenshiftCertInjectAnnotation(obj)
		}
	// ClusterRole overrides
	case ClusterRoleFile:
		if !mutatingWebhookEnabled(gatekeeper.Spec.MutatingWebhook) {
			if err := removeMutatingRBACRules(obj); err != nil {
				return err
			}
		}
	case ServiceFile:
		if isOpenshift {
			setOpenshiftCertAnnotation(obj)
		}
	}

	return nil
}

func setOpenshiftCertAnnotation(obj *unstructured.Unstructured) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}

	annotations["service.beta.openshift.io/serving-cert-secret-name"] = OpenshiftSecretName

	obj.SetAnnotations(annotations)
}

func setOpenshiftCertInjectAnnotation(obj *unstructured.Unstructured) {
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string, 1)
	}

	annotations["service.beta.openshift.io/inject-cabundle"] = "true"

	obj.SetAnnotations(annotations)
}

func commonOverrides(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	for _, f := range commonSpecOverridesFn {
		if err := f(obj, spec); err != nil {
			return err
		}
	}

	return nil
}

func auditOverrides(log logr.Logger, obj *unstructured.Unstructured, audit *operatorv1alpha1.AuditConfig) error {
	if audit != nil {
		if err := setAuditInterval(obj, audit.AuditInterval); err != nil {
			return err
		}

		if err := setConstraintViolationLimit(obj, audit.ConstraintViolationLimit); err != nil {
			return err
		}

		if err := setAuditFromCache(obj, audit.AuditFromCache); err != nil {
			return err
		}

		if err := setAuditChunkSize(obj, audit.AuditChunkSize); err != nil {
			return err
		}

		if err := setEmitEvents(obj, EmitAuditEventsArg, audit.EmitAuditEvents); err != nil {
			return err
		}

		err := setEventsInvolvedNamespace(obj, AuditEventsInvolvedNamespaceArg, audit.AuditEventsInvolvedNamespace)
		if err != nil {
			return err
		}

		if err := setCommonConfig(log, obj, audit.CommonConfig); err != nil {
			return err
		}
	}

	return nil
}

func webhookOverrides(log logr.Logger, obj *unstructured.Unstructured, webhook *operatorv1alpha1.WebhookConfig) error {
	if webhook != nil {
		if err := setEmitEvents(obj, EmitAdmissionEventsArg, webhook.EmitAdmissionEvents); err != nil {
			return err
		}

		if err := setEventsInvolvedNamespace(obj, AdmissionEventsInvolvedNamespaceArg,
			webhook.AdmissionEventsInvolvedNamespace); err != nil {
			return err
		}

		if err := setDisabledBuiltins(obj, webhook.DisabledBuiltins); err != nil {
			return err
		}

		if err := setMutationFlags(obj, webhook); err != nil {
			return err
		}

		if err := setLogDeniesFlag(obj, webhook.LogDenies); err != nil {
			return err
		}

		if err := setCommonConfig(log, obj, webhook.CommonConfig); err != nil {
			return err
		}
	}

	return nil
}

// override common properties
func webhookConfigurationOverrides(
	obj *unstructured.Unstructured,
	webhook *operatorv1alpha1.WebhookConfig,
	gatekeeperNamespace string,
	webhookName string,
	updateFailurePolicy bool,
	controllerDeploymentPending bool,
) error {
	// Set failure policy to ignore if deployment is still pending.
	if controllerDeploymentPending {
		ignore := admregv1.Ignore
		failurePolicy := &ignore

		if err := setFailurePolicy(obj, failurePolicy, webhookName); err != nil {
			return err
		}
	}

	if webhook != nil {
		if updateFailurePolicy && !controllerDeploymentPending {
			failurePolicy := webhook.FailurePolicy
			if err := setFailurePolicy(obj, failurePolicy, webhookName); err != nil {
				return err
			}
		}

		if err := setOperations(obj, webhook.Operations, webhookName); err != nil {
			return err
		}

		if err := setNamespaceSelector(obj, webhook.NamespaceSelector, gatekeeperNamespace, webhookName); err != nil {
			return err
		}
	} else if err := setNamespaceSelector(obj, nil, gatekeeperNamespace, webhookName); err != nil {
		return err
	}

	return nil
}

type matchRuleFunc func(map[string]interface{}) (bool, error)

var matchMutatingRBACRuleFns = []matchRuleFunc{
	matchGatekeeperMutatingRBACRule,
	matchMutatingWebhookConfigurationRBACRule,
}

func removeMutatingRBACRules(obj *unstructured.Unstructured) error {
	for _, f := range matchMutatingRBACRuleFns {
		if err := removeRBACRule(obj, f); err != nil {
			return err
		}
	}

	return nil
}

func removeRBACRule(obj *unstructured.Unstructured, matchRuleFn matchRuleFunc) error {
	rules, found, err := unstructured.NestedSlice(obj.Object, "rules")
	if err != nil || !found {
		return errors.Wrapf(err, "Failed to retrieve rules from clusterrole")
	}

	for i, rule := range rules {
		r := rule.(map[string]interface{})
		if found, err := matchRuleFn(r); err != nil {
			return err
		} else if found {
			rules = append(rules[:i], rules[i+1:]...)

			break
		}
	}

	if err := unstructured.SetNestedSlice(obj.Object, rules, "rules"); err != nil {
		return errors.Wrapf(err, "Failed to set rules in clusterrole")
	}

	return nil
}

func matchGatekeeperMutatingRBACRule(rule map[string]interface{}) (bool, error) {
	apiGroups, found, err := unstructured.NestedStringSlice(rule, "apiGroups")
	if !found || err != nil {
		return false, errors.Wrapf(err, "Failed to retrieve apiGroups from rule")
	}

	if apiGroups[0] == "mutations.gatekeeper.sh" {
		return true, nil
	}

	return false, nil
}

func matchMutatingWebhookConfigurationRBACRule(rule map[string]interface{}) (bool, error) {
	apiGroups, found, err := unstructured.NestedStringSlice(rule, "apiGroups")
	if !found || err != nil {
		return false, errors.Wrapf(err, "Failed to retrieve apiGroups from rule")
	}

	resources, found, err := unstructured.NestedStringSlice(rule, "resources")
	if !found || err != nil {
		return false, errors.Wrapf(err, "Failed to retrieve resources from rule")
	}

	if apiGroups[0] == "admissionregistration.k8s.io" &&
		resources[0] == "mutatingwebhookconfigurations" {
		return true, nil
	}

	return false, nil
}

func containerOverrides(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	for _, f := range commonContainerOverridesFn {
		err := setContainerAttrWithFn(obj, func(container map[string]interface{}) error {
			return f(container, spec)
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// setCommonConfig takes a deployment and CommonConfig struct and applies the configuration from
// CommonConfig to the deployment, returning any errors.
func setCommonConfig(log logr.Logger, obj *unstructured.Unstructured, config operatorv1alpha1.CommonConfig) error {
	// Set pod replicas
	if config.Replicas != nil {
		if err := unstructured.SetNestedField(obj.Object, int64(*config.Replicas), "spec", "replicas"); err != nil {
			return errors.Wrapf(err, "Failed to set replica value")
		}
	}

	// Set container resources
	if config.Resources != nil {
		err := setContainerAttrWithFn(obj, func(container map[string]interface{}) error {
			if err := unstructured.SetNestedField(container, util.ToMap(config.Resources), "resources"); err != nil {
				return errors.Wrapf(err, "Failed to set container resources")
			}

			return nil
		})
		if err != nil {
			return err
		}
	}

	// Set --log-level flag
	if config.LogLevel != nil {
		if err := setContainerArg(obj, LogLevelArg, string(*config.LogLevel)); err != nil {
			return err
		}
	}

	// Set --metrics-backend flag to Prometheus (this flag can be set more than
	// once for additional metrics providers)
	if err := setContainerArg(obj, "--metrics-backend", "prometheus"); err != nil {
		return err
	}

	// Set container arguments, skipping over any that are deny listed
	argDenyList := []string{
		"port",
		"prometheus-port",
		"health-addr",
		"validating-webhook-configuration-name",
		"mutating-webhook-configuration-name",
		"disable-cert-rotation",
		"client-cert-name",
		"tls-min-version",
	}

	for _, arg := range config.ContainerArguments {
		if slices.Contains(argDenyList, arg.Name) {
			log.Info(fmt.Sprintf("Argument --%s is deny listed and won't be applied.", arg.Name))

			continue
		}

		err := setContainerArg(obj, "--"+arg.Name, arg.Value)
		if err != nil {
			return err
		}
	}

	return nil
}

func removeAnnotations(obj *unstructured.Unstructured) error {
	err := unstructured.SetNestedField(
		obj.Object, map[string]interface{}{}, "spec", "template", "metadata", "annotations",
	)
	if err != nil {
		return errors.Wrapf(err, "Failed to remove annotations")
	}

	return nil
}

// openShiftDeploymentOverrides will remove runAsUser, runAsGroup, and seccompProfile on every container in the
// Deployment manifest. The seccompProfile is removed for backwards compatibility with OpenShift <= v4.10. Setting
// seccompProfile=runtime/default in such versions explicitly disqualified the workload from the restricted SCC.
// In OpenShift v4.11+, any workload running in a namespace prefixed with "openshift-*" must use the "restricted"
// profile unless there is a ClusterServiceVersion present, which is not the case for the Gatekeeper operand namespace.
// Add --disable-cert-rotation arguments
func openShiftDeploymentOverrides(obj *unstructured.Unstructured) error {
	containers, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil {
		return errors.Wrapf(err, "Failed to parse the deployment's containers")
	}

	for i := range containers {
		container, ok := containers[i].(map[string]interface{})
		if !ok {
			continue
		}

		unstructured.RemoveNestedField(container, "securityContext", "runAsUser")
		unstructured.RemoveNestedField(container, "securityContext", "runAsGroup")
		unstructured.RemoveNestedField(container, "securityContext", "seccompProfile")

		args, _, err := unstructured.NestedStringSlice(container, "args")
		if err != nil {
			return errors.Wrapf(err, "Failed to get container args in OpenShift overrides")
		}

		// Clean up if the cert-rotation is set explicitly
		args = slices.DeleteFunc(args, func(arg string) bool {
			return arg == "--disable-cert-rotation=false"
		})

		hasDisabledCert := slices.ContainsFunc(args, func(arg string) bool {
			return arg == "--disable-cert-rotation" || arg == "--disable-cert-rotation=true"
		})

		if !hasDisabledCert {
			args = append(args, "--disable-cert-rotation")

			err = unstructured.SetNestedStringSlice(container, args, "args")
			if err != nil {
				return errors.Wrapf(err, "Failed to set the OpenShift override to disable cert rotation")
			}
		}

		containers[i] = container
	}

	err = unstructured.SetNestedField(obj.Object, containers, "spec", "template", "spec", "containers")
	if err != nil {
		return errors.Wrapf(err, "Failed to set the OpenShift overrides for the Deployment containers")
	}

	const volumeErrMsg = "Failed to set the certificate volume mount for OpenShift"

	volumes, _, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "volumes")
	if err != nil || len(volumes) == 0 {
		return errors.Wrapf(err, volumeErrMsg)
	}

	vol, ok := volumes[0].(map[string]interface{})
	if !ok {
		return errors.Wrapf(err, "Failed to parse volumes")
	}

	err = unstructured.SetNestedField(vol, OpenshiftSecretName, "secret", "secretName")
	if err != nil {
		return errors.Wrapf(err, volumeErrMsg)
	}

	volumes[0] = vol

	err = unstructured.SetNestedField(obj.Object, volumes, "spec", "template", "spec", "volumes")
	if err != nil {
		return errors.Wrapf(err, volumeErrMsg)
	}

	return nil
}

func setMutationFlags(obj *unstructured.Unstructured, webhookConfig *operatorv1alpha1.WebhookConfig) error {
	if webhookConfig == nil {
		return nil
	}

	if webhookConfig.LogMutations != nil && webhookConfig.LogMutations.ToBool() {
		err := setContainerArg(obj, LogMutationsArg, webhookConfig.LogMutations.ToBoolString())
		if err != nil {
			return err
		}
	}

	if webhookConfig.MutationAnnotations != nil && webhookConfig.MutationAnnotations.ToBool() {
		return setContainerArg(
			obj, MutationAnnotationsArg, webhookConfig.MutationAnnotations.ToBoolString(),
		)
	}

	return nil
}

// Default is Disabled (false)
func setLogDeniesFlag(obj *unstructured.Unstructured, logDenies *operatorv1alpha1.Mode) error {
	if logDenies != nil && logDenies.ToBool() {
		err := setContainerArg(obj, LogDeniesArg, logDenies.ToBoolString())
		if err != nil {
			return err
		}
	}

	return nil
}

func setAuditInterval(obj *unstructured.Unstructured, auditInterval *metav1.Duration) error {
	if auditInterval != nil {
		return setContainerArg(
			obj, AuditIntervalArg, fmt.Sprint(auditInterval.Round(time.Second).Seconds()),
		)
	}

	return nil
}

func setConstraintViolationLimit(obj *unstructured.Unstructured, constraintViolationLimit *uint64) error {
	if constraintViolationLimit != nil {
		return setContainerArg(
			obj, ConstraintViolationLimitArg, strconv.FormatUint(*constraintViolationLimit, 10),
		)
	}

	return nil
}

func setAuditFromCache(obj *unstructured.Unstructured, auditFromCache *operatorv1alpha1.AuditFromCacheMode) error {
	if auditFromCache != nil {
		auditFromCacheValue := "false"
		if *auditFromCache == operatorv1alpha1.AuditFromCacheEnabled ||
			*auditFromCache == operatorv1alpha1.AuditFromCacheAutomatic {
			auditFromCacheValue = "true"
		}

		return setContainerArg(obj, AuditFromCacheArg, auditFromCacheValue)
	}

	return nil
}

func setAuditChunkSize(obj *unstructured.Unstructured, auditChunkSize *uint64) error {
	if auditChunkSize != nil {
		return setContainerArg(obj, AuditChunkSizeArg, strconv.FormatUint(*auditChunkSize, 10))
	}

	return nil
}

func setEmitEvents(obj *unstructured.Unstructured, argName string, emitEvents *operatorv1alpha1.Mode) error {
	if emitEvents != nil {
		emitArgValue := emitEvents.ToBoolString()

		return setContainerArg(obj, argName, emitArgValue)
	}

	return nil
}

func setEventsInvolvedNamespace(obj *unstructured.Unstructured,
	argName string, eventsInvolvedNs *operatorv1alpha1.Mode,
) error {
	if eventsInvolvedNs != nil {
		emitEventsInvolvedNsArgValue := eventsInvolvedNs.ToBoolString()

		return setContainerArg(obj, argName, emitEventsInvolvedNsArgValue)
	}

	return nil
}

func setDisabledBuiltins(obj *unstructured.Unstructured, disabledBuiltins []string) error {
	for _, b := range disabledBuiltins {
		if err := setContainerArg(obj, DisabledBuiltinArg, b); err != nil {
			return err
		}
	}

	return nil
}

func setEnableMutation(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	if !mutatingWebhookEnabled(spec.MutatingWebhook) {
		switch obj.GetName() {
		case AuditDeploymentName:
			return unsetContainerArg(obj, OperationArg, OperationMutationStatus)
		case WebhookDeploymentName:
			return unsetContainerArg(obj, OperationArg, OperationMutationWebhook)
		default:
			return nil
		}
	} else {
		switch obj.GetName() {
		case AuditDeploymentName:
			return setContainerArg(obj, OperationArg, OperationMutationStatus)
		case WebhookDeploymentName:
			return setContainerArg(obj, OperationArg, OperationMutationWebhook)
		default:
			return nil
		}
	}
}

func setWebhookConfigurationWithFn(
	obj *unstructured.Unstructured, webhookName string, webhookFn func(map[string]interface{}) error,
) error {
	webhooks, found, err := unstructured.NestedSlice(obj.Object, "webhooks")
	if err != nil || !found {
		return errors.Wrapf(err, "Failed to retrieve webhooks definition")
	}

	for _, w := range webhooks {
		webhook := w.(map[string]interface{})
		if webhook["name"] == webhookName {
			if err := webhookFn(webhook); err != nil {
				return err
			}
		}
	}

	if err := unstructured.SetNestedSlice(obj.Object, webhooks, "webhooks"); err != nil {
		return errors.Wrapf(err, "Failed to set webhooks")
	}

	return nil
}

func setFailurePolicy(
	obj *unstructured.Unstructured, failurePolicy *admregv1.FailurePolicyType, webhookName string,
) error {
	if failurePolicy == nil {
		return nil
	}

	setFailurePolicyFn := func(webhook map[string]interface{}) error {
		if err := unstructured.SetNestedField(webhook, string(*failurePolicy), "failurePolicy"); err != nil {
			return errors.Wrapf(err, "Failed to set webhook failure policy")
		}

		return nil
	}

	return setWebhookConfigurationWithFn(obj, webhookName, setFailurePolicyFn)
}

func setNamespaceSelector(
	obj *unstructured.Unstructured, namespaceSelector *metav1.LabelSelector, gatekeeperNamespace, webhookName string,
) error {
	// If no namespaceSelector is provided, override usage of the default Gatekeeper namespace.
	if namespaceSelector == nil {
		// Don't perform any overrides if no overrides are set and the default namespaceSelector can't be parsed
		webhooks, ok := obj.Object["webhooks"].([]interface{})
		if !ok || len(webhooks) == 0 {
			// Return nil since an invalid object is unrecoverable
			return nil
		}

		for _, webhook := range webhooks {
			webhookMap, ok := webhook.(map[string]interface{})
			if !ok {
				continue
			}

			curHookName, ok := webhookMap["name"].(string)
			if !ok || curHookName != webhookName {
				continue
			}

			namespaceSelectorUntyped, ok := webhookMap["namespaceSelector"].(map[string]interface{})
			if !ok {
				// Return nil since an invalid namespaceSelector is unrecoverable
				return nil
			}

			namespaceSelectorBytes, err := json.Marshal(namespaceSelectorUntyped)
			if err != nil {
				// Return nil since invalid JSON is unrecoverable
				return nil //nolint:nilerr
			}

			namespaceSelectorTyped := &metav1.LabelSelector{}

			err = json.Unmarshal(namespaceSelectorBytes, namespaceSelectorTyped)
			if err != nil {
				// Return nil since invalid JSON is unrecoverable
				return nil //nolint:nilerr
			}

			for i, expression := range namespaceSelectorTyped.MatchExpressions {
				for j, value := range expression.Values {
					if value == util.DefaultGatekeeperNamespace {
						namespaceSelectorTyped.MatchExpressions[i].Values[j] = gatekeeperNamespace

						namespaceSelector = namespaceSelectorTyped
					}
				}
			}

			break
		}
	}

	// If namespaceSelector is still nil, then there are no overrides to perform.
	if namespaceSelector == nil {
		return nil
	}

	setNamespaceSelectorFn := func(webhook map[string]interface{}) error {
		if err := unstructured.SetNestedField(webhook, util.ToMap(namespaceSelector), "namespaceSelector"); err != nil {
			return errors.Wrapf(err, "Failed to set webhook namespace selector")
		}

		return nil
	}

	return setWebhookConfigurationWithFn(obj, webhookName, setNamespaceSelectorFn)
}

func setOperations(
	obj *unstructured.Unstructured, operations []operatorv1alpha1.OperationType, webhookName string,
) error {
	// If no operations is provided, no override for operations
	if operations == nil {
		return nil
	}

	setOperationsFn := func(webhook map[string]interface{}) error {
		rules := webhook["rules"].([]interface{})
		if len(rules) == 0 {
			return nil
		}

		converted := make([]interface{}, 0, len(operations))
		for _, op := range operations {
			converted = append(converted, string(op))
		}

		for i, r := range rules {
			firstRuleObj := r.(map[string]interface{})
			firstRuleObj["operations"] = converted
			rules[i] = firstRuleObj
		}

		if err := unstructured.SetNestedSlice(webhook, rules, "rules"); err != nil {
			return errors.Wrapf(err, "Failed to set webhook operations")
		}

		return nil
	}

	return setWebhookConfigurationWithFn(obj, webhookName, setOperationsFn)
}

// Generic setters

func setAffinity(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	if spec.Affinity != nil {
		err := unstructured.SetNestedField(
			obj.Object, util.ToMap(spec.Affinity), "spec", "template", "spec", "affinity",
		)
		if err != nil {
			return errors.Wrapf(err, "Failed to set affinity value")
		}
	}

	return nil
}

func setNodeSelector(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	if spec.NodeSelector != nil {
		err := unstructured.SetNestedStringMap(
			obj.Object, spec.NodeSelector, "spec", "template", "spec", "nodeSelector",
		)
		if err != nil {
			return errors.Wrapf(err, "Failed to set nodeSelector value")
		}
	}

	return nil
}

func setPodAnnotations(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	if spec.PodAnnotations != nil {
		err := unstructured.SetNestedStringMap(
			obj.Object, spec.PodAnnotations, "spec", "template", "metadata", "annotations",
		)
		if err != nil {
			return errors.Wrapf(err, "Failed to set podAnnotations")
		}
	}

	return nil
}

func setTolerations(obj *unstructured.Unstructured, spec operatorv1alpha1.GatekeeperSpec) error {
	if spec.Tolerations != nil {
		tolerations := make([]interface{}, len(spec.Tolerations))
		for i, t := range spec.Tolerations {
			tolerations[i] = util.ToMap(t)
		}

		err := unstructured.SetNestedSlice(obj.Object, tolerations, "spec", "template", "spec", "tolerations")
		if err != nil {
			return errors.Wrapf(err, "Failed to set container tolerations")
		}
	}

	return nil
}

// Container specific setters

func setImage(container map[string]interface{}, spec operatorv1alpha1.GatekeeperSpec) error {
	image := os.Getenv(GatekeeperImageEnvVar)
	if image != "" {
		if err := unstructured.SetNestedField(container, image, "image"); err != nil {
			return errors.Wrapf(err, "Failed to set container image")
		}
	}
	// else should only happen in dev/test environments, in which case use the
	// default image in the Gatekeeper deployment manifests i.e. no overrides.

	if spec.Image == nil {
		return nil
	}

	if spec.Image.ImagePullPolicy != nil {
		err := unstructured.SetNestedField(container, string(*spec.Image.ImagePullPolicy), "imagePullPolicy")
		if err != nil {
			return errors.Wrapf(err, "Failed to set container image pull policy")
		}
	}

	return nil
}

func setContainerAttrWithFn(obj *unstructured.Unstructured, containerFn func(map[string]interface{}) error) error {
	containers, found, err := unstructured.NestedSlice(obj.Object, "spec", "template", "spec", "containers")
	if err != nil || !found {
		return errors.Wrapf(err, "Failed to retrieve containers")
	}

	for _, c := range containers {
		container := c.(map[string]interface{})
		if name, found, err := unstructured.NestedString(container, "name"); err != nil || !found {
			return errors.Wrapf(err, "Unable to retrieve container: %s", name)
		} else if name == managerContainer {
			if err := containerFn(container); err != nil {
				return err
			}
		}
	}

	err = unstructured.SetNestedSlice(obj.Object, containers, "spec", "template", "spec", "containers")
	if err != nil {
		return errors.Wrapf(err, "Failed to set containers")
	}

	return nil
}

// updateContainerArg takes an object, container flag argument name and value to
// set on the object, and a boolean whether the flag should be deleted, and
// returns any resulting errors.
func updateContainerArg(
	obj *unstructured.Unstructured, argName string, argValue string, remove bool,
) error {
	return setContainerAttrWithFn(obj, func(container map[string]interface{}) error {
		args, found, err := unstructured.NestedStringSlice(container, "args")
		if !found || err != nil {
			return errors.Wrapf(err, "Unable to retrieve container arguments for: %s", managerContainer)
		}

		// Determine whether this is a flag that can be declared more than once.
		multiArgFlags := []string{
			"--operation",
			"--disable-opa-builtin",
			"--exempt-namespace",
			"--exempt-namespace-prefix",
			"--exempt-namespace-suffix",
			"--metrics-backend",
		}
		isMultiArg := slices.Contains(multiArgFlags, argName)

		index := -1
		for i, arg := range args {
			n, v := util.FromArg(arg)
			if n == argName && (!isMultiArg || isMultiArg && v == argValue) {
				index = i

				break
			}
		}

		if index != -1 && remove {
			args = slices.Delete(args, index, index+1)
		} else if index == -1 && !remove {
			args = append(args, util.ToArg(argName, argValue))
		}

		return unstructured.SetNestedStringSlice(container, args, "args")
	})
}

// setContainerArg takes an object, and container flag argument name and value
// to set on the object, and returns any resulting errors.
func setContainerArg(
	obj *unstructured.Unstructured, argName string, argValue string,
) error {
	return updateContainerArg(obj, argName, argValue, false)
}

// unsetContainerArg takes an object, and container flag argument name and value
// to unset from the object, and returns any resulting errors.
func unsetContainerArg(
	obj *unstructured.Unstructured, argName string, argValue string,
) error {
	return updateContainerArg(obj, argName, argValue, true)
}

func setNamespace(obj *unstructured.Unstructured, asset, namespace string) error {
	if obj.GetNamespace() != "" {
		obj.SetNamespace(namespace)
	}

	if err := setClientConfigNamespace(obj, asset, namespace); err != nil {
		return err
	}

	if err := setControllerManagerExemptNamespace(obj, asset, namespace); err != nil {
		return err
	}

	return setRoleBindingSubjectNamespace(obj, asset, namespace)
}

func setClientConfigNamespace(obj *unstructured.Unstructured, asset, namespace string) error {
	if asset != ValidatingWebhookConfiguration && asset != MutatingWebhookConfiguration {
		return nil
	}

	webhooks, found, err := unstructured.NestedSlice(obj.Object, "webhooks")
	if err != nil || !found {
		return errors.Wrapf(err, "Failed to retrieve webhooks definition")
	}

	for _, w := range webhooks {
		webhook := w.(map[string]interface{})
		if err := unstructured.SetNestedField(webhook, namespace, "clientConfig", "service", "namespace"); err != nil {
			return errors.Wrapf(err, "Failed to set webhook clientConfig.service.namespace")
		}
	}

	if err := unstructured.SetNestedSlice(obj.Object, webhooks, "webhooks"); err != nil {
		return errors.Wrapf(err, "Failed to set webhooks")
	}

	return nil
}

func setControllerManagerExemptNamespace(obj *unstructured.Unstructured, asset, namespace string) error {
	if asset != WebhookFile {
		return nil
	}

	return setContainerArg(obj, ExemptNamespaceArg, namespace)
}

func setRoleBindingSubjectNamespace(obj *unstructured.Unstructured, asset, namespace string) error {
	if asset != ClusterRoleBindingFile && asset != RoleBindingFile {
		return nil
	}

	subjects, found, err := unstructured.NestedSlice(obj.Object, "subjects")
	if !found || err != nil {
		return errors.Wrapf(err, "Failed to retrieve subjects from roleBinding")
	}

	for _, s := range subjects {
		subject := s.(map[string]interface{})
		if err := unstructured.SetNestedField(subject, namespace, "namespace"); err != nil {
			return errors.Wrapf(err, "Failed to set namespace for rolebinding subject")
		}
	}

	if err := unstructured.SetNestedSlice(obj.Object, subjects, "subjects"); err != nil {
		return errors.Wrapf(err, "Failed to set updated subjects in rolebinding")
	}

	return nil
}
