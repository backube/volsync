module github.com/datacosmos-br/volsync

go 1.22.0

toolchain go1.22.4

require (
	github.com/dop251/diskrsync v1.3.0
	github.com/dop251/spgz v1.2.1
	github.com/go-logr/logr v1.4.2
	github.com/google/uuid v1.6.0
	github.com/kubernetes-csi/external-snapshotter/client/v6 v6.3.0
	github.com/kubernetes-csi/volume-data-source-validator/client v0.0.0-20230911161012-c2e130d28434
	github.com/onsi/ginkgo/v2 v2.19.0
	github.com/onsi/gomega v1.33.1
	github.com/prometheus/client_golang v1.19.1
	github.com/robfig/cron/v3 v3.0.1
	github.com/spf13/cobra v1.8.0
	github.com/spf13/pflag v1.0.5
	github.com/spf13/viper v1.19.0
	github.com/syncthing/syncthing v1.27.8
	go.uber.org/zap v1.27.0
	gopkg.in/yaml.v3 v3.0.1
	k8s.io/api v0.28.2
	k8s.io/apiextensions-apiserver v0.28.2
	k8s.io/apimachinery v0.30.1
	k8s.io/client-go v0.28.2
	k8s.io/component-base v0.28.2
	k8s.io/component-helpers v0.28.2
	k8s.io/klog/v2 v2.130.0
	k8s.io/kubectl v0.28.2
	k8s.io/utils v0.0.0-20240502163921-fe8a2dddb1d0
	sigs.k8s.io/controller-runtime v0.16.2
)


replace github.com/dop251/diskrsync => github.com/datacosmos-br/diskrsync v1.3.3

