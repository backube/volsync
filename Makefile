# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
#VERSION ?= $(shell git describe --tags --dirty --match 'v*' 2> /dev/null || git describe --always --dirty)

# Include common version information from the version.mk file
include ./version.mk

BUILDDATE := $(shell date -u '+%Y-%m-%dT%H:%M:%S.%NZ')

# Helper software versions
GOLANGCI_VERSION := v1.43.0
HELM_VERSION := v3.7.1
OPERATOR_SDK_VERSION := v1.15.0
KUTTL_VERSION := 0.11.1

# We don't vendor modules. Enforce that behavior
export GOFLAGS := -mod=readonly

# CHANNELS define the bundle channels used in the bundle.
# Add a new line here if you would like to change its default config. (E.g CHANNELS = "candidate,fast,stable")
# To re-generate a bundle for other specific channels without changing the standard setup, you can:
# - use the CHANNELS as arg of the bundle target (e.g make bundle CHANNELS=candidate,fast,stable)
# - use environment variables to overwrite this value (e.g export CHANNELS="candidate,fast,stable")
ifneq ($(origin CHANNELS), undefined)
BUNDLE_CHANNELS := --channels=$(CHANNELS)
endif

# DEFAULT_CHANNEL defines the default channel used in the bundle.
# Add a new line here if you would like to change its default config. (E.g DEFAULT_CHANNEL = "stable")
# To re-generate a bundle for any other default channel without changing the default setup, you can:
# - use the DEFAULT_CHANNEL as arg of the bundle target (e.g make bundle DEFAULT_CHANNEL=stable)
# - use environment variables to overwrite this value (e.g export DEFAULT_CHANNEL="stable")
ifneq ($(origin DEFAULT_CHANNEL), undefined)
BUNDLE_DEFAULT_CHANNEL := --default-channel=$(DEFAULT_CHANNEL)
endif
BUNDLE_METADATA_OPTS ?= $(BUNDLE_CHANNELS) $(BUNDLE_DEFAULT_CHANNEL)

# IMAGE_TAG_BASE defines the docker.io namespace and part of the image name for remote images.
# This variable is used to construct full image tags for bundle and catalog images.
#
# For example, running 'make bundle-build bundle-push catalog-build catalog-push' will build and push both
# backube/volsync-bundle:$VERSION and backube/volsync-catalog:$VERSION.
IMAGE_TAG_BASE ?= quay.io/backube/volsync

# BUNDLE_IMG defines the image:tag used for the bundle.
# You can use it as an arg. (E.g make bundle-build BUNDLE_IMG=<some-registry>/<project-name-bundle>:<tag>)
BUNDLE_IMG ?= $(IMAGE_TAG_BASE)-bundle:v$(VERSION)

# Image URL to use all building/pushing image targets
IMG ?= quay.io/backube/volsync:latest
# ENVTEST_K8S_VERSION refers to the version of kubebuilder assets to be downloaded by envtest binary.
ENVTEST_K8S_VERSION = 1.22

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# This is a requirement for 'setup-envtest.sh' in the test target.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

all: build

##@ General

# The help target prints out all targets with their descriptions organized
# beneath their categories. The categories are represented by '##@' and the
# target descriptions by '##'. The awk commands is responsible for reading the
# entire set of makefiles included in this invocation, looking for lines of the
# file as xyz: ## something, and then pretty-format the target and help. Then,
# if there's a line with ##@ something, that gets pretty-printed as a category.
# More info on the usage of ANSI control characters for terminal formatting:
# https://en.wikipedia.org/wiki/ANSI_escape_code#SGR_parameters
# More info on the awk command:
# http://linuxcommand.org/lc3_adv_awk.php

.PHONY: help
help: ## Display this help.
	@awk 'BEGIN {FS = ":.*##"; printf "\nUsage:\n  make \033[36m<target>\033[0m\n"} /^[a-zA-Z_0-9-]+:.*?##/ { printf "  \033[36m%-15s\033[0m %s\n", $$1, $$2 } /^##@/ { printf "\n\033[1m%s\033[0m\n", substr($$0, 5) } ' $(MAKEFILE_LIST)

##@ Development

.PHONY: manifests
manifests: controller-gen ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	cp config/crd/bases/* helm/volsync/crds

.PHONY: generate
generate: controller-gen ## Generate code containing DeepCopy, DeepCopyInto, and DeepCopyObject method implementations.
	$(CONTROLLER_GEN) object:headerFile="hack/boilerplate.go.txt" paths="./..."

.PHONY: lint
lint: golangci-lint ## Lint source code
	$(GOLANGCILINT) run ./...

.PHONY: helm-lint
helm-lint: helm ## Lint Helm chart
	cd helm && $(HELM) lint volsync

.PHONY: test
TEST_ARGS ?= -progress -randomizeAllSpecs -randomizeSuites -slowSpecThreshold 30 -p -cover -coverprofile cover.out -outputdir .
TEST_PACKAGES ?= ./...
test: manifests generate lint envtest helm-lint ginkgo ## Run tests.
	-rm -f cover.out
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) -p path)" $(GINKGO) $(TEST_ARGS) $(TEST_PACKAGES)

.PHONY: test-e2e
test-e2e: kuttl ## Run e2e tests. Requires cluster w/ VolSync already installed
	cd test-kuttl && $(KUTTL) test
	rm -f test-kuttl/kubeconfig

.PHONY: test-krew
test-krew: krew-plugin-manifest
	kubectl krew install -v=4 --manifest=kubectl-volsync/volsync.yaml --archive=bin/kubectl-volsync.tar.gz
	kubectl volsync --version
	kubectl krew uninstall volsync

##@ Build

.PHONY: build
build: generate lint ## Build manager binary.
	go build -o bin/manager -ldflags -X=main.volsyncVersion=$(BUILD_VERSION) main.go

.PHONY: cli
cli: bin/kubectl-volsync ## Build VolSync kubectl plugin

bin/kubectl-volsync: lint
	go build -o $@ -ldflags -X=github.com/backube/volsync/kubectl-volsync/cmd.volsyncVersion=$(BUILD_VERSION) ./kubectl-volsync/main.go

.PHONY: run
run: manifests generate lint  ## Run a controller from your host.
	go run -ldflags -X=main.volsyncVersion=$(BUILD_VERSION) ./main.go

.PHONY: docker-build
docker-build: test ## Build docker image with the manager.
	docker build --build-arg "version_arg=$(BUILD_VERSION)" -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

.PHONY: krew-plugin-manifest
krew-plugin-manifest: yq bin/kubectl-volsync ## Build & package the kubectl plugin & update the krew manifest
	rm -f bin/kubectl-volsync.tar.gz
	tar czf bin/kubectl-volsync.tar.gz LICENSE -C bin kubectl-volsync
	HASH="`sha256sum bin/kubectl-volsync.tar.gz | cut -f 1 -d ' '`" \
	BUILD_VERSION="$(BUILD_VERSION)" \
	$(YQ) --inplace '.spec.version=strenv(BUILD_VERSION) | with(.spec.platforms[]; .sha256=strenv(HASH) | .uri|=sub("v[[:digit:]]+\.[^/]+", strenv(BUILD_VERSION)))' ./kubectl-volsync/volsync.yaml

##@ Deployment

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl delete -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: deploy-openshift
deploy-openshift: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/openshift | kubectl apply -f -

.PHONY: undeploy
undeploy: ## Undeploy controller from the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/default | kubectl delete -f -

.PHONY: undeploy-openshift
undeploy-openshift: manifests kustomize ## Undeploy controller to the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/openshift | kubectl delete -f -


.PHONY: controller-gen
CONTROLLER_GEN = $(shell pwd)/bin/controller-gen
controller-gen: ## Download controller-gen locally if necessary.
	$(call go-get-tool,$(CONTROLLER_GEN),sigs.k8s.io/controller-tools/cmd/controller-gen@v0.7.0)

.PHONY: kustomize
KUSTOMIZE = $(shell pwd)/bin/kustomize
kustomize: ## Download kustomize locally if necessary.
	$(call go-get-tool,$(KUSTOMIZE),sigs.k8s.io/kustomize/kustomize/v3@v3.8.7)

.PHONY: envtest
ENVTEST = $(shell pwd)/bin/setup-envtest
envtest: ## Download envtest-setup locally if necessary.
	$(call go-get-tool,$(ENVTEST),sigs.k8s.io/controller-runtime/tools/setup-envtest@latest)

.PHONY: yq
YQ = $(shell pwd)/bin/yq
yq: ## Download yq locally if necessary.
	$(call go-get-tool,$(YQ),github.com/mikefarah/yq/v4@latest)


# go-get-tool will 'go get' any package $2 and install it to $1.
PROJECT_DIR := $(shell dirname $(abspath $(lastword $(MAKEFILE_LIST))))
define go-get-tool
@[ -f $(1) ] || { \
set -e ;\
TMP_DIR=$$(mktemp -d) ;\
cd $$TMP_DIR ;\
go mod init tmp ;\
echo "Downloading $(2)" ;\
GOBIN=$(PROJECT_DIR)/bin go get $(2) ;\
rm -rf $$TMP_DIR ;\
}
endef

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | $(OPERATOR_SDK) generate bundle -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)
	$(OPERATOR_SDK) bundle validate ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM = ./bin/opm
opm: ## Download opm locally if necessary.
ifeq (,$(wildcard $(OPM)))
ifeq (,$(shell which opm 2>/dev/null))
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.15.3/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}
else
OPM = $(shell which opm)
endif
endif

# A comma-separated list of bundle images (e.g. make catalog-build BUNDLE_IMGS=example.com/operator-bundle:v0.1.0,example.com/operator-bundle:v0.2.0).
# These images MUST exist in a registry and be pull-able.
BUNDLE_IMGS ?= $(BUNDLE_IMG)

# The image tag given to the resulting catalog image (e.g. make catalog-build CATALOG_IMG=example.com/operator-catalog:v0.2.0).
CATALOG_IMG ?= $(IMAGE_TAG_BASE)-catalog:v$(VERSION)

# Set CATALOG_BASE_IMG to an existing catalog image tag to add $BUNDLE_IMGS to that image.
ifneq ($(origin CATALOG_BASE_IMG), undefined)
FROM_INDEX_OPT := --from-index $(CATALOG_BASE_IMG)
endif

# Build a catalog image by adding bundle images to an empty catalog using the operator package manager tool, 'opm'.
# This recipe invokes 'opm' in 'semver' bundle add mode. For more information on add modes, see:
# https://github.com/operator-framework/community-operators/blob/7f1438c/docs/packaging-operator.md#updating-your-existing-operator
.PHONY: catalog-build
catalog-build: opm ## Build a catalog image.
	$(OPM) index add --container-tool docker --mode semver --tag $(CATALOG_IMG) --bundles $(BUNDLE_IMGS) $(FROM_INDEX_OPT)

# Push the catalog image.
.PHONY: catalog-push
catalog-push: ## Push a catalog image.
	$(MAKE) docker-push IMG=$(CATALOG_IMG)


##@ Download utilities

OS := $(shell go env GOOS)
ARCH := $(shell go env GOARCH)

# download-tool will curl any file $2 and install it to $1.
define download-tool
@[ -f $(1) ] || { \
set -e ;\
echo "Downloading $(2)" ;\
curl -sSLo "$(1)" "$(2)" ;\
chmod a+x "$(1)" ;\
}
endef

.PHONY: ginkgo
GINKGO := $(PROJECT_DIR)/bin/ginkgo
ginkgo: ## Download ginkgo
	$(call go-get-tool,$(GINKGO),github.com/onsi/ginkgo/ginkgo)

.PHONY: golangci-lint
GOLANGCILINT := $(PROJECT_DIR)/bin/golangci-lint
GOLANGCI_URL := https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh
golangci-lint: ## Download golangci-lint
ifeq (,$(wildcard $(GOLANGCILINT)))
	curl -sSfL $(GOLANGCI_URL) | sh -s -- -b $(PROJECT_DIR)/bin $(GOLANGCI_VERSION)
endif

.PHONY: helm
HELM := $(PROJECT_DIR)/bin/helm
HELM_URL := https://get.helm.sh/helm-$(HELM_VERSION)-$(OS)-$(ARCH).tar.gz
helm: ## Download helm
ifeq (,$(wildcard $(HELM)))
	curl -sSL "$(HELM_URL)" | tar xzf - -C $(PROJECT_DIR)/bin --strip-components=1 --wildcards '*/helm'
endif

.PHONY: kuttl
KUTTL := $(PROJECT_DIR)/bin/kuttl
KUTTL_URL := https://github.com/kudobuilder/kuttl/releases/download/v$(KUTTL_VERSION)/kubectl-kuttl_$(KUTTL_VERSION)_$(OS)_x86_64
kuttl: ## Download kuttl
	$(call download-tool,$(KUTTL),$(KUTTL_URL))

.PHONY: operator-sdk
OPERATOR_SDK := $(PROJECT_DIR)/bin/operator-sdk
OPERATOR_SDK_URL := https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(OS)_$(ARCH)
operator-sdk: ## Download operator-sdk
	$(call download-tool,$(OPERATOR_SDK),$(OPERATOR_SDK_URL))
