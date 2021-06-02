# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.0.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.2.0] - 2021-05-26

### Added

- Support for restic backups
- Metrics for monitoring replication
- Scribe CLI (kubectl plugin)
- Support for manually triggering replication instead of via schedule

### Changed

- Move to operator-sdk 1.7.2
- Use ubi-minimal for controller base image

### Fixed

- Fix deployment on OpenShift using Helm w/ deployments not named "scribe"
- Support retries w/in rsync pod to tolerate Submariner DNS delays (globalnet)
- Custom rsync port number was being ignored
- Don't overwrite annotations on the rsync Service

## [0.1.0] - 2021-02-10

### Added

- Support for rsync & rclone replication
- Helm chart to deploy operator

[Unreleased]: https://github.com/backube/scribe/compare/v0.2.0...HEAD
[0.2.0]: https://github.com/backube/scribe/compare/v0.1.0...v0.2.0
[0.1.0]: https://github.com/backube/scribe/releases/tag/v0.1.0
