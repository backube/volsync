# Scribe

Asynchronous volume replication for Kubernetes CSI storage

## About this operator

![maturity](https://img.shields.io/static/v1?label=maturity&message=alpha&color=red)

Scribe is a Kubernetes operator that performs asynchronous replication of
persistent volumes within, or across, clusters. Scribe supports replication in a
storage system independent manner. This means replication can be used with
storage systems that do not support replication natively. Data can also be
replicated across different types (and vendors) of storage.

Scribe supports both 1:1 replication relationships as well as 1:many
relationships. This provides the flexibility to support use cases such as
disaster recovery, mirroring data to a test environment, or data distribution to
a set of remote clusters from a central site.

### How it works

A ReplicationSource object in the same Namespace as the volume (PVC) to be
replicated determines how, when, and to where the data should be replicated.

A ReplicationDestination object at the destination serves as the target for the
replicated data.

Scribe has several replication methods than can be used to replicate data.

- Rclone-based replication for 1:many data distribution  
  With this replication method, data is replicated from the source to an
  intermediate cloud storage service ([supported by
  Rclone](https://rclone.org/#providers)). The destination(s) then retrieve the
  data from this intermediate location.
- Rsync-based replication for 1:1 data replication  
  This replication method is designed to replicate data directly to a remote
  location. It uses [Rsync](https://rsync.samba.org/) over an ssh connection to
  securely and efficiently transfer data.

**Please see the [📖 full documentation
📖](https://scribe-replication.readthedocs.io/) for more details.**

## Requirements

- Kubernetes >= 1.17
- The Kubernetes snapshot controller must be installed on the cluster, whether
  or not clones & snapshots are used.
- CSI-based storage driver that supports snapshots and/or clones is recommended,
  but not required

## Installation

Scribe is a cluster-level operator. A single instance of the operator will
provide replication capabilities to all namespaces in the cluster.  
**Running more than one instance of Scribe at a time is not supported.**

```console
$ helm install --create-namespace --namespace scribe-system scribe backube/scribe

NAME: scribe
LAST DEPLOYED: Thu Jan 28 13:52:18 2021
NAMESPACE: scribe-system
STATUS: deployed
REVISION: 1
TEST SUITE: None
NOTES:

The Scribe operator has been installed into the scribe-system namespace.

Please see https://scribe-replication.readthedocs.org for documentation.
```

## Configuration

The following parameters in the chart can be configured, either by using `--set`
on the command line or via a custom `values.yaml` file.

- `replicaCount`: `1`
  - The number of replicas of the operator to run. Only one is active at a time,
    controlled via leader election.
- `image.repository`: `quay.io/backube/scribe`
  - The container image of the Scribe operator
- `image.pullPolicy`: `IfNotPresent`
  - The image pull policy to apply to the operator's image
- `image.tag`: (current appVersion)
  - The tag to use when retrieving the operator image. This defaults to the tag
    for the current application version associated with this chart release.
- `rclone.repository`: `quay.io/backube/scribe-mover-rclone`
  - The container image for Scribe's rclone-based data mover
- `rclone.tag`: (current appVersion)
  - The tag to use for the rclone-based data mover
- `restic.repository`: `quay.io/backube/scribe-mover-restic`
  - The container image for Scribe's restic-based data mover
- `restic.tag`: (current appVersion)
  - The tag to use for the restic-based data mover
- `rsync.repository`: `quay.io/backube/scribe-mover-rsync`
  - The container image for Scribe's rsync-based data mover
- `rsync.tag`: (current appVersion)
  - The tag to use for the rsync-based data mover
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
