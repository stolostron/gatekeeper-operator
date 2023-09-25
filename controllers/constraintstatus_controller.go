package controllers

import (
	"context"
	"fmt"
	"strings"
	"time"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/apis/status/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// +kubebuilder:rbac:groups=config.gatekeeper.sh,resources=configs/status,verbs=get;update;list;create;patch
// +kubebuilder:rbac:groups=constraints.gatekeeper.sh,resources=*,verbs=get;list;watch;create;update;patch;delete
var (
	ControllerName = "contraintstatus_reconciler"
	configGVK      = schema.GroupVersionResource{
		Group:    v1alpha1.GroupVersion.Group,
		Version:  v1alpha1.GroupVersion.Version,
		Resource: "config",
	}
)

type discoveryInfo struct {
	apiResourceList        []*metav1.APIResourceList
	discoveryLastRefreshed time.Time
}

type constraintKind struct {
	apiGroups []string
	kinds     []string
}

type ConstraintStatusReconciler struct {
	client.Client
	Log                     logr.Logger
	DynamicClient           dynamic.Interface
	ClientSet               *kubernetes.Clientset
	Scheme                  *runtime.Scheme
	Namespace               string
	MaxConcurrentReconciles uint
	discoveryInfo
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConstraintStatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		// The work queue prevents the same item being reconciled concurrently:
		// https://github.com/kubernetes-sigs/controller-runtime/issues/1416#issuecomment-899833144
		WithOptions(controller.Options{MaxConcurrentReconciles: int(r.MaxConcurrentReconciles)}).
		Owns(&v1alpha1.Config{}).
		Named(ControllerName).
		For(&v1beta1.ConstraintPodStatus{},
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool {
					obj := e.Object.(*v1beta1.ConstraintPodStatus)

					return slices.Contains(obj.Status.Operations, "audit")
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldObj := e.ObjectOld.(*v1beta1.ConstraintPodStatus)
					newObj := e.ObjectNew.(*v1beta1.ConstraintPodStatus)

					return slices.Contains(newObj.Status.Operations, "audit") &&
						oldObj.Status.ObservedGeneration != newObj.Status.ObservedGeneration
				},
				DeleteFunc: func(e event.DeleteEvent) bool {
					obj := e.Object.(*v1beta1.ConstraintPodStatus)

					return slices.Contains(obj.Status.Operations, "audit")
				},
			},
			)).
		Complete(r)
}

// Reconcile implements reconcile.Reconciler.
func (r *ConstraintStatusReconciler) Reconcile(ctx context.Context,
	request reconcile.Request,
) (reconcile.Result, error) {
	log := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	log.Info("Reconciling ConstraintPodStatus and Config")

	gatekeeper := &operatorv1alpha1.Gatekeeper{}
	err := r.Get(ctx, types.NamespacedName{
		Namespace: "",
		Name:      "gatekeeper",
	}, gatekeeper)

	auditFromCache := *gatekeeper.Spec.Audit.AuditFromCache

	if err != nil || !strings.EqualFold(string(auditFromCache), "Automatic") {
		return reconcile.Result{Requeue: false}, fmt.Errorf("auditFromCache is not set so skip this reconcile")
	}

	// Get config or create if not exist
	config := &v1alpha1.Config{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: r.Namespace,
		Name:      "config",
	}, config)

	if apierrors.IsNotFound(err) {
		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config",
				Namespace: r.Namespace,
			},
			Spec: v1alpha1.ConfigSpec{
				Validation: v1alpha1.Validation{},
				Sync: v1alpha1.Sync{
					SyncOnly: []v1alpha1.SyncOnlyEntry{},
				},
			},
		}
		err = r.Create(ctx, config)

		if err != nil {
			return reconcile.Result{}, err
		}
	}

	contraintStatusList := &v1beta1.ConstraintPodStatusList{}
	err = r.List(ctx, contraintStatusList, client.InNamespace(request.NamespacedName.Namespace))

	if err != nil {
		return reconcile.Result{Requeue: false}, err
	}

	constraintKindNames := map[string][]string{}

	for _, c := range contraintStatusList.Items {
		if !slices.Contains(c.Status.Operations, "audit") {
			continue
		}

		labels := c.GetLabels()
		// TODO research contranit template name always same as constraint kind
		//nolint:all
		constraintKindNames[labels["internal.gatekeeper.sh/constraint-kind"]] = append(constraintKindNames[labels["internal.gatekeeper.sh/constraint-kind"]],
			labels["internal.gatekeeper.sh/constraint-name"])
	}

	syncOnlys := []v1alpha1.SyncOnlyEntry{}

	for kind, names := range constraintKindNames {
		constraintGVR := schema.GroupVersionResource{
			Group:    "constraints.gatekeeper.sh",
			Version:  "v1beta1",
			Resource: strings.ToLower(kind),
		}
		for _, name := range names {
			constraint, err := r.DynamicClient.Resource(constraintGVR).Get(ctx, name, metav1.GetOptions{})
			if err != nil {
				return reconcile.Result{}, err
			}

			constraintKinds, _, err := unstructured.NestedSlice(constraint.Object, "spec", "match", "kinds")
			if err != nil {
				return reconcile.Result{}, err
			}

			r.addSyncOnlys(constraintKinds, &syncOnlys)
		}
	}

	deleteDuplicateSyncOnly(&syncOnlys)
	config.Spec.Sync.SyncOnly = syncOnlys
	err = r.Update(ctx, config, &client.UpdateOptions{})

	if err != nil {
		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

func deleteDuplicateSyncOnly(syncOnlys *[]v1alpha1.SyncOnlyEntry) {
	unique := []v1alpha1.SyncOnlyEntry{}

	for _, sync := range *syncOnlys {
		isUniq := true

		for _, uniq := range unique {
			if uniq.Group == sync.Group &&
				uniq.Version == sync.Version &&
				uniq.Kind == sync.Kind {
				isUniq = false

				break
			}
		}

		if isUniq {
			unique = append(unique, sync)
		}
	}

	*syncOnlys = unique
}

func (r *ConstraintStatusReconciler) addSyncOnlys(constraintKinds []interface{}, syncOnlys *[]v1alpha1.SyncOnlyEntry) {
	for _, kind := range constraintKinds {
		newKind := kind.(map[string]interface{})
		apiGroups := newKind["apiGroups"].([]interface{})
		kindsInKinds := newKind["kinds"].([]interface{})

		for _, apiGroup := range apiGroups {
			for _, kindkind := range kindsInKinds {
				version := r.getApiVersion(kindkind.(string), apiGroup.(string))

				*syncOnlys = append(*syncOnlys, v1alpha1.SyncOnlyEntry{
					Group:   apiGroup.(string),
					Version: version,
					Kind:    kindkind.(string),
				})
			}
		}
	}
}

func (r *ConstraintStatusReconciler) getApiVersion(kind string, apiGroup string) string {
	// cool time(1 min) to refresh discoveries
	if len(r.apiResourceList) == 0 ||
		r.discoveryLastRefreshed.Add(time.Minute*1).Before(time.Now()) {
		err := r.refreshDiscoveryInfo()
		r.discoveryLastRefreshed = time.Now()

		if err != nil {
			return ""
		}
	}

	for _, resc := range r.apiResourceList {
		groupVerison := strings.Split(resc.GroupVersion, "/")
		var group string
		var version string
		// consider groupversion == v1 or groupversion == app1/v1
		if len(groupVerison) == 2 {
			group = groupVerison[0]
			version = groupVerison[1]
		} else {
			version = groupVerison[0]
		}

		for _, apiResource := range resc.APIResources {
			if apiResource.Kind == kind && group == apiGroup {
				return version
			}
		}
	}

	return ""
}

func (r *ConstraintStatusReconciler) refreshDiscoveryInfo() error {
	discoveryClient := r.ClientSet.Discovery()

	apiList, err := discoveryClient.ServerPreferredResources()
	if err != nil {
		return err
	}

	r.apiResourceList = apiList

	return nil
}
