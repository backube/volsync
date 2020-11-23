# Scribe

Scribe asynchronously replicates Kubernetes persistent volumes between clusters using either rsync
or rclone depending on the number of destinations.

[![Documentation
Status](https://readthedocs.org/projects/scribe-replication/badge/?version=latest)](https://scribe-replication.readthedocs.io/en/latest/?badge=latest)
[![Go Report
Card](https://goreportcard.com/badge/github.com/backube/scribe)](https://goreportcard.com/report/github.com/backube/scribe)
[![codecov](https://codecov.io/gh/backube/scribe/branch/master/graph/badge.svg)](https://codecov.io/gh/backube/scribe)

![Documentation](https://github.com/backube/scribe/workflows/Documentation/badge.svg)
![operator](https://github.com/backube/scribe/workflows/operator/badge.svg)
![mover-rsync](https://github.com/backube/scribe/workflows/mover-rsync/badge.svg)

:construction: :construction: :construction:
This project is in the very early stages. Stay tuned...
:construction: :construction: :construction:

## Helpful links

- [Contributing guidelines](https://github.com/backube/.github/blob/master/CONTRIBUTING.md)
- [Organization code of conduct](https://github.com/backube/.github/blob/master/CODE_OF_CONDUCT.md)

## Licensing

This project is licensed under the [GNU AGPL 3.0 License](LICENSE) with the following
exceptions:

- The files within the `api/*` directories are additionally licensed under
  Apache License 2.0. This is to permit Scribe's CustomResource types to be used
  by a wider range of software.
- Documentation is made available under the [Creative Commons
  Attribution-ShareAlike 4.0 International license (CC BY-SA
  4.0)](https://creativecommons.org/licenses/by-sa/4.0/)
