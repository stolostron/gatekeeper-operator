package controllers

import (
	"context"
	"reflect"
	"sort"
	"time"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/apis/status/v1beta1"
	"github.com/pkg/errors"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

var ControllerName = "constraintstatus_reconciler"

type ConstraintPodStatusReconciler struct {
	client.Client
	Scheme        *runtime.Scheme
	Log           logr.Logger
	DynamicClient *dynamic.DynamicClient
	Namespace     string
	// This includes api-resources list and it finds a missing version of resources.
	DiscoveryStorage *DiscoveryStorage
	// key = constraintPodName
	ConstraintToSyncOnly map[string][]v1alpha1.SyncOnlyEntry
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConstraintPodStatusReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: int(1)}).
		Named(ControllerName).
		For(&v1beta1.ConstraintPodStatus{},
			builder.WithPredicates(predicate.Funcs{
				// Execute this reconcile func when it is audit-constraintStatuspod
				// because a constraint creates 4 constraintPodstatus
				CreateFunc: func(e event.CreateEvent) bool {
					obj := e.Object.(*v1beta1.ConstraintPodStatus)

					return slices.Contains(obj.Status.Operations, "audit")
				},
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldObj := e.ObjectOld.(*v1beta1.ConstraintPodStatus)
					newObj := e.ObjectNew.(*v1beta1.ConstraintPodStatus)

					return slices.Contains(newObj.Status.Operations, "audit") &&
						// Update when the constraint is refreshed
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

// When spec.audit.auditFromCache is set to Automatic,
// Reconcile analyzes the constraint associated with the ConstraintPodStatus reconcile request.
// The kinds used in the constraint's match configuration is used to configure the syncOnly option.
func (r *ConstraintPodStatusReconciler) Reconcile(ctx context.Context,
	request reconcile.Request,
) (reconcile.Result, error) {
	log := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	log.Info("Reconciling ConstraintPodStatus and Config")
	// This is used for RequeueAfter
	var requeueTime time.Duration

	gatekeeper := &operatorv1alpha1.Gatekeeper{}
	// Get gatekeeper resource
	err := r.Get(ctx, types.NamespacedName{
		Namespace: "",
		Name:      "gatekeeper",
	}, gatekeeper)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.Error(err, "Gatekeeper resource is not found")

			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	// Get config or create if not exist
	config := &v1alpha1.Config{}
	err = r.Get(ctx, types.NamespacedName{
		Namespace: r.Namespace,
		Name:      "config",
	}, config)

	if err != nil {
		if apierrors.IsNotFound(err) {
			config = &v1alpha1.Config{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "config",
					Namespace: r.Namespace,
				},
			}

			createErr := r.Create(ctx, config)
			if createErr != nil {
				log.Error(err, "Fail to create the Gatekeeper Config object, will retry.")

				return reconcile.Result{}, createErr
			}

			log.Info("The Gatekeeper Config object was created")
		} else {
			return reconcile.Result{}, err
		}
	}

	constraintPodStatus := &v1beta1.ConstraintPodStatus{}

	err = r.Get(ctx, request.NamespacedName, constraintPodStatus)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(1).Info("Cannot find the ConstraintPodStatus")

			err = r.handleDeleteEvent(ctx, request.Name, config)
			if err != nil {
				return reconcile.Result{}, err
			}

			return reconcile.Result{}, nil
		}
		// Requeue
		return reconcile.Result{}, err
	}

	constraint, constraintName, err := getConstraint(ctx, *constraintPodStatus, r.DynamicClient)
	if err != nil {
		if apierrors.IsNotFound(err) {
			r.Log.Info("The Constraint was not found", "constraintName:", constraintName)

			return reconcile.Result{}, nil
		}

		return reconcile.Result{}, err
	}

	constraintMatchKinds, _, err := unstructured.NestedSlice(constraint.Object, "spec", "match", "kinds")
	if err != nil {
		r.Log.V(1).Info("There are no provided kinds in the Constraint", "constraintName:", constraintName)

		err = r.handleDeleteEvent(ctx, request.Name, config)
		if err != nil {
			return reconcile.Result{}, err
		}

		return reconcile.Result{}, nil
	}

	constraintSyncOnlyEntries, err := r.DiscoveryStorage.getSyncOnlys(constraintMatchKinds)
	if err != nil {
		if errors.Is(err, ErrNotFoundDiscovery) {
			r.Log.V(1).Info("Cannot find matched discovery. Requeue after 10 secs")

			requeueTime = time.Second * 10
		} else {
			log.Error(err, "Error to get matching kind and apigroup")

			return reconcile.Result{}, err
		}
	}

	r.ConstraintToSyncOnly[request.Name] = constraintSyncOnlyEntries

	uniqSyncOnly := r.getUniqSyncOnly()

	if reflect.DeepEqual(uniqSyncOnly, config.Spec.Sync.SyncOnly) {
		r.Log.V(1).Info("There are no changes detected. Cancel Updating")

		return reconcile.Result{RequeueAfter: requeueTime}, nil
	}

	config.Spec.Sync.SyncOnly = uniqSyncOnly

	err = r.Update(ctx, config, &client.UpdateOptions{})
	if err != nil {
		log.Error(err, "unable to update config syncOnly")

		return reconcile.Result{}, err
	}

	return reconcile.Result{RequeueAfter: requeueTime}, nil
}

func (r *ConstraintPodStatusReconciler) getUniqSyncOnly() []v1alpha1.SyncOnlyEntry {
	syncOnlySet := map[v1alpha1.SyncOnlyEntry]bool{}
	// Add to table for unique filtering
	for _, syncEntries := range r.ConstraintToSyncOnly {
		for _, entry := range syncEntries {
			syncOnlySet[entry] = true
		}
	}

	syncOnlys := make([]v1alpha1.SyncOnlyEntry, 0, len(syncOnlySet))
	for key := range syncOnlySet {
		syncOnlys = append(syncOnlys, key)
	}

	// Sort syncOnly so the returned value is consistent each time the method is called.
	sort.Slice(syncOnlys, func(i, j int) bool {
		stringi := syncOnlys[i].Group + " " + syncOnlys[i].Kind + " " + syncOnlys[i].Version
		stringj := syncOnlys[j].Group + " " + syncOnlys[j].Kind + " " + syncOnlys[j].Version

		return stringi < stringj
	})

	return syncOnlys
}

// handleDeleteEvent is called when a ConstraintPodStatus object is deleted.
// It deletes ConstraintPodStatus' key in the `ConstraintToSyncOnly` map and
// recalculates the appropriate SyncOnly entries.
func (r *ConstraintPodStatusReconciler) handleDeleteEvent(
	ctx context.Context, cpsName string, config *v1alpha1.Config,
) error {
	delete(r.ConstraintToSyncOnly, cpsName)

	updatedSyncOnly := r.getUniqSyncOnly()

	if reflect.DeepEqual(updatedSyncOnly, config.Spec.Sync.SyncOnly) {
		r.Log.V(1).Info("There are no changes detected. Will not update.")

		return nil
	}

	config.Spec.Sync.SyncOnly = updatedSyncOnly

	err := r.Update(ctx, config, &client.UpdateOptions{})
	if err != nil {
		r.Log.Error(err, "unable to update config syncOnly")

		return err
	}

	return nil
}
