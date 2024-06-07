package controllers

import (
	"context"

	"github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	"github.com/open-policy-agent/gatekeeper/v3/pkg/wildcard"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	cacheRuntime "sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
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

func (r *GatekeeperReconciler) initConfig(
	ctx context.Context,
	gatekeeper *operatorv1alpha1.Gatekeeper,
) error {
	configGVR := schema.GroupVersionResource{
		Group:    "config.gatekeeper.sh",
		Version:  "v1alpha1",
		Resource: "configs",
	}

	_, err := r.DynamicClient.Resource(configGVR).Get(ctx, "config", metav1.GetOptions{})
	if err != nil {
		if apierrors.IsNotFound(err) {
			err = createDefaultConfig(ctx, r.Client, r.Namespace, gatekeeper, r.Scheme)
			if err != nil {
				return err
			}

			r.Log.Info("The Gatekeeper Config resource is created.")

			return nil
		}

		return err
	}

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

	var configCtrlCtx context.Context

	configCtrlCtx, r.configCtrlCtxCancel = context.WithCancel(ctx)

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
	r.subControllerWait.Add(1)

	// Use another go routine for the Config controller
	go func() {
		err := configMgr.Start(configCtrlCtx)
		if err != nil {
			setupLog.Error(err, "A problem running Config manager. Triggering a reconcile to restart it.")
		}

		defer r.configCtrlCtxCancel()

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

		r.subControllerWait.Done()
	}()

	return nil
}
