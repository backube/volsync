# VolSync

Asynchronous volume replication for Kubernetes CSI storage

## About this operator

![maturity](https://img.shields.io/static/v1?label=maturity&message=alpha&color=red)

VolSync is a Kubernetes operator that performs asynchronous replication of
persistent volumes within, or across, clusters. VolSync supports replication
independent of the storage system. This means that replication can be used
with storage systems that do not natively support replication. Data can also be
replicated across different types (and vendors) of storage.

VolSync supports both 1:1 replication relationships as well as 1:many
relationships. This provides the flexibility to support use cases such as:

- Disaster recovery
- Mirroring data to a test environment
- Data distribution to a set of remote clusters from a central site
- Migrating between storage vendors (changing the StorageClass of a
  persistent volume claim).
- Creating periodic data backups

### How it works

You specify the details of how, when, and where to replicate the data
in a ReplicationSource object in the same namespace as the persistent
volume claim (PVC).

You create a ReplicationDestination object at the destination, which
specifies the target for the replicated data.

VolSync uses multiple replication methods to replicate data:

- Rclone-based replication for 1:many data distribution:  
  Data is replicated from the source to an intermediate cloud storage
  service, which is [supported by Rclone](https://rclone.org/#providers).
  The destinations retrieve the data from the intermediate location.
- Restic-based backup of PVC contents:  
  Data in a PVC is backed up by using the [restic](https://restic.net/)
  program. This method works well when the deployment configuration of
  the application is already source-controlled, and only the
  preservation of its persistent state is needed.
- Rsync-based replication for one-to-one data replication:  
  Data is replicated directly to a remote location. The replication uses
  the [Rsync](https://rsync.samba.org/) utility over an ssh connection
  to securely and efficiently transfer data.

**Please see the [📖 full documentation
📖](https://volsync.readthedocs.io/) for more details.**

## Requirements

- Kubernetes >= 1.17
- The Kubernetes snapshot controller must be installed on the cluster, whether
  or not clones & snapshots are used.
- CSI-based storage driver that supports snapshots and/or clones is recommended,
  but not required

## Installation

VolSync is a cluster-level operator. A single instance of the operator will
provide replication capabilities to all namespaces in the cluster.  
**Running more than one instance of VolSync at a time is not supported.**

```console
$ helm repo add backube-helm-charts https://backube.github.io/helm-charts/
$ helm install --create-namespace --namespace volsync-system volsync backube/volsync

NAME: volsync
LAST DEPLOYED: Thu Jan 28 13:52:18 2021
NAMESPACE: volsync-system
STATUS: deployed
REVISION: 1
TEST SUITE: None
NOTES:

The VolSync operator has been installed into the volsync-system namespace.

Please see https://volsync.readthedocs.org for documentation.
```

## Configuration

The following parameters in the chart can be configured, either by using `--set`
on the command line or via a custom `values.yaml` file.

- `manageCRDs`: true
  - Whether the chart should install/upgrade the VolSync CRDs
- `replicaCount`: `1`
  - The number of replicas of the operator to run. Only one is active at a time,
    controlled via leader election.
- `image.repository`: `quay.io/backube/volsync`
  - The container image of the VolSync operator
- `image.pullPolicy`: `IfNotPresent`
  - The image pull policy to apply to the operator's image
- `image.tag`: (current appVersion)
  - The tag to use when retrieving the operator image. This defaults to the tag
    for the current application version associated with this chart release.
- `image.image`: (empty)
  - Allows overriding the repository & tag as a single field to support
    specifying a specific container version by hash (e.g.,
    `quay.io/backube/volsync@sha256:XXXXXXX`).
- `rclone.repository`: `quay.io/backube/volsync-mover-rclone`
  - The container image for VolSync's rclone-based data mover
- `rclone.tag`: (current appVersion)
  - The tag to use for the rclone-based data mover
- `rclone.image`: (empty)
  - Allows overriding the repository & tag as a single field.
- `restic.repository`: `quay.io/backube/volsync-mover-restic`
  - The container image for VolSync's restic-based data mover
- `restic.tag`: (current appVersion)
  - The tag to use for the restic-based data mover
- `restic.image`: (empty)
  - Allows overriding the repository & tag as a single field.
- `rsync.repository`: `quay.io/backube/volsync-mover-rsync`
  - The container image for VolSync's rsync-based data mover
- `rsync.tag`: (current appVersion)
  - The tag to use for the rsync-based data mover
- `rsync.image`: (empty)
  - Allows overriding the repository & tag as a single field.
- `rsync-tls.repository`: `quay.io/backube/volsync-mover-rsync-tls`
  - The container image for VolSync's rsync-tls-based data mover
- `rsync-tls.tag`: (current appVersion)
  - The tag to use for the rsync-based data mover
- `rsync-tls.image`: (empty)
  - Allows overriding the repository & tag as a single field.
- `imagePullSecrets`: none
  - May be set if pull secret(s) are needed to retrieve the operator image
- `serviceAccount.create`: `true`
  - Whether to create the ServiceAccount for the operator
- `serviceAccount.annotations`: none
  - Annotations to add to the operator's service account
- `serviceAccount.name`: none
  - Override the name of the operator's ServiceAccount
- `podSecurityContext`: none
  - Allows setting the security context for the operator pod
- `podAnnotations`: none
  - Annotations to add to the operator's pod
- `securityContext`: none
  - Allows setting the operator container's security context
- `resources`: requests 100m CPU and 20Mi memory; limits 100m CPU and 300Mi
  memory
  - Allows overriding the resource requests/limits for the operator pod
- `nodeSelector`: none
  - Allows applying a node selector to the operator pod
- `tolerations`: none
  - Allows applying tolerations to the operator pod
- `affinity`: none
  - Allows setting the operator pod's affinity
