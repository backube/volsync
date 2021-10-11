module github.com/backube/volsync

go 1.15

require (
	github.com/go-logr/logr v0.4.0
	github.com/google/uuid v1.1.2
	github.com/kubernetes-csi/external-snapshotter/client/v4 v4.1.0
	github.com/onsi/ginkgo v1.16.4
	github.com/onsi/gomega v1.13.0
	github.com/openshift/api v0.0.0-20210625082935-ad54d363d274
	github.com/prometheus/client_golang v1.11.0
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/cobra v1.2.1
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.8.1
	k8s.io/api v0.22.2
	k8s.io/apimachinery v0.22.2
	k8s.io/cli-runtime v0.22.2
	k8s.io/client-go v0.22.2
	k8s.io/component-base v0.22.2
	k8s.io/klog/v2 v2.9.0
	k8s.io/kubectl v0.22.2
	k8s.io/utils v0.0.0-20210930125809-cb0fa318a74b // indirect
	sigs.k8s.io/controller-runtime v0.9.2
)
