/*
Copyright 2021 The VolSync authors.

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as published
by the Free Software Foundation, either version 3 of the License, or
(at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program.  If not, see <https://www.gnu.org/licenses/>.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"runtime"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	_ "embed"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v6/apis/volumesnapshot/v1"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/controllers"
	"github.com/backube/volsync/controllers/mover"
	"github.com/backube/volsync/controllers/mover/rclone"
	"github.com/backube/volsync/controllers/mover/restic"
	"github.com/backube/volsync/controllers/mover/rsync"
	"github.com/backube/volsync/controllers/mover/rsynctls"
	"github.com/backube/volsync/controllers/mover/syncthing"
	"github.com/backube/volsync/controllers/platform"
	"github.com/backube/volsync/controllers/utils"
	//+kubebuilder:scaffold:imports
)

var (
	scheme         = kruntime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	volsyncVersion = "0.0.0"

	//go:embed config/openshift/mover_scc.yaml
	volsyncMoverSCCYamlRaw []byte
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(snapv1.AddToScheme(scheme))
	utilruntime.Must(volsyncv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ocpsecurityv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// registerMovers Registers the data movers to be used by the VolSync operator.
func registerMovers() error {
	if err := rsync.Register(); err != nil {
		return fmt.Errorf("error registering rsync data mover: %w", err)
	}
	if err := rclone.Register(); err != nil {
		return fmt.Errorf("error registering rclone data mover: %w", err)
	}
	if err := restic.Register(); err != nil {
		return fmt.Errorf("error registering restic data mover: %w", err)
	}
	if err := rsynctls.Register(); err != nil {
		return fmt.Errorf("error registering rsync-tls data mover: %w", err)
	}
	if err := syncthing.Register(); err != nil {
		return fmt.Errorf("error registering syncthing data mover: %w", err)
	}
	return nil
}

// printInfo Prints version information about the operating system and movers to STDOUT.
func printInfo() {
	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	setupLog.Info(fmt.Sprintf("Operator Version: %s", volsyncVersion))
	for _, b := range mover.Catalog {
		setupLog.Info(b.VersionInfo())
	}
}

// configureChecks Configures the manager with a healthz and readyz check.
func configureChecks(mgr manager.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to setup health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("unable to setup ready check: %w", err)
	}
	return nil
}

// addCommandFlags Configures flags to be bound to the VolSync command.
func addCommandFlags(probeAddr *string, metricsAddr *string, enableLeaderElection *bool) {
	flag.StringVar(metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&utils.SCCName, "scc-name",
		utils.DefaultSCCName, "The name of the volsync security context constraint")
	opts := zap.Options{
		Development: true,
		TimeEncoder: zapcore.ISO8601TimeEncoder,
	}
	opts.BindFlags(flag.CommandLine)

	// Import flags into pflag so they can be bound by viper
	pflag.CommandLine.AddGoFlagSet(flag.CommandLine)
	pflag.Parse()
	if err := viper.BindPFlags(pflag.CommandLine); err != nil {
		setupLog.Error(err, "Unable to bind command line flags")
		os.Exit(1)
	}

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))
}

func ensurePrivilegedMoverScc(cfg *rest.Config) {
	setupLog.Info("Privileged Mover SCC", "scc-name", utils.SCCName)
	setupClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "error creating client")
		os.Exit(1)
	}

	err = platform.EnsureVolSyncMoverSCCIfOpenShift(context.Background(), setupClient, setupLog,
		utils.SCCName, volsyncMoverSCCYamlRaw)
	if err != nil {
		setupLog.Error(err, "unable to reconcile volsync mover scc", "scc-name", utils.SCCName)
		os.Exit(1)
	}
}

func initPodLogsClient(cfg *rest.Config) {
	_, err := utils.InitPodLogsClient(cfg)
	if err != nil {
		setupLog.Error(err, "unable to create client-go clientset for pod logs")
		os.Exit(1)
	}
	setupLog.Info("Mover Status Log", "log max bytes", utils.GetMoverLogMaxBytes(),
		"tail lines", utils.GetMoverLogTailLines(), "debug", utils.IsMoverLogDebug())
}

// nolint: funlen
func main() {
	err := registerMovers()
	if err != nil {
		setupLog.Error(err, "error registering data movers")
		os.Exit(1)
	}
	var probeAddr, metricsAddr string
	var enableLeaderElection bool
	addCommandFlags(&probeAddr, &metricsAddr, &enableLeaderElection)
	printInfo()

	leaseDuration := 137 * time.Second
	renewDeadline := 107 * time.Second
	retryPeriod := 26 * time.Second

	cfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		MetricsBindAddress:     metricsAddr,
		Port:                   9443,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         enableLeaderElection,
		LeaderElectionID:       "b95b3104.backube",
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		LeaderElectionReleaseOnCancel: true,
		LeaseDuration:                 &leaseDuration,
		RenewDeadline:                 &renewDeadline,
		RetryPeriod:                   &retryPeriod,
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	// Before starting controllers - create or patch volsync mover SCC if necessary
	ensurePrivilegedMoverScc(cfg)

	initPodLogsClient(cfg)

	if err = (&controllers.ReplicationSourceReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("ReplicationSource"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("volsync-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ReplicationSource")
		os.Exit(1)
	}
	if err = (&controllers.ReplicationDestinationReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("ReplicationDestination"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("volsync-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ReplicationDestination")
		os.Exit(1)
	}
	if err = (&controllers.VolumePopulatorReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controllers").WithName("VolumePopulator"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("volsync-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VolumePopulator")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder
	if err := configureChecks(mgr); err != nil {
		setupLog.Error(err, "unable to setup checks")
		os.Exit(1)
	}
	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
