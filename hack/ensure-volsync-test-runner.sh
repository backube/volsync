#! /bin/bash

set -e -o pipefail

SA_ACCOUNT_NAME=volsync-test-runner
NAMESPACE=$(kubectl config view --minify -o jsonpath='{..namespace}')

if [ -z "$NAMESPACE" ]; then
  NAMESPACE=default
fi

# Make sure service account exists in the current namespace
echo "Creating/Updating ${SA_ACCOUNT_NAME} service account in namespace ${NAMESPACE} for volsync custom-scorecard-tests ..."
kubectl apply -f - <<SVCACCOUNT
apiVersion: v1
kind: ServiceAccount
metadata:
  name: ${SA_ACCOUNT_NAME}
  namespace: ${NAMESPACE}
SVCACCOUNT

# Make sure the svc account has cluster:admin privileges
kubectl apply -f - <<CROLEBINDING
apiVersion: rbac.authorization.k8s.io/v1
kind: ClusterRoleBinding
metadata:
  name: ${SA_ACCOUNT_NAME}-binding
roleRef:
  apiGroup: rbac.authorization.k8s.io
  kind: ClusterRole
  name: cluster-admin
subjects:
- kind: ServiceAccount
  name: ${SA_ACCOUNT_NAME}
  namespace: ${NAMESPACE}
CROLEBINDING

echo "DONE."
