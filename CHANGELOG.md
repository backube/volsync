# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [unreleased]

### Added

- Restic - ReplicationSource/ReplicationDestination can now specify a CustomCA
  that is from a configmap rather than only from a secret.
- Rclone - ReplicationSource/ReplicationDestination can now specify a CustomCA
  that is contained in either a configmap or secret.
- Restic - New option to run a restic unlock before the backup in the next sync.
- Restic - Allow passing through of RCLONE_ env vars from the restic secret to
  the mover job.
- Volume Populator added for ReplicationDestinations.

### Changed

- Syncthing upgraded to v1.23.7
- Restic upgraded to v0.16.0
- Rclone upgraded to v1.63.1

## [0.7.1]

### Changed

- Modified leader election settings (LeaseDuration, RenewDeadline, RetryPeriod)
  to match OpenShift recommendations
- Syncthing upgraded to v1.23.2

### Fixed

- Updated the metrics service to use a unique pod selector (VolSync operator
  deployments only)

## [0.7.0]

### Added

- New rsync-tls data mover that will replace the existing rsync-ssh mover
- moverServiceAccount parameter in the spec to allow advanced users to specify
  their own service account to be used by mover jobs/deploys

### Changed

- VolSync now uses a single container image for the controller and all movers
- Rclone upgraded to v1.61.1
- Restic upgraded to v0.15.1
- Syncthing upgraded to v1.23.1

### Fixed

- Syncthing should ignore lost+found directory

### Security

- kube-rbac-proxy upgraded to 0.14.0
- All movers, except rsync-ssh, now run with reduced privileges by default (see docs)

## [0.6.1]

### Fixed

- set HTTP_PROXY, HTTPS_PROXY, NO_PROXY env vars on mover pod if they are set on
  the controller. Allows for cluster-wide proxy usage.

## [0.6.0]

### Added

- restic - allow passing in GOOGLE_APPLICATION_CREDENTIALS as a file

### Changed

- :warning: Breaking change :warning: - Helm chart now manages VolSync CRDs
  directly.  
  Upgrading the VolSync Helm chart from an earlier version will produce the
  following error:

  ```
  Error: UPGRADE FAILED: rendered manifests contain a resource that already exists. Unable to continue with update: CustomResourceDefinition "replicationdestinations.volsync.backube" in namespace "" exists and cannot be imported into the current release: invalid ownership metadata; label validation error: missing key "app.kubernetes.io/managed-by": must be set to "Helm"; annotation validation error: missing key "meta.helm.sh/release-name": must be set to "volsync"; annotation validation error: missing key "meta.helm.sh/release-namespace": must be set to "volsync-system"
  ```

  To fix, apply the missing labels and annotations as mentioned in the error
  message (your values may differ), then retry the upgrade:

  ```console
  $ kubectl label crd/replicationdestinations.volsync.backube app.kubernetes.io/managed-by=Helm
  customresourcedefinition.apiextensions.k8s.io/replicationdestinations.volsync.backube labeled
  $ kubectl label crd/replicationsources.volsync.backube app.kubernetes.io/managed-by=Helm
  customresourcedefinition.apiextensions.k8s.io/replicationsources.volsync.backube labeled
  $ kubectl annotate crd/replicationdestinations.volsync.backube meta.helm.sh/release-name=volsync
  customresourcedefinition.apiextensions.k8s.io/replicationdestinations.volsync.backube annotated
  $ kubectl annotate crd/replicationsources.volsync.backube meta.helm.sh/release-name=volsync
  customresourcedefinition.apiextensions.k8s.io/replicationsources.volsync.backube annotated
  $ kubectl annotate crd/replicationdestinations.volsync.backube meta.helm.sh/release-namespace=volsync-system
  customresourcedefinition.apiextensions.k8s.io/replicationdestinations.volsync.backube annotated
  $ kubectl annotate crd/replicationsources.volsync.backube meta.helm.sh/release-namespace=volsync-system
  customresourcedefinition.apiextensions.k8s.io/replicationsources.volsync.backube annotated
  ```

- VolSync privileged mover SCC installed at startup on OpenShift
- Syncthing upgraded to 1.22.1
- Updates to build with golang 1.19

### Fixed

- ReplicationSource fixes for rsync, rclone and restic to enable mounting
  ROX source PVCs as read-only

### Security

- rclone mover updated to run with reduced privileges by default
- restic mover updated to run with reduced privileges by default
- syncthing mover updated to run with reduced privileges by default
- kube-rbac-proxy upgraded to 0.13.1

## [0.5.2]

### Changed

- Updated release to build on golang 1.19 (except for the syncthing mover)
- remove deprecated io/ioutil and move to using functions in package os

## [0.5.1]

### Fixed

- Fix to Restic mover to be FIPS compatible.
- Fix to Syncthing mover to be FIPS compatible.
- Fix to Rsync mover to work with IPv6 addresses.
- Fix to node affinity to work when the node name does not match the hostname.

## [0.5.0]

### Added

- New data mover based on Syncthing for live data synchronization.
- Users can manually label destination Snapshot objects with
  `volsync.backube/do-not-delete` to prevent VolSync from deleting them. This
  provides a way for users to avoid having a Snapshot deleted while they are
  trying to use it. Users are then responsible for deleting the Snapshot.
- Publish Kubernetes Events to help troubleshooting

### Changed

- Operator-SDK upgraded to 1.22.0
- Rclone upgraded to 1.59.0
- Restic upgraded to 0.13.1
- Syncthing upgraded to 1.20.1

### Fixed

- Fix to RoleBinding created by VolSync for OCP namespace labeler.
- Fix to helm charts to remove hardcoded overwriting of pod security settings.
- Fix for node affinity (when using ReplicationSource in Direct mode) to use NodeSelector.
- Fixed log timestamps to be more readable.
- CLI: Fixed bug where previously specified options couldn't be removed from
  relationship file
- Fixed issue where a snapshot or clone created from a source PVC could
  request an incorrect size if the PVC capacity did not match the
  requested size.

### Security

- kube-rbac-proxy upgraded to 0.13.0

### Removed

- "Reconciled" condition removed from ReplicationSource and
  ReplicationDestination `.status.conditions[]` in favor of returning errors via
  the "Synchronizing" Condition.

## [0.4.0] - 2022-05-12

### Added

- Helm: Add ability to specify container images by SHA hash
- Started work on new CLI (kubectl plugin)
- Support FIPS mode on OpenShift
- Added additional field `LastSyncStartTime` to CRD status

### Changed

- Rename CopyMethod `None` to `Direct` to make it more descriptive.
- Upgrade OperatorSDK to 1.15
- Move Rclone and Rsync movers to the Mover interface
- Switch snapshot API version from `snapshot.storage.k8s.io/v1beta1` to
  `snapshot.storage.k8s.io/v1` so that VolSync remains compatible w/ Kubernetes
  1.24+
- Minimum Kubernetes version is now 1.20 due to the switch to
  `snapshot.storage.k8s.io/v1`

### Fixed

- Resources weren't always removed after each sync iteration

## [0.3.0] - 2021-08-05

### Added

- Introduced internal "Mover" interface to make adding/maintaining data movers
  more modular
- Added a Condition on the CRs to indicate whether they are synchronizing or
  idle.
- Rclone: Added unit tests

### Changed

- Renamed the project: Scribe :arrow_forward: VolSync
- CRD group has changed from `scribe.backube` to `volsync.backube`
- CRD status Conditions changed from operator-lib to the implementation in
  apimachinery

### Fixed

- Restic: Fixed error when the volume is empty

## [0.2.0] - 2021-05-26

### Added

- Support for restic backups
- Metrics for monitoring replication
- VolSync CLI (kubectl plugin)
- Support for manually triggering replication instead of via schedule

### Changed

- Move to operator-sdk 1.7.2
- Use ubi-minimal for controller base image

### Fixed

- Fix deployment on OpenShift using Helm w/ deployments not named "volsync"
- Support retries w/in rsync pod to tolerate Submariner DNS delays (globalnet)
- Custom rsync port number was being ignored
- Don't overwrite annotations on the rsync Service

## [0.1.0] - 2021-02-10

### Added

- Support for rsync & rclone replication
- Helm chart to deploy operator

[Unreleased]: https://github.com/backube/volsync/compare/release-0.7...HEAD
[0.7.1]: https://github.com/backube/volsync/compare/v0.7.0...v0.7.1
[0.7.0]: https://github.com/backube/volsync/compare/release-0.6...v0.7.0
[0.6.1]: https://github.com/backube/volsync/compare/v0.6.0...v0.6.1
[0.6.0]: https://github.com/backube/volsync/compare/release-0.5...v0.6.0
[0.5.2]: https://github.com/backube/volsync/compare/v0.5.1...v0.5.2
[0.5.1]: https://github.com/backube/volsync/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/backube/volsync/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/backube/volsync/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/backube/volsync/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/backube/volsync/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/backube/volsync/releases/tag/v0.1.0
