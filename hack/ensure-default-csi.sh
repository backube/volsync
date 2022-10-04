#! /bin/bash

set -e -o pipefail

# Makes sure the default storagedriver is a csi one

DEFAULT_STORAGE_CLASS=$(kubectl get storageclasses -o=jsonpath='{range .items[?(.metadata.annotations.storageclass\.kubernetes\.io/is-default-class=="true")]}{.metadata.name}{" "}{.provisioner}')

DEFAULT_STORAGE_CLASS_NAME=$(echo "${DEFAULT_STORAGE_CLASS}" | awk '{print $1}')
DEFAULT_PROVISIONER=$(echo "${DEFAULT_STORAGE_CLASS}" | awk '{print $2}')

if [[ -n "${DEFAULT_STORAGE_CLASS_NAME}" ]]; then
  # Found a default storage class
  if [[ "${DEFAULT_PROVISIONER}" =~ .*"csi".* ]]; then
    echo "Default storage class ${DEFAULT_STORAGE_CLASS_NAME} is csi, no updates required."
    exit 0
  fi

  # We need to remove the default annotation
  kubectl annotate sc/"${DEFAULT_STORAGE_CLASS_NAME}" storageclass.kubernetes.io/is-default-class-
fi

# No default set at this point, try to find a csi driver to use as default
STORAGE_CLASSES=$(kubectl get storageclasses -o=jsonpath='{range .items[*]}{@.metadata.name}{"\t"}{@.provisioner}{"\n"}{end}')

IFS=$'\n'
for sc in ${STORAGE_CLASSES}; do
  echo "sc is ${sc}"
  SC_NAME=$(echo "${sc}" | awk '{print $1}')
  PROVISIONER=$(echo "${sc}" | awk '{print $2}')

  if [[ "${PROVISIONER}" =~ .*"csi".* ]]; then
    echo "Updating storage class ${SC_NAME} as the default ..."
    kubectl annotate sc/"${SC_NAME}" storageclass.kubernetes.io/is-default-class="true"

    exit 0
  fi
done

