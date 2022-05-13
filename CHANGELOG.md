# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Added

- Users can manually label destination Snapshot objects with
  `volsync.backube/do-not-delete` to prevent VolSync from deleting them. This
  provides a way for users to avoid having a Snapshot deleted while they are
  trying to use it. Users are then responsible for deleting the Snapshot.
- Publish Kubernetes Events to help troubleshooting

### Changed

- Operator-SDK upgraded to 1.20.0
- Rclone upgraded to 1.58.1
- Restic upgraded to 0.13.1
- Syncthing upgraded to 1.20.1

### Fixed

- CLI: Fixed bug where previously specified options couldn't be removed from
  relationship file

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

[Unreleased]: https://github.com/backube/volsync/compare/v0.4.0...HEAD
[0.4.0]: https://github.com/backube/volsync/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/backube/volsync/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/backube/volsync/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/backube/volsync/releases/tag/v0.1.0
