# VolSync


## Fork Notes
**Note**: This is a fork of [backube/volsync](https://github.com/backube/volsync). 

If you need help or have any questions, join the [home-operations](https://discord.com/invite/home-operations) Discord!

For installation of the Helm Chart from this fork, use:
```bash
helm repo add volsync-fork https://perfectra1n.github.io/volsync/charts
helm install --create-namespace -n volsync-system volsync volsync-fork/volsync
```
Specify the mover image(s), for at least Kopia, [here](https://github.com/perfectra1n/volsync/blob/0532cc29596bc054060889fec8cd5bb263370e76/helm/volsync/values.yaml#L40) in the Helm chart values, and specify the image to be:
```
ghcr.io/perfectra1n/volsync:latest
```

Also, the documentation is hosted in the [GitHub Pages documentation](https://perfectra1n.github.io/volsync/) (Fork documentation mirror)

As an example, this is my Argo Helm Chart release (and please check for the latest version [here](https://github.com/perfectra1n/volsync/pkgs/container/volsync)):
```yaml
apiVersion: argoproj.io/v1alpha1
kind: Application
metadata:
  name: volsync
  namespace: argocd

spec:
  project: default
  source:
    chart: volsync
    repoURL: https://perfectra1n.github.io/volsync/charts/
    targetRevision: "0.15.4"
    helm:
      values: |
        manageCRDs: true
        metrics:
          disableAuth: true
        image:
          repository: ghcr.io/perfectra1n/volsync
          tag: "0.15.21"
          image: ""
        kopia:
          repository: ghcr.io/perfectra1n/volsync
          tag: "0.15.21"
          image: ""
        rclone:
          repository: ghcr.io/perfectra1n/volsync
          # Overrides the image tag whose default is the chart appVersion.
          tag: "0.15.1"
          image: ""
        restic:
          repository: ghcr.io/perfectra1n/volsync
          # Overrides the image tag whose default is the chart appVersion.
          tag: "0.15.1"
          image: ""
        rsync:
          repository: ghcr.io/perfectra1n/volsync
          # Overrides the image tag whose default is the chart appVersion.
          tag: "0.15.1"
          image: ""
        rsync-tls:
          repository: ghcr.io/perfectra1n/volsync
          # Overrides the image tag whose default is the chart appVersion.
          tag: "0.15.1"
          image: ""
        syncthing:
          repository: ghcr.io/perfectra1n/volsync
          # Overrides the image tag whose default is the chart appVersion.
          tag: "0.15.1"
          image: ""
```

Then a `ReplicationSource` example:
```yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationSource
metadata:
  name: homepage-kopia
  namespace: apps
spec:
  sourcePVC: homepage-icons
  trigger:
    schedule: "0 */4 * * *"
  kopia:
    copyMethod: Direct
    repository: volsync-kopia-repo
    # Note: hostname is ALWAYS just the namespace "apps" (intentional design)
    # All ReplicationSources in namespace share same hostname (multi-tenancy design)
    # Combined with unique username, creates unique identity: homepage-kopia@apps
    username: homepage-kopia  # Custom username
    retain:
      weekly: 2
      monthly: 4
    moverSecurityContext:
      runAsUser: 0
      runAsGroup: 0
      fsGroup: 0
```

a `ReplicationDestination` example (referencing the above `ReplicationSource`):
```yaml
apiVersion: volsync.backube/v1alpha1
kind: ReplicationDestination
metadata:
  name: kopia-test-restore
  namespace: test
spec:
  trigger:
    manual: restore-test
  kopia:
    sourceIdentity:
      sourceName: homepage-kopia
      sourceNamespace: apps  # Hostname is ALWAYS just the namespace (intentional)
      # The combination of username (from sourceName) + hostname (namespace) = unique identity
    destinationPVC: test-restore-data
    copyMethod: Direct
    storageClassName: "truenas-csi-iscsi"
    previous: 1
    moverSecurityContext:
      runAsUser: 0
      runAsGroup: 0
      fsGroup: 0
```

And a `repository` example:
```yaml
apiVersion: v1
kind: Secret
metadata:
  name: volsync-kopia-repo
  namespace: storage
  annotations:
    replicator.v1.mittwald.de/replicate-to: "asdf,asdf,asdf"
type: Opaque
stringData:
  # The repository url
  KOPIA_REPOSITORY: s3://kopia-volsync-backups/backups
  # The repository encryption key
  KOPIA_PASSWORD: "kopiapassword"
  AWS_ACCESS_KEY_ID: "kopia-volsync-user"
  AWS_SECRET_ACCESS_KEY: "minio-user-password"
  # For non-AWS S3 (MinIO, etc.)
  AWS_S3_ENDPOINT: s3.example.com
  KOPIA_S3_ENDPOINT: s3.example.com
  # Optional: specify region
  AWS_REGION: us-east-1

```

Again, the documentation is hosted in the [GitHub Pages documentation](https://perfectra1n.github.io/volsync/) (Fork documentation mirror)

## Overview
VolSync asynchronously replicates Kubernetes persistent volumes between clusters
using either [rsync](https://rsync.samba.org/) or [rclone](https://rclone.org/).
It also supports creating backups of persistent volumes via
[restic](https://restic.net/).

[![Documentation
Status](https://readthedocs.org/projects/volsync/badge/?version=latest)](https://volsync.readthedocs.io/en/latest/?badge=latest)
[![Go Report
Card](https://goreportcard.com/badge/github.com/backube/volsync)](https://goreportcard.com/report/github.com/backube/volsync)
[![codecov](https://codecov.io/gh/backube/volsync/branch/main/graph/badge.svg)](https://codecov.io/gh/backube/volsync)
![maturity](https://img.shields.io/static/v1?label=maturity&message=alpha&color=red)

![Documentation](https://github.com/backube/volsync/workflows/Documentation/badge.svg)
![operator](https://github.com/backube/volsync/workflows/operator/badge.svg)

## Getting started

The fastest way to get started is to install VolSync in a [kind
cluster](https://kind.sigs.k8s.io/):

* Install kind if you don't already have it:  
  `$ go install sigs.k8s.io/kind@latest`
* Use our convenient script to start a cluster, install the CSI hostpath driver,
  and the snapshot controller.  
  `$ ./hack/setup-kind-cluster.sh`
* Install the latest release via [Helm](https://helm.sh/)  
  `$ helm repo add backube https://backube.github.io/helm-charts/`  
  `$ helm install --create-namespace -n volsync-system volsync backube/volsync`
* See the [usage
  instructions](https://volsync.readthedocs.io/en/stable/usage/index.html) for
  information on setting up replication relationships.

More detailed information on installation and usage can be found in the
[official documentation](https://volsync.readthedocs.io/).

## Helpful links

* [VolSync documentation](https://volsync.readthedocs.io)
* [Changelog](CHANGELOG.md)
* [Contributing guidelines](https://github.com/backube/.github/blob/master/CONTRIBUTING.md)
* [Organization code of conduct](https://github.com/backube/.github/blob/master/CODE_OF_CONDUCT.md)

## Licensing

This project is licensed under the [GNU AGPL 3.0 License](LICENSE) with the following
exceptions:

* The files within the `api/*` directories are additionally licensed under
  Apache License 2.0. This is to permit VolSync's CustomResource types to be used
  by a wider range of software.
* Documentation is made available under the [Creative Commons
  Attribution-ShareAlike 4.0 International license (CC BY-SA
  4.0)](https://creativecommons.org/licenses/by-sa/4.0/)
