package controllers

import (
	"context"
	"reflect"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	"github.com/go-logr/logr"
	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/pkg/wildcard"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	cacheRuntime "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
)

// Default config data
func getDefaultConfig(namespace string) *v1alpha1.Config {
	config := &v1alpha1.Config{
		ObjectMeta: metav1.ObjectMeta{
			Namespace: namespace,
			Name:      "config",
		},
		Spec: v1alpha1.ConfigSpec{
			Match: []v1alpha1.MatchEntry{
				{
					ExcludedNamespaces: []wildcard.Wildcard{
						"kube-*", "multicluster-engine",
						"hypershift", "hive", "rhacs-operator", "open-cluster-*", "openshift-*",
					},
					Processes: []string{
						"webhook", "mutation-webhook",
					},
				},
			},
		},
	}

	return config
}

func createDefaultConfig(ctx context.Context, c client.Client, namespace string,
	gatekeeper *operatorv1alpha1.Gatekeeper, scheme *runtime.Scheme,
) error {
	var config *v1alpha1.Config

	// When it is DisableDefaultMatches = true
	if gatekeeper.Spec.Config != nil && gatekeeper.Spec.Config.DisableDefaultMatches {
		config = &v1alpha1.Config{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "config",
				Namespace: namespace,
			},
		}
	} else {
		config = getDefaultConfig(namespace)
	}

	matches := []v1alpha1.MatchEntry{}
	if gatekeeper.Spec.Config != nil && gatekeeper.Spec.Config.Matches != nil {
		// Will append gatekeeper.Spec.Config.Matches
		matches = gatekeeper.Spec.Config.Matches
	}

	// Append matched from Gatekeeper CR spec.config.matches
	config.Spec.Match = append(config.Spec.Match, matches...)

	// Set OwnerReference
	if err := controllerutil.SetOwnerReference(gatekeeper, config, scheme); err != nil {
		return err
	}

	err := c.Create(ctx, config)
	if err != nil {
		return err
	}

	return nil
}

// Reset Config.Spec.Match and append the default exempt namespaces and
// provided matches from the Gatekeeper CR
func setExemptNamespaces(ctx context.Context,
	c client.Client, existingConfig *v1alpha1.Config,
	gatekeeper *operatorv1alpha1.Gatekeeper, scheme *runtime.Scheme, log logr.Logger,
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
		log.V(1).Info("The gatekeeper matches already exist. Skip adding DefaultExemptNamespaces")

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
		log.V(1).Info("No need to Update")

		return nil
	}

	existingConfig.Spec.Match = newMatch

	// Set OwnerReference
	if !ownerRefFound {
		if err := controllerutil.SetOwnerReference(gatekeeper, existingConfig, scheme); err != nil {
			return err
		}
	}

	err := c.Update(ctx, existingConfig, &client.UpdateOptions{})
	if err != nil {
		return err
	}

	log.Info("Updated Config object with excluded namespaces")

	return nil
}

func (r *GatekeeperReconciler) handleConfigController(ctx context.Context) error {
	isCRDReady, err := checkCrdAvailable(ctx, r.DynamicClient, "Config", "configs.config.gatekeeper.sh")
	if err != nil {
		return err
	}

	if !isCRDReady {
		return errCrdNotReady
	}

	if r.isConfigCtrlRunning {
		return nil
	}

	configMgr, err := ctrl.NewManager(r.KubeConfig, ctrl.Options{
		Scheme: r.Scheme,
		Metrics: server.Options{
			BindAddress: "0",
		},
		LeaderElection:   r.EnableLeaderElection,
		LeaderElectionID: "5ff985ccc.config.gatekeeper.sh",
		Cache: cacheRuntime.Options{
			ByObject: map[client.Object]cacheRuntime.ByObject{
				&v1alpha1.Config{}: {
					Namespaces: map[string]cacheRuntime.Config{
						r.Namespace: {
							FieldSelector: fields.SelectorFromSet(fields.Set{"metadata.name": "config"}),
						},
					},
				},
			},
		},
	})
	if err != nil {
		setupLog.Error(err, "Failed to setup NewManager for Config controller")

		return err
	}

	if err := (&ConfigReconciler{
		Scheme:    r.Scheme,
		Client:    configMgr.GetClient(),
		Log:       ctrl.Log.WithName("Config"),
		Namespace: r.Namespace,
	}).SetupWithManager(configMgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Config")

		return err
	}

	r.isConfigCtrlRunning = true

	// Use another go routine for the Config controller
	go func() {
		err := configMgr.Start(ctx)
		if err != nil {
			setupLog.Error(err, "A problem running Config manager. Triggering a reconcile to restart it.")
		}

		r.isConfigCtrlRunning = false

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
