package controllers

import (
	"context"
	"reflect"

	"github.com/go-logr/logr"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
)

type ConfigReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	Log       logr.Logger
	Namespace string
}

// SetupWithManager sets up the controller with the Manager.
func (r *ConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		WithOptions(controller.Options{MaxConcurrentReconciles: int(1)}).
		Named(ControllerName).
		For(&v1alpha1.Config{},
			builder.WithPredicates(predicate.Funcs{
				// Reconcile only when Spec.Match is changed
				UpdateFunc: func(e event.UpdateEvent) bool {
					oldObj := e.ObjectOld.(*v1alpha1.Config)
					newObj := e.ObjectNew.(*v1alpha1.Config)

					return !reflect.DeepEqual(oldObj.Spec.Match, newObj.Spec.Match)
				},
			},
			)).
		Complete(r)
}

// Reconcile only when Spec.Match is changed
// When the user updates spec.match manually. This Reconcile updates the spec.match with
// the default exempt namespaces and the Gatekeeper CR spec.config.matches
func (r *ConfigReconciler) Reconcile(ctx context.Context,
	request reconcile.Request,
) (reconcile.Result, error) {
	log := r.Log.WithValues("Request.Namespace", request.Namespace, "Request.Name", request.Name)
	log.Info("Reconciling Config")

	gatekeeper := &operatorv1alpha1.Gatekeeper{}

	err := r.Get(ctx, types.NamespacedName{
		Name: "gatekeeper",
	}, gatekeeper)
	if err != nil {
		if apierrors.IsNotFound(err) {
			log.V(2).Info("Gatekeeper does not exist")

			return reconcile.Result{}, nil
		}

		log.Error(err, "Failed to get the Gatekeeper CR")

		return reconcile.Result{}, err
	}

	config := &v1alpha1.Config{}

	// When the user deletes the Config CR, this recreates it
	err = r.Get(ctx, types.NamespacedName{
		Namespace: r.Namespace,
		Name:      "config",
	}, config)
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = createDefaultConfig(ctx, r.Client, r.Namespace, gatekeeper, r.Scheme)
			if err != nil {
				log.Error(err, "Failed to create the Config")

				return reconcile.Result{}, err
			}

			log.Info("The Config object was not found and has been recreated.")

			return reconcile.Result{}, nil
		} else {
			log.Error(err, "Failed to get the Config")

			return reconcile.Result{}, err
		}
	}

	err = r.setExemptNamespaces(ctx, config, gatekeeper)
	if err != nil {
		log.V(1).Error(err, "Adding default exempt namespaces to the Config has failed")

		return reconcile.Result{}, err
	}

	return reconcile.Result{}, nil
}

// Reset Config.Spec.Match and append the default exempt namespaces and
// provided matches from the Gatekeeper CR
func (r *ConfigReconciler) setExemptNamespaces(
	ctx context.Context,
	existingConfig *v1alpha1.Config,
	gatekeeper *operatorv1alpha1.Gatekeeper,
) error {
	// Find OwnerReference
	ownerRefFound := false

	for _, ownerRef := range existingConfig.GetOwnerReferences() {
		if ownerRef.UID == gatekeeper.UID {
			ownerRefFound = true

			break
		}
	}

	// The ownerRefFound which is false means the Config resource was not created by gatekeeper-operator
	if !ownerRefFound && len(existingConfig.Spec.Match) != 0 {
		r.Log.V(1).Info("The gatekeeper matches already exist. Skip adding DefaultExemptNamespaces")

		return nil
	}

	// Reset Config Match
	var newMatch []v1alpha1.MatchEntry

	var configDefault *v1alpha1.Config

	// When it is DisableDefaultMatches = false or nil then append default exempt namespaces
	if gatekeeper.Spec.Config == nil || !gatekeeper.Spec.Config.DisableDefaultMatches {
		configDefault = getDefaultConfig("")
		newMatch = append(newMatch, configDefault.Spec.Match...)
	}

	// Avoid gatekeeper.Spec.Config nil error
	if gatekeeper.Spec.Config != nil {
		// Append matched from Gatekeeper CR spec.config.matches
		newMatch = append(newMatch, gatekeeper.Spec.Config.Matches...)
	}

	// When ownerRefFound is false, config will be updated for adding ownerRef
	if reflect.DeepEqual(existingConfig.Spec.Match, newMatch) && ownerRefFound {
		r.Log.V(1).Info("No need to Update")

		return nil
	}

	existingConfig.Spec.Match = newMatch

	// Set OwnerReference
	if !ownerRefFound {
		if err := controllerutil.SetOwnerReference(gatekeeper, existingConfig, r.Scheme); err != nil {
			return err
		}
	}

	err := r.Update(ctx, existingConfig, &client.UpdateOptions{})
	if err != nil {
		return err
	}

	r.Log.Info("Updated Config object with excluded namespaces")

	return nil
}
