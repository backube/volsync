#! /bin/bash

set -e -o pipefail

echo "Starting minio server"

../../../hack/run-minio.sh

MINIO_ACCESS_KEY=$(kubectl get secret --namespace "$NAMESPACE" minio -o jsonpath="{.data.access-key}" | base64 --decode)
MINIO_SECRET_KEY=$(kubectl get secret --namespace "$NAMESPACE" minio -o jsonpath="{.data.secret-key}" | base64 --decode)

cat <<EOF >./rclone.conf
[rclone-data-mover]
type = s3
provider = Minio
env_auth = false
access_key_id = $MINIO_ACCESS_KEY
secret_access_key = $MINIO_SECRET_KEY
region = us-east-1
endpoint = http://minio.$NAMESPACE.svc.cluster.local:9000
EOF

kubectl create secret generic rclone-secret --from-file=rclone.conf=./rclone.conf -n "$NAMESPACE"