#! /bin/bash

set -e -o pipefail


updateDefaultStorageClass() {
  # Try to find a csi driver to use as default
  STORAGE_CLASSES=$(kubectl get storageclasses -o=jsonpath='{range .items[*]}{@.metadata.name}{"\t"}{@.provisioner}{"\n"}{end}')

  IFS=$'\n'
  for sc in ${STORAGE_CLASSES}; do
    echo "sc is ${sc}"
    SC_NAME=$(echo "${sc}" | awk '{print $1}')
    PROVISIONER=$(echo "${sc}" | awk '{print $2}')

    if [[ "${PROVISIONER}" =~ .*"csi".* ]]; then
      echo "Updating storage class ${SC_NAME} as the default ..."
      kubectl annotate sc/"${SC_NAME}" storageclass.kubernetes.io/is-default-class="true"

      # Update DEFAULT_PROVISIONER
      DEFAULT_PROVISIONER=${PROVISIONER}

      return 0
    fi
  done
}

updateDefaultVolumeSnapshotClass() {
  DRIVER_TO_USE=$1

  VSCS=$(kubectl get volumesnapshotclasses -o=jsonpath='{range .items[*]}{@.metadata.name}{"\t"}{@.driver}{"\n"}{end}')
  IFS=$'\n'
  for vsc in ${VSCS}; do
    echo "vsc is ${vsc}"
    VSC_NAME=$(echo "${vsc}" | awk '{print $1}')
    DRIVER=$(echo "${vsc}" | awk '{print $2}')

    if [[ "${DRIVER}" == "${DRIVER_TO_USE}" ]]; then
      echo "Updating volume snapshot class ${VSC_NAME} as the default ..."
      kubectl annotate volumesnapshotclass/"${VSC_NAME}" snapshot.storage.kubernetes.io/is-default-class="true"

      return 0
    fi
  done

  # At this point no volume snapshot class was found that matches the driver - create one
  echo "Creating VolumeStorageClass for ${DRIVER_TO_USE} driver ..."
  kubectl create -f - <<SNAPCLASS
apiVersion: snapshot.storage.k8s.io/v1
kind: VolumeSnapshotClass
metadata:
  name: test-csi-snap
  annotations:
    snapshot.storage.kubernetes.io/is-default-class: "true"
driver: ${DRIVER_TO_USE}
deletionPolicy: Delete
SNAPCLASS

}

# Log storageclasses and volumesnapshotclasses (for debug purposes)
echo "### Current storageclasses ###"
kubectl get storageclasses
echo "### Current volumesnapshotclasses ###"
kubectl get volumesnapshotclasses
echo ""

# First make sure the default storagedriver is a csi one
DEFAULT_STORAGE_CLASS=$(kubectl get storageclasses -o=jsonpath='{range .items[?(.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")]}{.metadata.name}{" "}{.provisioner}')

DEFAULT_STORAGE_CLASS_NAME=$(echo "${DEFAULT_STORAGE_CLASS}" | awk '{print $1}')
DEFAULT_PROVISIONER=$(echo "${DEFAULT_STORAGE_CLASS}" | awk '{print $2}')

if [[ -n "${DEFAULT_STORAGE_CLASS_NAME}" ]]; then
  # Found a default storage class
  if [[ "${DEFAULT_PROVISIONER}" =~ .*"csi".* ]]; then
    echo "Default storage class ${DEFAULT_STORAGE_CLASS_NAME} is csi, no update required."
  else
    echo "Removing default annotation on storageclass ${DEFAULT_STORAGE_CLASS_NAME} ..."
    # We need to remove the default annotation
    kubectl annotate sc/"${DEFAULT_STORAGE_CLASS_NAME}" storageclass.kubernetes.io/is-default-class-

    updateDefaultStorageClass
  fi
else
  updateDefaultStorageClass
fi

# Make sure the volumesnapshotclass has a default and uses the proper provisioner
DEFAULT_VSC=$(kubectl get volumesnapshotclasses -o=jsonpath='{range .items[?(.metadata.annotations.snapshot\.storage\.kubernetes\.io/is-default-class=="true")]}{.metadata.name}{" "}{.driver}')

DEFAULT_VSC_NAME=$(echo "${DEFAULT_VSC}" | awk '{print $1}')
DEFAULT_VSC_DRIVER=$(echo "${DEFAULT_VSC}" | awk '{print $2}')

if [[ -n "${DEFAULT_VSC_NAME}" ]]; then
  # Found a default volume snapshot class
  if [[ "${DEFAULT_VSC_DRIVER}" == "${DEFAULT_PROVISIONER}" ]]; then
    echo "Default volume snapshot class ${DEFAULT_VSC_NAME} uses driver ${DEFAULT_PROVISIONER}, no update required."
  else
    echo "Removing default annotation on volume snapshot class ${DEFAULT_VSC_NAME} ..."
    kubectl annotate volumesnapshotclass/"${DEFAULT_VSC_NAME}" snapshot.storage.kubernetes.io/is-default-class-

    updateDefaultVolumeSnapshotClass "${DEFAULT_PROVISIONER}"
  fi
else
  updateDefaultVolumeSnapshotClass "${DEFAULT_PROVISIONER}"
fi
