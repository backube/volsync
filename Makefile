# VERSION defines the project version for the bundle.
# Update this value when you upgrade the version of your project.
# To re-generate a bundle for another specific version without changing the standard setup, you can:
# - use the VERSION as arg of the bundle target (e.g make bundle VERSION=0.0.2)
# - use environment variables to overwrite this value (e.g export VERSION=0.0.2)
#VERSION ?= $(shell git describe --tags --dirty --match 'v*' 2> /dev/null || git describe --always --dirty)

# Include common version information from the version.mk file
include ./version.mk

# Helper software versions
CONTROLLER_TOOLS_VERSION := v0.10.0
ENVTEST_K8S_VERSION = 1.25.0
GOLANGCI_VERSION := v1.51.0
HELM_VERSION := v3.8.2
KUBECTL_VERSION := v1.25.2
KUSTOMIZE_VERSION := v4.5.4
OPERATOR_SDK_VERSION := v1.26.0
PIPENV_VERSION := 2022.8.30

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

# BUNDLE_GEN_FLAGS are the flags passed to the operator-sdk generate bundle command
BUNDLE_GEN_FLAGS ?= -q --overwrite --version $(VERSION) $(BUNDLE_METADATA_OPTS)

# USE_IMAGE_DIGESTS defines if images are resolved via tags or digests
# You can enable this value if you would like to use SHA Based Digests
# To enable set flag to true
USE_IMAGE_DIGESTS ?= false
ifeq ($(USE_IMAGE_DIGESTS), true)
	BUNDLE_GEN_FLAGS += --use-image-digests
endif

# Image URL to use all building/pushing image targets
IMG ?= quay.io/backube/volsync:latest

# Get the currently used golang install path (in GOPATH/bin, unless GOBIN is set)
ifeq (,$(shell go env GOBIN))
GOBIN=$(shell go env GOPATH)/bin
else
GOBIN=$(shell go env GOBIN)
endif

# Setting SHELL to bash allows bash commands to be executed by recipes.
# Options are set to exit when a recipe line exits non-zero or a piped command fails.
SHELL = /usr/bin/env bash -o pipefail
.SHELLFLAGS = -ec

.PHONY: all
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

.PHONY: auto-generated-files
auto-generated-files: bundle custom-scorecard-tests-generate-config generate manifests ## Update all the automatically generated files

.PHONY: manifests
manifests: controller-gen yq ## Generate WebhookConfiguration, ClusterRole and CustomResourceDefinition objects.
	$(CONTROLLER_GEN) rbac:roleName=manager-role crd webhook paths="./..." output:crd:artifacts:config=config/crd/bases
	@{ \
		for SRC in config/crd/bases/*.yaml; do \
			DST="helm/volsync/templates/$$(basename "$$SRC")"; \
			echo "{{- if .Values.manageCRDs }}" > "$$DST"; \
			$(YQ) '.metadata.annotations."helm.sh/resource-policy"="keep"' "$$SRC" >> "$$DST"; \
			echo "{{- end }}" >> "$$DST"; \
		done; \
	}

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
TEST_ARGS ?= --randomize-all --randomize-suites -p -cover -coverprofile cover.out --output-dir . --skip-package mover-restic
TEST_PACKAGES ?= ./...
test: bundle generate lint envtest helm-lint ginkgo ## Run tests.
	-rm -f cover.out
	KUBEBUILDER_ASSETS="$(shell $(ENVTEST) use $(ENVTEST_K8S_VERSION) --bin-dir $(LOCALBIN) -p path)" $(GINKGO) $(TEST_ARGS) $(TEST_PACKAGES)

.PHONY: test-e2e-install
test-e2e-install: ## Install environment for running e2e
	./.ci-scripts/retry.sh pip install --user --upgrade pipenv==$(PIPENV_VERSION)
	cd test-e2e && ../.ci-scripts/retry.sh pipenv install --deploy --no-site-packages -v
	cd test-e2e && ../.ci-scripts/retry.sh pipenv run ansible-galaxy install -r requirements.yml

.PHONY: test-e2e
test-e2e: ## Run e2e tests. Requires cluster w/ VolSync + minio already installed
	./test-e2e/run_tests_in_parallel.sh

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
docker-build:  ## Build docker image with the manager.
	docker build --build-arg "builddate_arg=$(BUILDDATE)" --build-arg "version_arg=$(BUILD_VERSION)" -t ${IMG} .

.PHONY: docker-push
docker-push: ## Push docker image with the manager.
	docker push ${IMG}

# PLATFORMS defines the target platforms for  the manager image be build to provide support to multiple
# architectures. (i.e. make docker-buildx IMG=myregistry/mypoperator:0.0.1). To use this option you need to:
# - able to use docker buildx . More info: https://docs.docker.com/build/buildx/
# - have enable BuildKit, More info: https://docs.docker.com/develop/develop-images/build_enhancements/
# - be able to push the image for your registry (i.e. if you do not inform a valid value via IMG=<myregistry/image:<tag>> than the export will fail)
# To properly provided solutions that supports more than one platform you should use this option.
PLATFORMS ?= linux/arm64,linux/amd64,linux/s390x,linux/ppc64le
.PHONY: docker-buildx
docker-buildx: test ## Build and push docker image for the manager for cross-platform support
	# copy existing Dockerfile and insert --platform=${BUILDPLATFORM} into Dockerfile.cross, and preserve the original Dockerfile
	sed -e '1 s/\(^FROM\)/FROM --platform=\$$\{BUILDPLATFORM\}/; t' -e ' 1,// s//FROM --platform=\$$\{BUILDPLATFORM\}/' Dockerfile > Dockerfile.cross
	- docker buildx create --name project-v3-builder
	docker buildx use project-v3-builder
	- docker buildx build --push --platform=$(PLATFORMS) --tag ${IMG} -f Dockerfile.cross
	- docker buildx rm project-v3-builder
	rm Dockerfile.cross

.PHONY: krew-plugin-manifest
krew-plugin-manifest: yq bin/kubectl-volsync ## Build & package the kubectl plugin & update the krew manifest
	rm -f bin/kubectl-volsync.tar.gz
	tar czf bin/kubectl-volsync.tar.gz LICENSE -C bin kubectl-volsync
	HASH="`sha256sum bin/kubectl-volsync.tar.gz | cut -f 1 -d ' '`" \
	VERSION="v$(VERSION)" \
	$(YQ) --inplace '.spec.version=strenv(VERSION) | with(.spec.platforms[]; .sha256=strenv(HASH) | .uri|=sub("v[[:digit:]]+\.[^/]+", strenv(VERSION)))' ./kubectl-volsync/volsync.yaml

##@ Deployment

ifndef ignore-not-found
  ignore-not-found = false
endif

.PHONY: install
install: manifests kustomize ## Install CRDs into the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/crd | kubectl apply -f -

.PHONY: uninstall
uninstall: manifests kustomize ## Uninstall CRDs from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/crd | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: deploy
deploy: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/default | kubectl apply -f -

.PHONY: deploy-openshift
deploy-openshift: manifests kustomize ## Deploy controller to the K8s cluster specified in ~/.kube/config.
	cd config/manager && $(KUSTOMIZE) edit set image controller=${IMG}
	$(KUSTOMIZE) build config/openshift | kubectl apply -f -

.PHONY: undeploy
undeploy: manifests kustomize ## Undeploy controller from the K8s cluster specified in ~/.kube/config. Call with ignore-not-found=true to ignore resource not found errors during deletion.
	$(KUSTOMIZE) build config/default | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

.PHONY: undeploy-openshift
undeploy-openshift: manifests kustomize ## Undeploy controller to the K8s cluster specified in ~/.kube/config.
	$(KUSTOMIZE) build config/openshift | kubectl delete --ignore-not-found=$(ignore-not-found) -f -

##@ Build Dependencies

## Location to install dependencies to
LOCALBIN ?= $(shell pwd)/bin
$(LOCALBIN):
	mkdir -p $(LOCALBIN)

.PHONY: controller-gen
CONTROLLER_GEN := $(LOCALBIN)/controller-gen
controller-gen: $(CONTROLLER_GEN) ## Download controller-gen locally if necessary.
# -mod=mod is a workaround for a go bug and can probably be removed when we move to go 1.20
$(CONTROLLER_GEN): $(LOCALBIN)
	test -s $@ || GOBIN=$(LOCALBIN) go install -mod=mod sigs.k8s.io/controller-tools/cmd/controller-gen@$(CONTROLLER_TOOLS_VERSION)

.PHONY: kustomize
KUSTOMIZE := $(LOCALBIN)/kustomize
kustomize: $(KUSTOMIZE) ## Download kustomize locally if necessary.
$(KUSTOMIZE): $(LOCALBIN)
	test -s $@ || GOBIN=$(LOCALBIN) go install sigs.k8s.io/kustomize/kustomize/v4@$(KUSTOMIZE_VERSION)

.PHONY: envtest
ENVTEST := $(LOCALBIN)/setup-envtest
envtest: $(ENVTEST) ## Download envtest-setup locally if necessary.
$(ENVTEST): $(LOCALBIN)
	test -s $@ || GOBIN=$(LOCALBIN) go install sigs.k8s.io/controller-runtime/tools/setup-envtest@latest

.PHONY: yq
YQ := $(LOCALBIN)/yq
yq: $(YQ) ## Download yq locally if necessary.
$(YQ): $(LOCALBIN)
	test -s $@ || GOFLAGS= GOBIN=$(LOCALBIN) go install github.com/mikefarah/yq/v4@v4.34.2

.PHONY: bundle
bundle: manifests kustomize operator-sdk ## Generate bundle manifests and metadata, then validate generated files.
	$(OPERATOR_SDK) generate kustomize manifests -q
	cd config/manager && $(KUSTOMIZE) edit set image controller=$(IMG)
	$(KUSTOMIZE) build config/manifests | \
		sed "s/MIN_KUBE_VERSION/$(MIN_KUBE_VERSION)/" | \
		sed "s/OLM_SKIPRANGE/$(OLM_SKIPRANGE)/" | \
		sed "s/CSV_REPLACES_VERSION/$(CSV_REPLACES_VERSION)/" | \
		$(OPERATOR_SDK) generate bundle $(BUNDLE_GEN_FLAGS)
	$(OPERATOR_SDK) bundle validate --select-optional suite=operatorframework --optional-values k8s-version=$(ENVTEST_K8S_VERSION) ./bundle

.PHONY: bundle-build
bundle-build: ## Build the bundle image.
	docker build -f bundle.Dockerfile -t $(BUNDLE_IMG) .

.PHONY: bundle-push
bundle-push: ## Push the bundle image.
	$(MAKE) docker-push IMG=$(BUNDLE_IMG)

.PHONY: opm
OPM := $(LOCALBIN)/opm
opm: $(OPM) ## Download opm locally if necessary.
$(OPM): $(LOCALBIN)
	@{ \
	set -e ;\
	mkdir -p $(dir $(OPM)) ;\
	OS=$(shell go env GOOS) && ARCH=$(shell go env GOARCH) && \
	curl -sSLo $(OPM) https://github.com/operator-framework/operator-registry/releases/download/v1.23.0/$${OS}-$${ARCH}-opm ;\
	chmod +x $(OPM) ;\
	}

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


# Name of volsync custom scorecard test image
CUSTOM_SCORECARD_IMG_TAG ?= latest
CUSTOM_SCORECARD_IMG ?= $(IMAGE_TAG_BASE)-custom-scorecard-tests:$(CUSTOM_SCORECARD_IMG_TAG)

# Build the custom scorecard image - this can be used to run e2e tests using operator-sdk
# See more info here: https://sdk.operatorframework.io/docs/testing-operators/scorecard/custom-tests/
.PHONY: custom-scorecard-tests-build
custom-scorecard-tests-build:
	docker build --build-arg "version_arg=$(BUILD_VERSION)" --build-arg "pipenv_version_arg=$(PIPENV_VERSION)" --build-arg "helm_version_arg=$(HELM_VERSION)" --build-arg "kubectl_version_arg=$(KUBECTL_VERSION)" -f Dockerfile.volsync-custom-scorecard-tests -t ${CUSTOM_SCORECARD_IMG} .

.PHONY: custom-scorecard-tests-generate-config
custom-scorecard-tests-generate-config: kustomize
	cd custom-scorecard-tests && ./generateE2ETestsConfig.sh ${CUSTOM_SCORECARD_IMG}
	cd custom-scorecard-tests && $(KUSTOMIZE) build scorecard > config.yaml


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
GINKGO := $(LOCALBIN)/ginkgo
ginkgo: $(GINKGO) ## Download ginkgo
$(GINKGO): $(LOCALBIN)
	test -s $@ || GOBIN=$(LOCALBIN) go install github.com/onsi/ginkgo/v2/ginkgo@latest

.PHONY: golangci-lint
GOLANGCILINT := $(LOCALBIN)/golangci-lint
GOLANGCI_URL := https://raw.githubusercontent.com/golangci/golangci-lint/master/install.sh
golangci-lint: $(GOLANGCILINT) ## Download golangci-lint
$(GOLANGCILINT): $(LOCALBIN)
	test -s $@ || { curl -sSfL $(GOLANGCI_URL) | sh -s -- -b $(LOCALBIN) $(GOLANGCI_VERSION); }

.PHONY: helm
HELM := $(LOCALBIN)/helm
HELM_URL := https://get.helm.sh/helm-$(HELM_VERSION)-$(OS)-$(ARCH).tar.gz
helm: $(HELM) ## Download helm
$(HELM): $(LOCALBIN)
	test -s $@ || { curl -sSL "$(HELM_URL)" | tar xzf - -C $(LOCALBIN) --strip-components=1 --wildcards '*/helm'; }

.PHONY: operator-sdk
OPERATOR_SDK := $(LOCALBIN)/operator-sdk
OPERATOR_SDK_URL := https://github.com/operator-framework/operator-sdk/releases/download/$(OPERATOR_SDK_VERSION)/operator-sdk_$(OS)_$(ARCH)
operator-sdk: $(OPERATOR_SDK) ## Download operator-sdk
$(OPERATOR_SDK): $(LOCALBIN)
	test -s $@ || $(call download-tool,$(OPERATOR_SDK),$(OPERATOR_SDK_URL))
