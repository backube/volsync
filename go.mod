module github.com/backube/scribe

go 1.15

require (
	github.com/go-logr/logr v0.4.0
	github.com/go-logr/zapr v0.4.0 // indirect
	github.com/kubernetes-csi/external-snapshotter/v2 v2.1.1
	github.com/onsi/ginkgo v1.15.2
	github.com/onsi/gomega v1.10.1
	github.com/operator-framework/operator-lib v0.1.0
	github.com/prometheus/client_golang v1.9.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/cobra v1.1.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.7.0
	k8s.io/api v0.20.5
	k8s.io/apimachinery v0.20.5
	k8s.io/cli-runtime v0.20.5
	k8s.io/client-go v0.20.5
	k8s.io/component-base v0.20.5
	k8s.io/klog/v2 v2.4.0
	k8s.io/kubectl v0.20.5
	sigs.k8s.io/controller-runtime v0.6.2
)
