#! /bin/bash

set -e -o pipefail

# Possible versions:
# https://hub.docker.com/r/kindest/node/tags?page=1&ordering=name
# skopeo inspect docker://kindest/node:v1.17.0 | jq .RepoTags
KUBE_VERSION="${1:-1.20.2}"

# Determine the Kube minor version
[[ "${KUBE_VERSION}" =~ ^[0-9]+\.([0-9]+) ]] && KUBE_MINOR="${BASH_REMATCH[1]}" || exit 1

KIND_CONFIG=""
KIND_CONFIG_FILE="$(mktemp --tmpdir kind-config-XXXXXX.yaml)"

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
if [[ $KUBE_MINOR -ge 20 ]]; then
  TAG="v4.0.0"  # https://github.com/kubernetes-csi/external-snapshotter/releases
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"

  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"
elif [[ $KUBE_MINOR -ge 17 ]]; then
  TAG="v3.0.3"  # https://github.com/kubernetes-csi/external-snapshotter/releases
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotclasses.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshotcontents.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/client/config/crd/snapshot.storage.k8s.io_volumesnapshots.yaml"

  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/deploy/kubernetes/snapshot-controller/rbac-snapshot-controller.yaml"
  kubectl create -f "https://raw.githubusercontent.com/kubernetes-csi/external-snapshotter/${TAG}/deploy/kubernetes/snapshot-controller/setup-snapshot-controller.yaml"
fi

# Kube 1.13 requires CSIDriver & CSINodeInfo CRDs
if [[ $KUBE_MINOR -eq 13 ]]; then
  kubectl create -f https://raw.githubusercontent.com/kubernetes/csi-api/master/pkg/crd/manifests/csidriver.yaml
  kubectl create -f https://raw.githubusercontent.com/kubernetes/csi-api/master/pkg/crd/manifests/csinodeinfo.yaml
fi

# Install the hostpath CSI driver
HP_BASE="$(mktemp --tmpdir -d csi-driver-host-path-XXXXXX)"
case "$KUBE_MINOR" in
  13)
    HOSTPATH_BRANCH="v1.1.0"
    DEPLOY_SCRIPT="deploy-hostpath.sh"
    ;;
  14)
    HOSTPATH_BRANCH="v1.2.0"
    DEPLOY_SCRIPT="deploy-hostpath.sh"
    ;;
  15)
    HOSTPATH_BRANCH="v1.3.0"
    DEPLOY_SCRIPT="deploy-hostpath.sh"
    ;;
  16|17)
    HOSTPATH_BRANCH="v1.4.0"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
  *)
    HOSTPATH_BRANCH="v1.6.0"
    DEPLOY_SCRIPT="deploy.sh"
    ;;
esac
git clone --depth 1 -b "$HOSTPATH_BRANCH" https://github.com/kubernetes-csi/csi-driver-host-path.git "$HP_BASE"

DEPLOY_PATH="${HP_BASE}/deploy/kubernetes-1.${KUBE_MINOR}/"
# For versions not yet supported, use the latest
if [[ ! -d "${DEPLOY_PATH}" ]]; then
  DEPLOY_PATH="${HP_BASE}/deploy/kubernetes-latest/"
fi
"${DEPLOY_PATH}/${DEPLOY_SCRIPT}"
rm -rf "${HP_BASE}"

CSI_DRIVER_NAME="hostpath.csi.k8s.io"
if [[ $KUBE_MINOR -eq 13 ]]; then
  CSI_DRIVER_NAME="csi-hostpath"
fi
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
