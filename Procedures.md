# Checklists for various maintenance tasks

## Creating a release

* Update [CHANGELOG.md](CHANGELOG.md) with the major changes since the last
  release
* Update the helm chart template
  * In [Chart.yaml](helm/volsync/Chart.yaml), update `version`, `appVersion`,
    and `annotations.artifacthub.io/changes`
  * Also update the
    [annotation](https://artifacthub.io/docs/topics/annotations/helm/) for the
    signing key if necessary (`annotations.artifacthub.io/signKey`)
* Create a PR w/ the above and commit to `main`
* Branch to `release-X.Y`
* Tag the release: `vX.Y.Z`
* Ensure tagged container images become available on Quay for all containers

### Release the updated Helm chart

* Package the chart:  
  `$ helm package helm/volsync`
* Sign the chart (uses the [GPG Helm
  plugin](https://artifacthub.io/packages/helm-plugin/gpg/gpg)):  
  `$ helm gpg sign volsync-X.Y.Z.tgz`
* Add these files to the `backube/helm-charts` repo:  
  `cd ../helm-charts`  
  `./build-index ../volsync/volsync-X.Y.Z.tgz`
* Create a PR against that repo w/ the changes

## Updating tool/dependency versions

* Go
  * Change the version number in [operator.yml](.github/workflows/operator.yml)
  * Change the version number in [go.mod](go.mod)
  * Change the version number for the builder images in
    * [Dockerfile](Dockerfile)
    * [mover-rclone/Dockerfile](mover-rclone/Dockerfile)
    * [mover-restic/Dockerfile](mover-restic/Dockerfile)
* [golangci-lint](https://github.com/golangci/golangci-lint/releases)
  * Change the version number in [Makefile](Makefile)
  * Check for added/deprecated linters and adjust [.golangci.yml](.golangci.yml)
    as necessary
* [Helm](https://github.com/helm/helm/releases)
  * Change the version number in [Makefile](Makefile)
* [Kind](https://github.com/kubernetes-sigs/kind/releases)
  * Change the version number in [operator.yml](.github/workflows/operator.yml)
* [Kuttl](https://github.com/kudobuilder/kuttl/releases)
  * Change the version number in [Makefile](Makefile)
* [operator-sdk](https://github.com/operator-framework/operator-sdk/releases)
  * Change the version number in [Makefile](Makefile)
  * Follow the [upgrade
    guide](https://sdk.operatorframework.io/docs/upgrading-sdk-version/) as
    appropriate
* [Rclone](https://github.com/rclone/rclone/releases)
  * Change the version number in
    [mover-rclone/Dockerfile](mover-rclone/Dockerfile) and update GIT hash to match
* [Restic](https://github.com/restic/restic/releases)
  * Change the version number in
    [mover-restic/Dockerfile](mover-restic/Dockerfile), and update GIT hash to
    match
