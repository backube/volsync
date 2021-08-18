#!/bin/bash

set -e -o pipefail


kubectl create ns source \
    ; kubectl -n source create -f examples/source-database/ \
    ; ./hack/run-minio.sh \
    ; kubectl create -f examples/restic/source-restic/source-restic.yaml -n source \
    ; kubectl create -f examples/restic/volsync_v1alpha1_replicationsource.yaml -n source 

    # kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash
    # ./hack/run-minio.sh

    # kubectl port-forward --namespace minio svc/minio 9000:9000 > /dev/null &
