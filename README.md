# VolSync

> **Note**: This is a fork of [backube/volsync](https://github.com/backube/volsync). For installation of the Helm Chart from this fork, use:
> ```bash
> helm repo add volsync-fork https://perfectra1n.github.io/volsync/charts
> helm install --create-namespace -n volsync-system volsync volsync-fork/volsync
> ```
> Specify the mover image(s), for at least Kopia, [here](https://github.com/perfectra1n/volsync/blob/0532cc29596bc054060889fec8cd5bb263370e76/helm/volsync/values.yaml#L40) in the Helm chart values, and specify the image to be:
> ```
> ghcr.io/perfectra1n/volsync:latest
> ```
>
> Also, the documentation is hosted in the [GitHub Pages documentation](https://perfectra1n.github.io/volsync/) (Fork documentation mirror)

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
