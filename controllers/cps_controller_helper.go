package controllers

import (
	"context"
	"strings"

	gkv1alpha1 "github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	gkv1beta1 "github.com/open-policy-agent/gatekeeper/v3/apis/status/v1beta1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/utils/strings/slices"
	ctrl "sigs.k8s.io/controller-runtime"
	cacheRuntime "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	"github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
)

var setupLog = ctrl.Log.WithName("setup")

func (r *GatekeeperReconciler) handleCPSController(ctx context.Context,
	gatekeeper *operatorv1alpha1.Gatekeeper,
) error {
	isCRDReady, err := checkCrdAvailable(ctx, r.DynamicClient,
		"ConstraintPodStatus", "constraintpodstatuses.status.gatekeeper.sh")
	if err != nil {
		return err
	}

	if !isCRDReady {
		return errCrdNotReady
	}

	isAutomaticOn := checkCPScontrollerPrereqs(gatekeeper)

	// auditFromCache is not set to Automatic, so stop the existing ConstraintPodStatus controller
	if !isAutomaticOn {
		if r.isCPSCtrlRunning && r.cpsCtrlCtxCancel != nil {
			setupLog.Info("Gatekeeper auditFromCache unset from Automatic. Stopping the ConstraintPodStatus manager.")
			r.cpsCtrlCtxCancel()
		}

		return nil
	}

	if r.isCPSCtrlRunning {
		return nil
	}

	var cpsCtrlCtx context.Context

	cpsCtrlCtx, r.cpsCtrlCtxCancel = context.WithCancel(ctx)

	cpsMgr, err := ctrl.NewManager(r.KubeConfig, ctrl.Options{
		Scheme: r.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
		LeaderElection:   r.EnableLeaderElection,
		LeaderElectionID: "5ff985ccc.constraintstatuspod.gatekeeper.sh",
		Cache: cacheRuntime.Options{
			ByObject: map[client.Object]cacheRuntime.ByObject{
				&gkv1beta1.ConstraintPodStatus{}: {
					Transform: func(obj interface{}) (interface{}, error) {
						constraintStatus := obj.(*gkv1beta1.ConstraintPodStatus)
						// Only cache fields that are utilized by the controllers.
						guttedObj := &gkv1beta1.ConstraintPodStatus{
							TypeMeta: constraintStatus.TypeMeta,
							ObjectMeta: metav1.ObjectMeta{
								Name:      constraintStatus.Name,
								Labels:    constraintStatus.Labels,
								Namespace: constraintStatus.Namespace,
							},
							Status: gkv1beta1.ConstraintPodStatusStatus{
								ObservedGeneration: constraintStatus.Status.ObservedGeneration,
								Operations:         constraintStatus.Status.Operations,
							},
						}

						return guttedObj, nil
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "Failed to setup NewManager for ConstraintPodStatus controller")

		return err
	}

	constraintToSyncOnly := r.getConstraintToSyncOnly(ctx)

	if err := (&ConstraintPodStatusReconciler{
		Scheme:               r.Scheme,
		Client:               cpsMgr.GetClient(),
		DynamicClient:        r.DynamicClient,
		Log:                  ctrl.Log.WithName("ConstraintPodStatus"),
		Namespace:            r.Namespace,
		ConstraintToSyncOnly: constraintToSyncOnly,
		DiscoveryStorage:     r.DiscoveryStorage,
	}).SetupWithManager(cpsMgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ConstraintPodStatus")

		return err
	}

	r.isCPSCtrlRunning = true
	r.subControllerWait.Add(1)

	// Use another go routine for the ConstraintPodStatus controller
	go func() {
		err := cpsMgr.Start(cpsCtrlCtx)
		if err != nil {
			setupLog.Error(err, "A problem running ConstraintPodStatus manager. Triggering a reconcile to restart it.")
		}

		defer r.cpsCtrlCtxCancel()

		r.isCPSCtrlRunning = false

		// In case it is not an error and a child context is cancelled
		// because the auditFromCache changed from Automatic,
		// sending this channel avoids encountering a race condition.
		// If the error happens when cpsMgr start, it will retry to start cpsMgr
		r.ManualReconcileTrigger <- event.GenericEvent{
			Object: &unstructured.Unstructured{
				Object: map[string]interface{}{
					"apiVersion": v1alpha1.GroupVersion.String(),
					"kind":       "Gatekeeper",
					"metadata": map[string]interface{}{
						"name": defaultGatekeeperCrName,
					},
				},
			},
		}

		r.subControllerWait.Done()
	}()

	return nil
}

func (r *GatekeeperReconciler) getConstraintToSyncOnly(ctx context.Context) map[string][]gkv1alpha1.SyncOnlyEntry {
	cpsGVR := schema.GroupVersionResource{
		Group:    "status.gatekeeper.sh",
		Version:  "v1beta1",
		Resource: "constraintpodstatuses",
	}

	// key = ConstraintPodStatus Name
	constraintToSyncOnly := map[string][]gkv1alpha1.SyncOnlyEntry{}

	unstructCpsList, err := r.DynamicClient.Resource(cpsGVR).List(ctx, metav1.ListOptions{})
	if err != nil {
		return constraintToSyncOnly
	}

	// Add to table for unique filtering
	for _, cps := range unstructCpsList.Items {
		operations, found, err := unstructured.NestedStringSlice(cps.Object, "status", "operations")
		if !found || err != nil {
			r.Log.Error(err, "Failed to parse status.operations from ConstraintPodStatus "+cps.GetName())

			continue
		}

		// Pick only Audit ConstraintPodStatus
		if !slices.Contains(operations, "audit") {
			continue
		}

		constraint, constraintName, err := getConstraint(ctx, cps.GetLabels(), r.DynamicClient)
		if err != nil {
			if apierrors.IsNotFound(err) {
				r.Log.Info("The Constraint was not found", "constraintName:", constraintName)
			}

			continue
		}

		constraintMatchKinds, _, err := unstructured.NestedSlice(constraint.Object, "spec", "match", "kinds")
		if err != nil {
			r.Log.V(1).Info("There are no provided kinds in the Constraint", "constraintName:", constraintName)

			continue
		}

		constraintSyncOnlyEntries, err := r.DiscoveryStorage.getSyncOnlys(constraintMatchKinds)
		if err != nil {
			// No need to retry. The ConstraintPodStatus_controller will sort out
			continue
		}

		constraintToSyncOnly[cps.GetName()] = constraintSyncOnlyEntries
	}

	return constraintToSyncOnly
}

// Helper function to get constraint from ConstraintPodStatus
func getConstraint(ctx context.Context, labels map[string]string,
	dynamicClient *dynamic.DynamicClient,
) (*unstructured.Unstructured, string, error) {
	constraintKind := labels["internal.gatekeeper.sh/constraint-kind"]
	constraintName := labels["internal.gatekeeper.sh/constraint-name"]

	constraintGVR := schema.GroupVersionResource{
		Group:    "constraints.gatekeeper.sh",
		Version:  "v1beta1",
		Resource: strings.ToLower(constraintKind),
	}

	constraint, err := dynamicClient.Resource(constraintGVR).Get(ctx, constraintName, metav1.GetOptions{})
	if err != nil {
		return nil, constraintName, err
	}

	return constraint, constraintName, nil
}

// Check gatekeeper auditFromCache=Automatic
func checkCPScontrollerPrereqs(gatekeeper *operatorv1alpha1.Gatekeeper) bool {
	return gatekeeper.Spec.Audit != nil && gatekeeper.Spec.Audit.AuditFromCache != nil &&
		*gatekeeper.Spec.Audit.AuditFromCache == operatorv1alpha1.AuditFromCacheAutomatic
}
