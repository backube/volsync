#! /bin/bash

set -e -o pipefail


# setup resources within the destination namespace
kubectl create ns dest \
    ; kubectl -n dest create -f examples/restic/source-restic/ \
    ; kubectl -n dest create -f examples/source-database/mysql-pvc.yaml \
    ; kubectl -n dest create -f examples/restic/volsync_v1alpha1_replicationdestination.yaml \
    ; kubectl -n dest create -f examples/destination-database/ 



