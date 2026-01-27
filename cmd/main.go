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
	"bufio"
	"context"
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"time"

	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
	"k8s.io/client-go/rest"

	_ "embed"

	snapv1 "github.com/kubernetes-csi/external-snapshotter/client/v8/apis/volumesnapshot/v1"
	volumepopulatorv1beta1 "github.com/kubernetes-csi/volume-data-source-validator/client/apis/volumepopulator/v1beta1"
	ocpsecurityv1 "github.com/openshift/api/security/v1"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
	"go.uber.org/zap/zapcore"
	kruntime "k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/certwatcher"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/metrics/filters"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	volsyncv1alpha1 "github.com/backube/volsync/api/v1alpha1"
	"github.com/backube/volsync/internal/controller"
	"github.com/backube/volsync/internal/controller/mover"
	"github.com/backube/volsync/internal/controller/platform"
	"github.com/backube/volsync/internal/controller/utils"
	//+kubebuilder:scaffold:imports
)

var (
	scheme         = kruntime.NewScheme()
	setupLog       = ctrl.Log.WithName("setup")
	volsyncVersion = "0.0.0"

	//go:embed openshift/mover_scc.yaml
	volsyncMoverSCCYamlRaw []byte

	// See each mover_<movertype>_register.go where they add themselves to
	// enabledMovers
	enabledMovers = []func() error{}
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(snapv1.AddToScheme(scheme))
	utilruntime.Must(volsyncv1alpha1.AddToScheme(scheme))
	utilruntime.Must(ocpsecurityv1.AddToScheme(scheme))
	utilruntime.Must(volumepopulatorv1beta1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

// registerMovers Registers the data movers to be used by the VolSync operator.
func registerMovers() error {
	// logger isn't initialized yet, write to stdout
	bufout := bufio.NewWriter(os.Stdout)
	defer bufout.Flush()

	for _, moverRegisterFunc := range enabledMovers {
		if err := moverRegisterFunc(); err != nil {
			return err
		}
	}
	fmt.Fprintf(bufout, "Registered Movers: %v\n", mover.GetEnabledMoverList())
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
func addCommandFlags(probeAddr *string, metricsAddr *string, enableLeaderElection *bool, secureMetrics *bool,
	metricsRequireRBAC *bool, metricsCertPath *string, metricsCertName *string, metricsCertKey *string,
	enableHTTP2 *bool) {
	flag.StringVar(metricsAddr, "metrics-bind-address", ":0", "The address the metric endpoint binds to."+
		"Use :8443 for HTTPS or :8080 for HTTP, or leave as 0 to disable the metrics service.")
	flag.BoolVar(secureMetrics, "metrics-secure", true,
		"If set, the metrics endpoint is served securely via HTTPS. Use --metrics-secure=false to use HTTP instead.")
	flag.BoolVar(metricsRequireRBAC, "metrics-require-rbac", true,
		"enables protection of the metrics endpoint with RBAC-based authn/authz. see "+
			"https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.18.4/pkg/metrics/filters#WithAuthenticationAndAuthorization "+
			" for more info.")
	flag.StringVar(probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	flag.StringVar(&utils.SCCName, "scc-name", utils.DefaultSCCName,
		"The name of the volsync security context constraint")
	flag.StringVar(&utils.MoverImagePullSecrets, "mover-image-pull-secrets", "",
		"comma-separated list of pull secrets volsync should copy from its namespace and use for mover jobs")
	flag.BoolVar(enableHTTP2, "enable-http2", false, "If set, HTTP/2 will be enabled for the metrics and webhook servers")
	//flag.StringVar(&webhookCertPath, "webhook-cert-path", "", "The directory that contains the webhook certificate.")
	//flag.StringVar(&webhookCertName, "webhook-cert-name", "tls.crt", "The name of the webhook certificate file.")
	//flag.StringVar(&webhookCertKey, "webhook-cert-key", "tls.key", "The name of the webhook key file.")
	flag.StringVar(metricsCertPath, "metrics-cert-path", "", "The directory that contains the metrics server certificate.")
	flag.StringVar(metricsCertName, "metrics-cert-name", "tls.crt", "The name of the metrics server certificate file.")
	flag.StringVar(metricsCertKey, "metrics-cert-key", "tls.key", "The name of the metrics server key file.")

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

// Prereq CRs we want to always be present in certain environments but do not want to reconcile often (just at startup)
func ensureCRs(cfg *rest.Config) {
	setupClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		setupLog.Error(err, "error creating client")
		os.Exit(1)
	}

	// Privileged mover SCC required in OpenShift envs
	setupLog.Info("Privileged Mover SCC", "scc-name", utils.SCCName)
	err = platform.EnsureVolSyncMoverSCCIfOpenShift(context.Background(), setupClient, setupLog,
		utils.SCCName, volsyncMoverSCCYamlRaw)
	if err != nil {
		setupLog.Error(err, "unable to reconcile volsync mover scc", "scc-name", utils.SCCName)
		os.Exit(1)
	}

	// VolumePopulator CR should be registered if the VolumePopulator CRD is present
	err = controller.EnsureVolSyncVolumePopulatorCRIfCRDPresent(context.Background(), setupClient, setupLog)
	if err != nil {
		setupLog.Error(err, "unable to reconcile VolumePopulator CR")
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
		fmt.Printf("error registering data movers: %+v", err)
		os.Exit(1)
	}
	var probeAddr, metricsAddr string
	var secureMetrics bool
	var metricsRequireRBAC bool
	var enableLeaderElection bool
	var enableHTTP2 bool
	var metricsCertPath, metricsCertName, metricsCertKey string
	//var webhookCertPath, webhookCertName, webhookCertKey string
	var tlsOpts []func(*tls.Config)

	addCommandFlags(&probeAddr, &metricsAddr, &enableLeaderElection, &secureMetrics,
		&metricsRequireRBAC, &metricsCertPath, &metricsCertName, &metricsCertKey, &enableHTTP2)
	printInfo()

	leaseDuration := 137 * time.Second
	renewDeadline := 107 * time.Second
	retryPeriod := 26 * time.Second

	// if the enable-http2 flag is false (the default), http/2 should be disabled
	// due to its vulnerabilities. More specifically, disabling http/2 will
	// prevent from being vulnerable to the HTTP/2 Stream Cancellation and
	// Rapid Reset CVEs. For more information see:
	// - https://github.com/advisories/GHSA-qppj-fm5r-hxr3
	// - https://github.com/advisories/GHSA-4374-p667-p6c8
	disableHTTP2 := func(c *tls.Config) {
		setupLog.Info("disabling http/2")
		c.NextProtos = []string{"http/1.1"}
	}

	if !enableHTTP2 {
		tlsOpts = append(tlsOpts, disableHTTP2)
	}

	// Create watchers for metrics and webhooks certificates
	var metricsCertWatcher *certwatcher.CertWatcher
	//var webhookCertWatcher *certwatcher.CertWatcher

	// // Initial webhook TLS options
	// webhookTLSOpts := tlsOpts

	// if len(webhookCertPath) > 0 {
	// 		setupLog.Info("Initializing webhook certificate watcher using provided certificates",
	// 			"webhook-cert-path", webhookCertPath, "webhook-cert-name", webhookCertName, "webhook-cert-key", webhookCertKey)

	// 		var err error
	// 		webhookCertWatcher, err = certwatcher.New(
	// 			filepath.Join(webhookCertPath, webhookCertName),
	// 			filepath.Join(webhookCertPath, webhookCertKey),
	// 		)
	// 		if err != nil {
	// 			setupLog.Error(err, "Failed to initialize webhook certificate watcher")
	// 			os.Exit(1)
	// 		}

	// 		webhookTLSOpts = append(webhookTLSOpts, func(config *tls.Config) {
	// 			config.GetCertificate = webhookCertWatcher.GetCertificate
	// 		})
	// 	}
	// }

	// webhookServer := webhook.NewServer(webhook.Options{
	// 	TLSOpts: webhookTLSOpts,
	// })

	// Metrics endpoint is enabled in 'config/default/kustomization.yaml'. The Metrics options configure the server.
	// More info:
	// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/server
	// - https://book.kubebuilder.io/reference/metrics.html
	metricsServerOptions := metricsserver.Options{
		BindAddress:   metricsAddr,
		SecureServing: secureMetrics,
		// TODO(user): TLSOpts is used to allow configuring the TLS config used for the server. If certificates are
		// not provided, self-signed certificates will be generated by default. This option is not recommended for
		// production environments as self-signed certificates do not offer the same level of trust and security
		// as certificates issued by a trusted Certificate Authority (CA). The primary risk is potentially allowing
		// unauthorized access to sensitive metrics data. Consider replacing with CertDir, CertName, and KeyName
		// to provide certificates, ensuring the server communicates using trusted and secure certificates.
		TLSOpts: tlsOpts,
	}

	if secureMetrics && metricsRequireRBAC {
		// FilterProvider is used to protect the metrics endpoint with authn/authz.
		// These configurations ensure that only authorized users and service accounts
		// can access the metrics endpoint. The RBAC are configured in 'config/rbac/kustomization.yaml'. More info:
		// https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.19.0/pkg/metrics/filters#WithAuthenticationAndAuthorization
		metricsServerOptions.FilterProvider = filters.WithAuthenticationAndAuthorization
	}

	// If the certificate is not specified, controller-runtime will automatically
	// generate self-signed certificates for the metrics server. While convenient for development and testing,
	// this setup is not recommended for production.
	//
	// TODO(user): If you enable certManager, uncomment the following lines:
	// - [METRICS-WITH-CERTS] at config/default/kustomization.yaml to generate and use certificates
	// managed by cert-manager for the metrics server.
	// - [PROMETHEUS-WITH-CERTS] at config/prometheus/kustomization.yaml for TLS certification.
	if len(metricsCertPath) > 0 {
		setupLog.Info("Initializing metrics certificate watcher using provided certificates",
			"metrics-cert-path", metricsCertPath, "metrics-cert-name", metricsCertName, "metrics-cert-key", metricsCertKey)

		var err error
		metricsCertWatcher, err = certwatcher.New(
			filepath.Join(metricsCertPath, metricsCertName),
			filepath.Join(metricsCertPath, metricsCertKey),
		)
		if err != nil {
			setupLog.Error(err, "to initialize metrics certificate watcher", "error", err)
			os.Exit(1)
		}

		metricsServerOptions.TLSOpts = append(metricsServerOptions.TLSOpts, func(config *tls.Config) {
			config.GetCertificate = metricsCertWatcher.GetCertificate
		})
	}

	cfg := ctrl.GetConfigOrDie()
	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsServerOptions,
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

	// Before starting controllers - create or patch volsync mover SCC and VolumePopulator CR if necessary
	ensureCRs(cfg)

	initPodLogsClient(cfg)

	// Index fields that are required for the ReplicationSource controller
	if err := controller.IndexFieldsForReplicationSource(context.Background(), mgr.GetFieldIndexer()); err != nil {
		setupLog.Error(err, "unable to index fields for controller", "controller", "ReplicationSource")
		os.Exit(1)
	}
	if err = (&controller.ReplicationSourceReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controller").WithName("ReplicationSource"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("volsync-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ReplicationSource")
		os.Exit(1)
	}
	if err = (&controller.ReplicationDestinationReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controller").WithName("ReplicationDestination"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("volsync-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "ReplicationDestination")
		os.Exit(1)
	}

	// Index fields that are required for the VolumePopulator controller
	if err := controller.IndexFieldsForVolumePopulator(context.Background(), mgr.GetFieldIndexer()); err != nil {
		setupLog.Error(err, "unable to index fields for controller", "controller", "VolumePopulator")
		os.Exit(1)
	}
	if err = (&controller.VolumePopulatorReconciler{
		Client:        mgr.GetClient(),
		Log:           ctrl.Log.WithName("controller").WithName("VolumePopulator"),
		Scheme:        mgr.GetScheme(),
		EventRecorder: mgr.GetEventRecorderFor("volsync-controller"),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create controller", "controller", "VolumePopulator")
		os.Exit(1)
	}
	//+kubebuilder:scaffold:builder

	if metricsCertWatcher != nil {
		setupLog.Info("Adding metrics certificate watcher to manager")
		if err := mgr.Add(metricsCertWatcher); err != nil {
			setupLog.Error(err, "unable to add metrics certificate watcher to manager")
			os.Exit(1)
		}
	}

	// if webhookCertWatcher != nil {
	// 	setupLog.Info("Adding webhook certificate watcher to manager")
	// 	if err := mgr.Add(webhookCertWatcher); err != nil {
	// 		setupLog.Error(err, "unable to add webhook certificate watcher to manager")
	// 		os.Exit(1)
	// 	}
	// }

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
