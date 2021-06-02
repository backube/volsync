# Scribe

Scribe asynchronously replicates Kubernetes persistent volumes between clusters
using either rsync or rclone depending on the number of destinations.

[![Documentation
Status](https://readthedocs.org/projects/scribe-replication/badge/?version=latest)](https://scribe-replication.readthedocs.io/en/latest/?badge=latest)
[![Go Report
Card](https://goreportcard.com/badge/github.com/backube/scribe)](https://goreportcard.com/report/github.com/backube/scribe)
[![codecov](https://codecov.io/gh/backube/scribe/branch/master/graph/badge.svg)](https://codecov.io/gh/backube/scribe)
![maturity](https://img.shields.io/static/v1?label=maturity&message=alpha&color=red)

![Documentation](https://github.com/backube/scribe/workflows/Documentation/badge.svg)
![operator](https://github.com/backube/scribe/workflows/operator/badge.svg)

## Getting started

### Try Scribe in Kind

For a convenient script to start a `kind cluster`, try this
[script to setup a kind cluster](hack/setup-kind-cluster.sh).

### Try Scribe in a Kind, Kubernetes, or Openshift cluster

Follow the steps in the [installation
instructions](https://scribe-replication.readthedocs.io/en/latest/installation/index.html).
Here are
[useful commands to configure cluster storage classes](https://scribe-replication.readthedocs.io/en/latest/installation/index.html#configure-default-csi-storage).

## Scribe kubectl plugin

To try out Scribe with a command line interface `scribe`:

```bash
make cli
cp bin/kubectl-scribe /usr/local/bin/
```

**NOTE:** `scribe` tool is being actively developed. Options, flags,
and names are likely to be updated frequently. PRs and new issues are welcome!

Available commands:

```bash
kubectl scribe start-replication
kubectl scribe set-replication
kubectl scribe continue-replication
kubectl scribe remove-replication
```

* Try the current examples:
  * [single cluster cross namespace example](./docs/usage/rsync/db-example-cli.md)
  * [multiple cluster example](./docs/usage/rsync/multi-context-sync-cli.md)

## Helpful links

* [Scribe documentation](https://scribe-replication.readthedocs.io)
* [Changelog](CHANGELOG.md)
* [Contributing guidelines](https://github.com/backube/.github/blob/master/CONTRIBUTING.md)
* [Organization code of conduct](https://github.com/backube/.github/blob/master/CODE_OF_CONDUCT.md)

## Licensing

This project is licensed under the [GNU AGPL 3.0 License](LICENSE) with the following
exceptions:

* The files within the `api/*` directories are additionally licensed under
  Apache License 2.0. This is to permit Scribe's CustomResource types to be used
  by a wider range of software.
* Documentation is made available under the [Creative Commons
  Attribution-ShareAlike 4.0 International license (CC BY-SA
  4.0)](https://creativecommons.org/licenses/by-sa/4.0/)
