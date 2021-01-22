/*
Copyright 2020 The Scribe authors.

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
	"flag"
	"fmt"
	"os"
	"runtime"

	scribev1alpha1 "github.com/backube/scribe/api/v1alpha1"
	"github.com/backube/scribe/controllers"
	snapv1 "github.com/kubernetes-csi/external-snapshotter/v2/pkg/apis/volumesnapshot/v1beta1"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	_ "k8s.io/client-go/plugin/pkg/client/auth/gcp"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	// +kubebuilder:scaffold:imports
)

var (
	scheme        = kruntime.NewScheme()
	setupLog      = ctrl.Log.WithName("setup")
	scribeVersion = "0.0.0"
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(snapv1.AddToScheme(scheme))
	utilruntime.Must(scribev1alpha1.AddToScheme(scheme))
	// +kubebuilder:scaffold:scheme
}

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	flag.StringVar(&metricsAddr, "metrics-addr", ":8080", "The address the metric endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "enable-leader-election", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&controllers.RcloneContainerImage, "rclone-container-image",
		controllers.DefaultRcloneContainerImage, "The container image for the rclone data mover")
	flag.StringVar(&controllers.RsyncContainerImage, "rsync-container-image",
		controllers.DefaultRsyncContainerImage, "The container image for the rsync data mover")
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	setupLog.Info(fmt.Sprintf("Go Version: %s", runtime.Version()))
	setupLog.Info(fmt.Sprintf("Go OS/Arch: %s/%s", runtime.GOOS, runtime.GOARCH))
	setupLog.Info(fmt.Sprintf("Operator Version: %s", scribeVersion))
	setupLog.Info(fmt.Sprintf("Rclone container: %s", controllers.RcloneContainerImage))
	setupLog.Info(fmt.Sprintf("Rsync container: %s", controllers.RsyncContainerImage))

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:             scheme,
		MetricsBindAddress: metricsAddr,
		Port:               9443,
		LeaderElection:     enableLeaderElection,
		LeaderElectionID:   "b95b3104.backube",
	})
	if err != nil {
		setupLog.Error(err, "unable to start manager")
		os.Exit(1)
	}

	if err = (&controllers.ReplicationSourceReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Source"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Source")
		os.Exit(1)
	}
	if err = (&controllers.ReplicationDestinationReconciler{
		Client: mgr.GetClient(),
		Log:    ctrl.Log.WithName("controllers").WithName("Destination"),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "Destination")
		os.Exit(1)
	}
	// +kubebuilder:scaffold:builder

	setupLog.Info("starting manager")
	if err := mgr.Start(ctrl.SetupSignalHandler()); err != nil {
		setupLog.Error(err, "problem running manager")
		os.Exit(1)
	}
}
