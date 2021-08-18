# VolSync

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
  instructions](https://volsync.readthedocs.io/en/latest/usage/index.html) for
  information on setting up replication relationships.

More detailed information on installation and usage can be found in the
[official documentation](https://volsync.readthedocs.io/).

## VolSync kubectl plugin

We're also working on a command line interface to VolSync via a kubectl plugin.
To try that out:

```console
make cli
cp bin/kubectl-volsync /usr/local/bin/
```

**NOTE:** `volsync` plugin is being actively developed. Options, flags, and
names are likely to be updated frequently. PRs and new issues are welcome!

Available commands:

```console
kubectl volsync start-replication
kubectl volsync set-replication
kubectl volsync continue-replication
kubectl volsync remove-replication
```

Try the current examples:

* [single cluster cross namespace
  example](https://volsync.readthedocs.io/en/latest/usage/rsync/db_example_cli.html)
* [multiple cluster
  example](https://volsync.readthedocs.io/en/latest/usage/rsync/db_example_cli.html)

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
