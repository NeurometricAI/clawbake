package main

import (
	"flag"
	"os"
	"strconv"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	clawbakev1alpha1 "github.com/clawbake/clawbake/api/v1alpha1"
	"github.com/clawbake/clawbake/internal/operator"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(corev1.AddToScheme(scheme))
	utilruntime.Must(appsv1.AddToScheme(scheme))
	utilruntime.Must(networkingv1.AddToScheme(scheme))
	utilruntime.Must(clawbakev1alpha1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	var leaderElect bool

	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8081", "The address the metrics endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8082", "The address the probe endpoint binds to.")
	flag.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election for controller manager.")

	opts := zap.Options{Development: true}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
	logger := ctrl.Log.WithName("setup")

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "clawbake-operator-leader",
	})
	if err != nil {
		logger.Error(err, "unable to create manager")
		os.Exit(1)
	}

	serverNamespace := os.Getenv("SERVER_NAMESPACE")
	if serverNamespace == "" {
		serverNamespace = "clawbake"
	}

	ttydEnabled := os.Getenv("INSTANCE_TTYD_ENABLED") != "false"

	ttydImage := os.Getenv("INSTANCE_TTYD_IMAGE")
	if ttydImage == "" {
		ttydImage = "tsl0922/ttyd:alpine"
	}

	tuiEnabled := ttydEnabled && os.Getenv("INSTANCE_TUI_ENABLED") != "false"
	tuiPort := int32(7681)
	if p := os.Getenv("INSTANCE_TUI_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			tuiPort = int32(v)
		}
	}
	tuiCommand := os.Getenv("INSTANCE_TUI_COMMAND")
	if tuiCommand == "" {
		tuiCommand = "/ttyd-bin/ttyd -W -p 7681 node /app/openclaw.mjs tui --token $OPENCLAW_GATEWAY_TOKEN"
	}

	shellEnabled := ttydEnabled && os.Getenv("INSTANCE_SHELL_ENABLED") != "false"
	shellPort := int32(7682)
	if p := os.Getenv("INSTANCE_SHELL_PORT"); p != "" {
		if v, err := strconv.Atoi(p); err == nil {
			shellPort = int32(v)
		}
	}
	shellCommand := os.Getenv("INSTANCE_SHELL_COMMAND")
	if shellCommand == "" {
		shellCommand = "/ttyd-bin/ttyd -W -p 7682 /bin/bash"
	}

	reconciler := &operator.ClawInstanceReconciler{
		Client:               mgr.GetClient(),
		Scheme:               mgr.GetScheme(),
		Recorder:             mgr.GetEventRecorderFor("clawbake-operator"),
		ServerNamespace:      serverNamespace,
		DefaultGatewayConfig: os.Getenv("DEFAULT_GATEWAY_CONFIG"),
		TtydImage:            ttydImage,
		TUIEnabled:           tuiEnabled,
		TUIPort:              tuiPort,
		TUICommand:           tuiCommand,
		ShellEnabled:         shellEnabled,
		ShellPort:            shellPort,
		ShellCommand:         shellCommand,
	}

	if serverURL := os.Getenv("SERVER_URL"); serverURL != "" {
		reconciler.Notifier = operator.NewHTTPNotifier(serverURL)
		logger.Info("instance-ready notifications enabled", "serverURL", serverURL)
	}
	if err := reconciler.SetupWithManager(mgr); err != nil {
		logger.Error(err, "unable to create controller", "controller", "ClawInstance")
		os.Exit(1)
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Error(err, "unable to set up health check")
		os.Exit(1)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Error(err, "unable to set up ready check")
		os.Exit(1)
	}

	logger.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		logger.Error(err, "problem running manager")
		os.Exit(1)
	}
}
