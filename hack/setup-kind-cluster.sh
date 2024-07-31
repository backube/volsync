#! /bin/bash

set -e -o pipefail

# Possible versions:
# https://hub.docker.com/r/kindest/node/tags?page=1&ordering=name
# skopeo list-tags docker://kindest/node
KUBE_VERSION="${1:-1.29.0}"

function log {
  echo "=====  $*  ====="
}

# Determine the Kube minor version
[[ "${KUBE_VERSION}" =~ ^[0-9]+\.([0-9]+) ]] && KUBE_MINOR="${BASH_REMATCH[1]}" || exit 1
log "Detected kubernetes minor version: ${KUBE_MINOR}"

KIND_CONFIG=""
KIND_CONFIG_FILE="$(mktemp --tmpdir kind-config-XXXXXX.yaml)"

# Enforce pod security standards
# https://kubernetes.io/docs/tasks/configure-pod-container/enforce-standards-admission-controller/#configure-the-admission-controller
# https://kubernetes.io/docs/tutorials/security/cluster-level-pss/
if [[ $KUBE_MINOR -ge 23 ]]; then
  log "Setting up PodSecurity admission"
  # Pod security controller has to be configured by passing a config file to
  # api-server on the command line
  SHARED_DIR="$(mktemp -d --tmpdir control-plane-shared-XXXXXX)"
  cat - > "${SHARED_DIR}/admission.yaml" <<ADMCFG
apiVersion: apiserver.config.k8s.io/v1
kind: AdmissionConfiguration
plugins:
- name: PodSecurity
  configuration:
    apiVersion: pod-security.admission.config.k8s.io/v1beta1
    kind: PodSecurityConfiguration
    # Defaults applied when a mode label is not set.
    #
    # Level label values must be one of:
    # - "privileged" (default)
    # - "baseline"
    # - "restricted"
    #
    # Version label values must be one of:
    # - "latest" (default)
    # - specific version like "v1.24"
    defaults:
      enforce: "privileged"
      enforce-version: "latest"
      audit: "restricted"
      audit-version: "latest"
      warn: "restricted"
      warn-version: "latest"
    exemptions:
      usernames: []
      runtimeClasses: []
      namespaces:
        - default             # CSI hostpath runs here
        - kube-system
        - local-path-storage  # default local storage provisioner
ADMCFG
  KIND_CONFIG="--config ${KIND_CONFIG_FILE}"
  cat - > "${KIND_CONFIG_FILE}" <<KINDCONFIG
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
nodes:
- role: control-plane
  kubeadmConfigPatches:
  - |
    kind: ClusterConfiguration
    apiServer:
        extraArgs:
          admission-control-config-file: /etc/config/admission.yaml
        extraVolumes:
          - name: config-files
            hostPath: /shared
            mountPath: /etc/config
            readOnly: false
            pathType: "DirectoryOrCreate"
  extraMounts:
  - hostPath: ${SHARED_DIR}
    containerPath: /shared
    readOnly: true
    selinuxRelabel: false
    propagation: HostToContainer
KINDCONFIG
fi

if [[ $KUBE_MINOR -le 16 ]]; then
  KIND_CONFIG="--config ${KIND_CONFIG_FILE}"
  cat - > "${KIND_CONFIG_FILE}" <<KINDCONFIG16
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
kubeadmConfigPatches:
- |
  kind: ClusterConfiguration
  metadata:
    name: config
  apiServer:
    extraArgs:
      "feature-gates": "VolumeSnapshotDataSource=true"
  scheduler:
    extraArgs:
      "feature-gates": "VolumeSnapshotDataSource=true"
  controllerManager:
    extraArgs:
      "feature-gates": "VolumeSnapshotDataSource=true"
- |
  kind: InitConfiguration
  metadata:
    name: config
  nodeRegistration:
    kubeletExtraArgs:
      "feature-gates": "VolumeSnapshotDataSource=true"
- |
  kind: KubeletConfiguration
  featureGates:
    VolumeSnapshotDataSource: true
KINDCONFIG16
fi

if [[ $KUBE_MINOR -le 13 ]]; then
  KIND_CONFIG="--config ${KIND_CONFIG_FILE}"
  cat - > "${KIND_CONFIG_FILE}" <<KINDCONFIG13
kind: Cluster
apiVersion: kind.x-k8s.io/v1alpha4
kubeadmConfigPatches:
- |
  kind: ClusterConfiguration
  metadata:
    name: config
  apiServer:
    extraArgs:
      "feature-gates": "CSIDriverRegistry=true,CSINodeInfo=true,VolumeSnapshotDataSource=true"
  scheduler:
    extraArgs:
      "feature-gates": "CSIDriverRegistry=true,CSINodeInfo=true,VolumeSnapshotDataSource=true"
  controllerManager:
    extraArgs:
      "feature-gates": "CSIDriverRegistry=true,CSINodeInfo=true,VolumeSnapshotDataSource=true"
- |
  kind: InitConfiguration
  metadata:
    name: config
  nodeRegistration:
    kubeletExtraArgs:
      "feature-gates": "CSIDriverRegistry=true,CSINodeInfo=true,VolumeSnapshotDataSource=true"
- |
  kind: KubeletConfiguration
  featureGates:
    CSIDriverRegistry: true
    CSINodeInfo: true
    VolumeSnapshotDataSource: true
KINDCONFIG13
fi

# Create the cluster
kind delete cluster || true
# shellcheck disable=SC2086
kind create cluster ${KIND_CONFIG} --image "kindest/node:v${KUBE_VERSION}"

rm -f "${KIND_CONFIG_FILE}"

# Kube >= 1.17, we need to deploy the snapshot controller
if [[ $KUBE_MINOR -ge 24 ]]; then  # Kube 1.24 removed snapshot.storage.k8s.io/v1beta1
  # renovate: datasource=github-releases depName=kubernetes-csi/external-snapshotter versioning=semver-coerced
  TAG="v8.0.1"  # https://github.com/kubernetes-csi/external-snapshotter/releases
  log "Deploying external snapshotter: ${TAG}"
  kubectl create -k "https://github.com/kubernetes-csi/external-snapshotter/client/config/crd?ref=${TAG}"
  kubectl create -n kube-system -k "https://github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/snapshot-controller?ref=${TAG}"

  # Deploy validating webhook server for snapshots: https://github.com/kubernetes-csi/external-snapshotter#validating-webhook
  log "Deploying validating webhook server for volumesnapshots: ${TAG}"
  EXT_SNAPSHOTTER_BASE="$(mktemp --tmpdir -d external-snapshotter-XXXXXX)"
  git clone --depth 1 -b "${TAG}" https://github.com/kubernetes-csi/external-snapshotter.git "${EXT_SNAPSHOTTER_BASE}"

  SNAP_WEBHOOK_PATH="${EXT_SNAPSHOTTER_BASE}/deploy/kubernetes/webhook-example"
  # webhook server need a TLS certificate - run script to generate and deploy secret to cluster (requires openssl)
  "${SNAP_WEBHOOK_PATH}"/create-cert.sh --service snapshot-validation-service --secret snapshot-validation-secret --namespace kube-system
  < "${SNAP_WEBHOOK_PATH}"/admission-configuration-template "${SNAP_WEBHOOK_PATH}"/patch-ca-bundle.sh > "${SNAP_WEBHOOK_PATH}"/admission-configuration.yaml

  # Update namespace in the example files
  for yamlfile in "${SNAP_WEBHOOK_PATH}"/*.yaml
  do
    sed -i s/'namespace: "default"'/'namespace: "kube-system"'/g "${yamlfile}"
    sed -i s/'namespace: default'/'namespace: kube-system'/g "${yamlfile}"
  done
  kubectl apply -f "${SNAP_WEBHOOK_PATH}"
  rm -rf "${EXT_SNAPSHOTTER_BASE}"

elif [[ $KUBE_MINOR -ge 20 ]]; then  # Kube 1.20 added snapshot.storage.k8s.io/v1
  TAG="v5.0.1"  # https://github.com/kubernetes-csi/external-snapshotter/releases
  log "Deploying external snapshotter: ${TAG}"
  kubectl create -k "https://github.com/kubernetes-csi/external-snapshotter/client/config/crd?ref=${TAG}"
  kubectl create -n kube-system -k "https://github.com/kubernetes-csi/external-snapshotter/deploy/kubernetes/snapshot-controller?ref=${TAG}"
elif [[ $KUBE_MINOR -ge 17 ]]; then  # Kube 1.17 switched snapshot.storage.k8s.io/v1alpha1 -> v1beta1
  TAG="v3.0.3"  # https://github.com/kubernetes-csi/external-snapshotter/releases
  log "Deploying external snapshotter: ${TAG}"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"

  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"
fi

if [[ $KUBE_MINOR -ge 24 ]]; then # Volume Populators should work (AnyVolumeDataSource feature gate enabled by default)
  # Install the volume-data-source-validator (For validating PVC dataRefSource against known VolumePopulators)
  # renovate: datasource=github-releases depName=kubernetes-csi/volume-data-source-validator versioning=semver-coerced
  TAG="v1.3.0" # https://github.com/kubernetes-csi/volume-data-source-validator/releases
  log "Deploying volume data source validator: ${TAG}"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/volume-data-source-validator/${TAG}/client/config/crd/populator.storage.k8s.io_volumepopulators.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/volume-data-source-validator/${TAG}/deploy/kubernetes/rbac-data-source-validator.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/volume-data-source-validator/${TAG}/deploy/kubernetes/setup-data-source-validator.yaml"
fi

# Kube 1.13 requires CSIDriver & CSINodeInfo CRDs
if [[ $KUBE_MINOR -eq 13 ]]; then
  kubectl create -f https://raw.githubusercontent.com/kubernetes/csi-api/master/pkg/crd/manifests/csidriver.yaml
  kubectl create -f https://raw.githubusercontent.com/kubernetes/csi-api/master/pkg/crd/manifests/csinodeinfo.yaml
fi

# Install the hostpath CSI driver
# https://github.com/kubernetes-csi/csi-driver-host-path/releases
HP_BASE="$(mktemp --tmpdir -d csi-driver-host-path-XXXXXX)"
case "$KUBE_MINOR" in
  13)
    TAG="v1.1.0"
    DEPLOY_SCRIPT="deploy-hostpath.sh"
    ;;
  14)
    TAG="v1.2.0"
    DEPLOY_SCRIPT="deploy-hostpath.sh"
    ;;
  15)
    TAG="v1.3.0"
    DEPLOY_SCRIPT="deploy-hostpath.sh"
    ;;
  16|17)
    TAG="v1.4.0"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
  18)
    TAG="v1.7.2"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
  19|20)
    TAG="v1.7.3"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
  21)
    TAG="v1.9.0"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
  22|23)
    TAG="v1.10.0"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
  *)
    # renovate: datasource=github-releases depName=kubernetes-csi/csi-driver-host-path versioning=semver-coerced
    TAG="v1.14.1"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
esac
log "Deploying CSI hostpath driver: ${TAG}"
git clone --depth 1 -b "$TAG" https://github.com/kubernetes-csi/csi-driver-host-path.git "$HP_BASE"

DEPLOY_PATH="${HP_BASE}/deploy/kubernetes-1.${KUBE_MINOR}/"
# For versions not yet supported, use the latest
if [[ ! -d "${DEPLOY_PATH}" ]]; then
  DEPLOY_PATH="${HP_BASE}/deploy/kubernetes-latest/"
fi

# Remove the CSI testing manifest. It exposes csi.sock as a TCP socket using
# socat. This is insecure, but the larger problem is that it pulls the socat
# image from docker.io, making it subject to rate limits that break this script.
rm -f "${DEPLOY_PATH}/hostpath/csi-hostpath-testing.yaml"

"${DEPLOY_PATH}/${DEPLOY_SCRIPT}"
rm -rf "${HP_BASE}"

CSI_DRIVER_NAME="hostpath.csi.k8s.io"
if [[ $KUBE_MINOR -eq 13 ]]; then
  CSI_DRIVER_NAME="csi-hostpath"
fi
log "Creating StorageClass for CSI hostpath driver"
kubectl apply -f - <<SC
apiVersion: storage.k8s.io/v1
kind: StorageClass
metadata:
  name: csi-hostpath-sc
provisioner: ${CSI_DRIVER_NAME}
reclaimPolicy: Delete
volumeBindingMode: Immediate
allowVolumeExpansion: true
SC

# Change the default SC
kubectl annotate sc/standard storageclass.kubernetes.io/is-default-class-
kubectl annotate sc/csi-hostpath-sc storageclass.kubernetes.io/is-default-class="true"

# For some versions we need to create the snapclass ourselves
if [[ $KUBE_MINOR -eq 15 || $KUBE_MINOR -eq 16 ]]; then
  log "Creating VolumeStorageClass for CSI hostpath driver"
  kubectl create -f - <<SNAPALPHA
apiVersion: snapshot.storage.k8s.io/v1alpha1
kind: VolumeSnapshotClass
metadata:
  name: csi-hostpath-snapclass
snapshotter: hostpath.csi.k8s.io
SNAPALPHA
fi

# Make VSC the cluster default
kubectl annotate volumesnapshotclass/csi-hostpath-snapclass snapshot.storage.kubernetes.io/is-default-class="true"

# On 1.20, we need to enable fsGroup so that volumes are writable
# Release v1.9.0 included this change for 1.21+
if [[ $KUBE_MINOR -eq 20 ]]; then
  log "Enabling fsGroupPolicy for CSI driver"
  kubectl delete csidriver hostpath.csi.k8s.io
  kubectl create -f - <<DRIVERINFO
apiVersion: storage.k8s.io/v1
kind: CSIDriver
metadata:
  name: hostpath.csi.k8s.io
  labels:
    app.kubernetes.io/instance: hostpath.csi.k8s.io
    app.kubernetes.io/part-of: csi-driver-host-path
    app.kubernetes.io/name: hostpath.csi.k8s.io
    app.kubernetes.io/component: csi-driver
spec:
  volumeLifecycleModes:
  - Persistent
  - Ephemeral
  podInfoOnMount: true
  fsGroupPolicy: File
DRIVERINFO
fi

# Add a node topology key so that e2e tests can run
kubectl label nodes --all topology.kubernetes.io/zone=z1
