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

package main

import (
	"flag"
	"fmt"
	"os"

	constraintV1 "github.com/open-policy-agent/frameworks/constraint/pkg/apis/templates/v1beta1"
	gkv1alpha1 "github.com/open-policy-agent/gatekeeper/v3/apis/config/v1alpha1"
	gkv1beta1 "github.com/open-policy-agent/gatekeeper/v3/apis/status/v1beta1"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/source"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	operatorv1alpha1 "github.com/gatekeeper/gatekeeper-operator/api/v1alpha1"
	"github.com/gatekeeper/gatekeeper-operator/controllers"
	"github.com/gatekeeper/gatekeeper-operator/pkg/platform"
	"github.com/gatekeeper/gatekeeper-operator/pkg/util"
	"github.com/gatekeeper/gatekeeper-operator/pkg/version"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(operatorv1alpha1.AddToScheme(scheme))
	utilruntime.Must(gkv1beta1.AddToScheme(scheme))
	utilruntime.Must(gkv1alpha1.AddToScheme(scheme))
	utilruntime.Must(constraintV1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")

	opts := zap.Options{
		Development: true,
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	ctrl.Log.WithName("Gatekeeper Operator version").Info(fmt.Sprintf("%#v", version.Get()))
	ctx := ctrl.SetupSignalHandler()

	metricsOptions := server.Options{
		BindAddress: metricsAddr,
	}

	webhookOptions := webhook.NewServer(
		webhook.Options{
			Port: 9443,
		},
	)

	cfg := ctrl.GetConfigOrDie()

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsOptions,
		WebhookServer:          webhookOptions,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "5ff985cc.gatekeeper.sh",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	platformInfo, err := platform.GetPlatformInfo(cfg)
	if err != nil {
		setupLog.Error(err, "unable to get platform name")
		os.Exit(1)
	}

	namespace, err := gatekeeperNamespace(platformInfo)
	if err != nil {
		setupLog.Error(err, "unable to get Gatekeeper namespace")
		os.Exit(1)
	}

	dynamicClient := dynamic.NewForConfigOrDie(mgr.GetConfig())
	manualReconcileTrigger := make(chan event.GenericEvent, 1024)
	fromCPSMgrSource := &source.Channel{Source: manualReconcileTrigger, DestBufferSize: 1024}

	if err = (&controllers.GatekeeperReconciler{
		Client:                 mgr.GetClient(),
		Log:                    ctrl.Log.WithName("controllers").WithName("Gatekeeper"),
		Scheme:                 mgr.GetScheme(),
		Namespace:              namespace,
		PlatformInfo:           platformInfo,
		DynamicClient:          dynamicClient,
		KubeConfig:             cfg,
		EnableLeaderElection:   enableLeaderElection,
		ManualReconcileTrigger: manualReconcileTrigger,
		DiscoveryStorage: &controllers.DiscoveryStorage{
			Log:       ctrl.Log.WithName("discovery_storage"),
			ClientSet: kubernetes.NewForConfigOrDie(mgr.GetConfig()),
		},
	}).SetupWithManager(mgr, fromCPSMgrSource); err != nil {
		setupLog.Error(err, "Unable to create controller", "controller", "Gatekeeper")
		os.Exit(1)
	}

	// +kubebuilder:scaffold:builder

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up health check")
		os.Exit(1)
	}

	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "Unable to set up ready check")
		os.Exit(1)
	}

	setupLog.Info("Starting manager")

	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "Problem running manager")

		os.Exit(1)
	}
}

func gatekeeperNamespace(platformInfo platform.PlatformInfo) (string, error) {
	if ns := os.Getenv("GATEKEEPER_TARGET_NAMESPACE"); ns != "" {
		return ns, nil
	}

	ns, err := util.GetOperatorNamespace()
	if err != nil {
		return "", errors.Wrapf(err, "Failed to get operator namespace")
	}

	if platformInfo.IsOpenShift() {
		return util.GetPlatformNamespace(platformInfo), nil
	}

	return ns, nil
}
