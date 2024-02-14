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
* If updating the helm chart README.md, then make sure to update the
  description in the [csv](config/manifests/bases/volsync.clusterserviceversion.yaml).
  Then run `make bundle` to make sure the csv in the bundle dir is updated.

### Release an updated CLI plugin to krew

* After tagging the release, build the cli and updated krew manifest. This will
  create the `kubectl-volsync.tar.gz` file and update the hash in the
  [volsync.yaml](kubectl-volsync/volsync.yaml) file.:  
  `$ make krew-plugin-manifest`
* Upload the tar.gz to the github release as an "asset"
* Ensure the path in the `volsync.yaml` file is updated to point to the new
  release asset.
* Submit the updated yaml to the [krew-index
  repo](https://github.com/kubernetes-sigs/krew-index/blob/master/plugins/volsync.yaml)

## Creating a release branch

When creating a release branch use the format `release-X.Y`. For example
`release-0.6`.

* Make sure the version.mk file is updated with the version to be published
* Make sure the bundle metadata has been updated with this information (see
  steps below).  The operator csv yaml in bundle/manifests should contain the
  proper release version.  Note that this step and the previous may already be
  done prior to creating the relesae branch.
* Run the make target to update the custom scorecard config.yaml and have it
  point to the custom scorecard image tagged with the release.

  e.g. If creating a `release-0.6` branch, do the following:

  ```bash
  make custom-scorecard-tests-generate-config CUSTOM_SCORECARD_IMG_TAG=release-0.6
  ```

* Commit the generated changes in custom-scorecard-tests (particularly the
  config.yaml) in the `release-X.Y` branch.
* custom-scorecard-tests/config.yaml should also be copied over to the CICD
  midstream repo (volsync-projects) in the appropriate downstream build
  branch.

* In the main branch, edit the [periodic.yml](.github/workflows/periodic.yml)
  github actions workflow to enable periodic builds for the newly created
  `release-X.Y` branch.

## Creating/Updating the operator bundle

* Note: For packaging as an operator, the CSV file is normally auto-generated,
  but some updates may be necessary by hand.  If making updates by hand, edit
  the CSV template in
  [volsync.clusterserviceversion.yaml](config/manifests/bases/volsync.clusterserviceversion.yaml)

* If updates are made to the CSV, any of the CRDs or RBAC (any changes in
  config essentially) then run the bundle command to ensure the bundle manifests
  are up-to-date.

  `$ make bundle`

## Making a new bundle version

If developing a new version, update the [version.mk](version.mk) file and set
the proper values for:

* `VERSION`  
* `CHANNELS` - This should be the set of channels this version should be
  published to  
* `DEFAULT_CHANNEL`  
* `MIN_KUBE_VERSION`

Then run `$ make bundle` again to generate the metadata for the new version

**Note**:  A skip range may need to be specified when creating a new version.

Generally OLM will handle semver updates when updating .Y (for example
from 0.4.0 to 0.4.1 will update automatically within a channel).

However, if the version is to go into a new channel (say, a new channel
`acm-2.6`) then we will need to add replaces or skipRange to the CSV.  As an
example, say we have the following scenario:

* Channel `acm-2.5` with versions `0.4.0` and `0.4.1`  
* User is subscribed to `acm-2.5` and automatically at the head of this channel
  with version `0.4.1` installed  
* Then we make a version `0.4.2` and put this into new channel `acm-2.6`  
* If a user updates their subscription (or ACM does this for them) to channel
  `acm-2.6` then the operator will not automatically update from `0.4.1` to
  `0.4.2`

To get this upgrade to work the easiest solution atm may be to add a skipRange
to the CSV.

The following annotation can be added to the [csv](bundle/manifests/volsync.clusterserviceversion.yaml)

```yaml
olm.skipRange: '>=0.4.0 <0.4.2'
```

This will allow upgrades (even between channels) to `0.4.2` from any version
starting from `0.4.0`.  It will also upgrade directly to `0.4.2` without
installing any versions in-between.

In addition, 'replaces' can be specified in the spec to indicate a specific version
that the operator version replaces.

As an example:

```yaml
  spec:
    version: 0.4.2
    replaces: volsync.v0.4.1
```

The advantage of using 'replaces' along with the skipRange is that the version
in spec.replaces will not be removed from the update graph in the channel.
That is, with the above, a user could still install version 0.4.1.  If the csv
only had the skip range and not the replaces, then users would not be able to
install the 0.4.1 release and only the 0.4.2 release.

***Update*** This is now automated using `make bundle`.  CSV should be updated
to insert the olm.skipRange and replaces (if required).

See references:

* [operator-framework: how to update operators](https://github.com/operator-framework/operator-lifecycle-manager/blob/master/doc/design/how-to-update-operators.md)

* [olm architecture: creating an update graph](https://olm.operatorframework.io/docs/concepts/olm-architecture/operator-catalog/creating-an-update-graph/)

* [OLM doc discussing upgrades](https://docs.google.com/document/d/1X4xwBK4ECIXjaA_0DuVNuWieY9Qjbl42BMRAyIYc_HM/edit)

* [issue](https://github.com/operator-framework/olm-docs/issues/243)

### Steps to generate a bundle, bundle image, etc for testing

Most of these steps will most likely be automated downstream, but they can be
run manually to generate a bundle at a specific version as well as a bundle
image, catalog source image, etc.

Generally the metadata generated by these commands should not be committed into
this repo

Note for the following commands `VERSION`, `REPLACES_VERSION`, `OLM_SKIPRANGE`,
`CHANNELS`, `DEFAULT_CHANNEL` have
defaults set in the [version.mk](version.mk) file so if those values are
correct, no need to specify them

Note: REPLACES_VERSION should be empty for the first release in a new channel.
(For example when releasing a 0.6.0 version in a new channel, we need the skipRange
still to indicate older versions can upgrade to the new one - but REPLACES_VERSION
needs to be empty as the replaces version needs to exist in the channel or upgrades
will not work).  When REPLACES_VERSION is blank, (see make bundle in the Makefile),
the csv will be updated but the operator-sdk will automatically remove replaces
from the csv.spec when it's empty for us.

* Generate a bundle at a specific version (0.4.1 in this example) that will be
  published to channel `stable` and channel `acm-2.5`, while making the
  default channel `stable`:

  `$ make bundle VERSION=0.4.1 CHANNELS=stable,acm-2.5
  DEFAULT_CHANNEL=stable`

* To make and push a bundle image (this will be used by an operator catalog)
  that contains the bundle:

  `$ make bundle-build
  BUNDLE_IMG=quay.io/backube/volsync-operator-bundle:v0.4.1`  
  `$ make bundle-push BUNDLE_IMG=quay.io/backube/volsync-operator-bundle:v0.4.1`

* To Make and push a catalog image (this can be done for testing) that will use
  the bundle image in the previous step:

  `$ make catalog-build
  BUNDLE_IMG=quay.io/backube/volsync-operator-bundle:v0.4.1
  CATALOG_IMG=quay.io/backube/test-operator-catalog:v0.4.1`  
  `$ make catalog-push BUNDLE_IMG=quay.io/backube/volsync-operator-bundle:v0.4.1
  CATALOG_IMG=quay.io/backube/test-operator-catalog:v0.4.1`

## Updating tool/dependency versions

* Go
  * Change the version number in [operator.yml](.github/workflows/operator.yml)
  * Change the version number in [go.mod](go.mod)
    * Run `go mod tidy -go=X.Y`
  * Change the version number in [custom-scorecard-tests/go.mod](custom-scorecard-tests/go.mod)
    * Run `go mod tidy -go=X.Y`
  * Change the version number for the builder images in
    * [Dockerfile](Dockerfile)
    * [Dockerfile.volsync-custom-scorecard-tests](Dockerfile.volsync-custom-scorecard-tests)
  * Update the OpenShift CI builder images & image substitution rules
* [golangci-lint](https://github.com/golangci/golangci-lint/releases)
  * Change the version number in [Makefile](Makefile)
  * Check for added/deprecated linters and adjust [.golangci.yml](.golangci.yml)
    as necessary
* [Helm](https://github.com/helm/helm/releases)
  * Change the version number in [Makefile](Makefile)
* [Kind](https://github.com/kubernetes-sigs/kind/releases)
  * Kind version: Change the version number in
    [operator.yml](.github/workflows/operator.yml)
  * Kubernetes version used by kind:
    * [Available kubernetes
      versions](https://hub.docker.com/r/kindest/node/tags?page=1&ordering=name)
    * Update the default kube version in the
      [setup-kind-cluster.sh](./hack/setup-kind-cluster.sh) script to be the
      latest image
    * Update the matrix list for CI in
      [operator.yml](.github/workflows/operator.yml)
* [Kuttl](https://github.com/kudobuilder/kuttl/releases)
  * Change the version number in [Makefile](Makefile)
* [operator-sdk](https://github.com/operator-framework/operator-sdk/releases)
  * Change the version number in [Makefile](Makefile)
  * Follow the [upgrade
    guide](https://sdk.operatorframework.io/docs/upgrading-sdk-version/) as
    appropriate
  * Make an entry in [CHANGELOG.md](CHANGELOG.md)
  * Make an entry in [Chart.yaml](helm/volsync/Chart.yaml)
  * Run `make bundle` to propagate changes to the operator bundle files
* [Rclone](https://github.com/rclone/rclone/releases)
  * Change the version number in
    [mover-rclone/Dockerfile](mover-rclone/Dockerfile) and update GIT hash to match
  * Make an entry in [CHANGELOG.md](CHANGELOG.md)
  * Make an entry in [Chart.yaml](helm/volsync/Chart.yaml)
* [Restic](https://github.com/restic/restic/releases)
  * Change the version number in
    [mover-restic/Dockerfile](mover-restic/Dockerfile), and update GIT hash to
    match
  * Make an entry in [CHANGELOG.md](CHANGELOG.md)
  * Make an entry in [Chart.yaml](helm/volsync/Chart.yaml)
* [Syncthing](https://github.com/syncthing/syncthing/releases)
  * Change the version number in
    [mover-syncthing/Dockerfile](mover-syncthing/Dockerfile), and update GIT
    hash to match
  * Make an entry in [CHANGELOG.md](CHANGELOG.md)
  * Make an entry in [Chart.yaml](helm/volsync/Chart.yaml)
