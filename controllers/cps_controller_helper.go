package controllers

import (
	"context"
	"strings"
	"time"

	"github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	gkv1alpha1 "github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/apis/status/v1beta1"
	gkv1beta1 "github.com/open-policy-agent/gatekeeper/v3/apis/status/v1beta1"
	"github.com/pkg/errors"
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
)

var (
	setupLog       = ctrl.Log.WithName("setup")
	errCrdNotReady = errors.New("CRD is not ready")
)

func (r *GatekeeperReconciler) handleCPSController(mainCtx context.Context,
	gatekeeper *operatorv1alpha1.Gatekeeper,
) error {
	isCRDReady, err := checkCPSCrdAvailable(mainCtx, r.DynamicClient)
	if err != nil {
		return err
	}

	if !isCRDReady {
		return errCrdNotReady
	}

	isAutomaticOn := checkCPScontrollerPrereqs(gatekeeper)

	// auditFromCache is not set to Automatic, so stop the existing ConstraintPodStatus controller
	if !isAutomaticOn {
		if r.isCPSCtrlRunning {
			r.StopCPSController()
		}

		return nil
	}

	if r.isCPSCtrlRunning {
		return nil
	}

	var cpsCtrlCtx context.Context

	cpsCtrlCtx, r.cpsCtrlCtxCancel = context.WithCancel(mainCtx)

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
		setupLog.Error(err, "Failed to setup NewManager for ConstraintPodStatus contoller")

		return err
	}

	constraintToSyncOnly := r.getConstraintToSyncOnly(mainCtx)

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

	// Use another go routine for the ConstraintPodStatus controller
	go func() {
		err := cpsMgr.Start(cpsCtrlCtx)
		if err != nil {
			setupLog.Error(err, "A problem running ConstraintPodStatus manager. Triggering a reconcile to restart it.")
		}

		defer r.cpsCtrlCtxCancel()

		r.cpsCtrlCtxCancel = nil
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
	}()

	return nil
}

func (r *GatekeeperReconciler) getConstraintToSyncOnly(mainCtx context.Context) map[string][]gkv1alpha1.SyncOnlyEntry {
	cpsList := &v1beta1.ConstraintPodStatusList{}

	// key = ConstraintPodStatus Name
	constraintToSyncOnly := map[string][]gkv1alpha1.SyncOnlyEntry{}

	err := r.Client.List(mainCtx, cpsList, &client.ListOptions{})
	if err != nil {
		return constraintToSyncOnly
	}

	// Add to table for unique filtering
	for _, cps := range cpsList.Items {
		// Pick only Audit ConstraintPodStatus
		if !slices.Contains(cps.Status.Operations, "audit") {
			continue
		}

		constraint, constraintName, err := getConstraint(mainCtx, cps, r.DynamicClient)
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

		constraintToSyncOnly[cps.Name] = constraintSyncOnlyEntries
	}

	return constraintToSyncOnly
}

// Helper function to get constraint from ConstraintPodStatus
func getConstraint(ctx context.Context, cps gkv1beta1.ConstraintPodStatus,
	dynamicClient *dynamic.DynamicClient,
) (*unstructured.Unstructured, string, error) {
	labels := cps.GetLabels()
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

// Check ConstraintPodStatus Crd status is "True" and type is "NamesAccepted"
func checkCPSCrdAvailable(mainCtx context.Context, dynamicClient *dynamic.DynamicClient) (bool, error) {
	crdGVR := schema.GroupVersionResource{
		Group:    "apiextensions.k8s.io",
		Version:  "v1",
		Resource: "customresourcedefinitions",
	}

	crd, err := dynamicClient.Resource(crdGVR).
		Get(mainCtx, "constraintpodstatuses.status.gatekeeper.sh", metav1.GetOptions{})
	if err != nil {
		setupLog.V(1).Info("Cannot fetch ConstraintPodStatus CRD")

		return false, err
	}

	conditions, ok, _ := unstructured.NestedSlice(crd.Object, "status", "conditions")
	if !ok {
		setupLog.V(1).Info("Cannot parse ConstraintPodStatus status conditions")

		return false, errors.New("Failed to parse status, conditions")
	}

	for _, condition := range conditions {
		parsedCondition := condition.(map[string]interface{})

		status, ok := parsedCondition["status"].(string)
		if !ok {
			setupLog.V(1).Info("Cannot parse ConstraintPodStatus conditions status")

			return false, errors.New("Failed to parse status string")
		}

		conditionType, ok := parsedCondition["type"].(string)
		if !ok {
			setupLog.V(1).Info("Cannot parse ConstraintPodStatus conditions type")

			return false, errors.New("Failed to parse ConstraintPodStatus conditions type")
		}

		if conditionType == "NamesAccepted" && status == "True" {
			setupLog.V(1).Info("ConstraintPodStatus CRD is ready")

			return true, nil
		}
	}

	setupLog.V(1).Info("ConstraintPodStatus CRD is not ready yet")

	return false, nil
}

// Check gatekeeper auditFromCache=Automatic
func checkCPScontrollerPrereqs(gatekeeper *operatorv1alpha1.Gatekeeper) bool {
	return gatekeeper.Spec.Audit != nil && gatekeeper.Spec.Audit.AuditFromCache != nil &&
		*gatekeeper.Spec.Audit.AuditFromCache == operatorv1alpha1.AuditFromCacheAutomatic
}

func (r *GatekeeperReconciler) StopCPSController() {
	if r.cpsCtrlCtxCancel == nil {
		return
	}

	setupLog.Info("Gatekeeper auditFromCache unset from Automatic. Stopping the ConstraintPodStatus manager.")

	r.cpsCtrlCtxCancel()

	for r.isCPSCtrlRunning {
		setupLog.Info("Waiting for the ConstraintPodStatus manager to shutdown")

		time.Sleep(1 * time.Second)
	}
}
