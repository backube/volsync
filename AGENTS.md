# Agent Guide

VolSync is a Kubernetes operator and CLI for asynchronous persistent-volume
replication. Follow the Go toolchain declared in `go.mod` and the CI workflow.

## Repository map and working guidance

- `api/v1alpha1/` defines the custom-resource API; `internal/controller/` and
  `cmd/` contain controller and manager code.
- `mover-*/` contains data mover implementations; `kubectl-volsync/` contains
  the CLI plugin.
- `config/`, `helm/`, and `bundle/` hold deployment, CRD, Helm, and OLM assets.
- `docs/`, `examples/`, and `test-e2e/` describe and exercise user-visible
  behavior.
- `hack/` contains developer-facing setup and check scripts; CI-only test
  harnesses live in `.ci-scripts/`.
- Before changing behavior, read the relevant implementation, callers, tests,
  and docs. Treat CRD schemas and API behavior as compatibility contracts.
- For API changes, update the API source and run `make auto-generated-files`
  (or use `make manifests`, `make generate`, and `make bundle` individually).
  Do not hand-edit generated CRDs or deepcopy code. Review generated diffs and
  keep the intended outputs.

## Build and checks

- Run `make help` to list the repository's Make targets.
- `pre-commit run -a` runs repository-wide hooks. It requires Python,
  pre-commit, Ruby (for markdownlint), and ShellCheck; hooks also include
  whitespace and YAML checks, among others.
- `make lint` runs golangci-lint across `./...`.
- `make build` builds `bin/manager` and first runs `manifests`, `generate`, and
  `lint`. It may update generated files; review those changes.
- Optional container build: `make docker-build IMG=quay.io/backube/volsync:dev`
  (requires Docker).

## Tests

- For a fast package test that needs no API server, choose a package whose test
  suite does not start envtest. For example:

  ```console
  go test ./internal/controller/errors
  ```

- `make test TEST_PACKAGES=./path/to/package` selects Ginkgo packages, but
  still runs its `bundle`, `generate`, `lint`, `envtest`, `helm-lint`, and
  `ginkgo` prerequisites.
- `make test` is the canonical unit-test command. It defaults to
  `TEST_PACKAGES=./...`; its Ginkgo options randomize tests, run in parallel,
  collect coverage, and skip `mover-restic`. Controller suites use
  controller-runtime envtest local API servers, not a live cluster. The envtest
  tools or required Kubernetes API server assets may need to be downloaded on
  first run.
- `make test-race` is `make test` with `--race` appended to the Ginkgo options,
  so it runs the same suite under the Go race detector (roughly twice as slow).
  This is what CI runs for `test-operator`, and a detected data race fails the
  job.
- Run `.ci-scripts/test-run-rustfs-storage-class.sh` to check StorageClass
  selection behavior without a cluster, and
  `.ci-scripts/test-run-rustfs-bucket-init.sh` to check bucket-init retries and
  failure diagnostics.

## Live end-to-end tests

These Ansible tests require a Kubernetes cluster with VolSync and RustFS already
installed. `make test-e2e-install` installs Pipenv and Ansible/Python
dependencies; use a Python version supported by `test-e2e/Pipfile.lock` and CI.
For an existing cluster, install VolSync and then run the RustFS fixture setup
below. For a disposable local Kind cluster, these scripts create the cluster,
install VolSync, and provision the RustFS fixtures (requires Docker, Kind,
kubectl, and Helm; the HTTPS fixture also requires OpenSSL and Python 3):

```console
./hack/setup-kind-cluster.sh
./hack/run-in-kind.sh
./hack/run-rustfs.sh
RUSTFS_NAMESPACE=rustfs-tls RUSTFS_USE_TLS=1 ./hack/run-rustfs.sh
```

The RustFS installer prefers the repository-pinned Helm at `bin/helm`; run
`make helm` to download it, or set `HELM_BIN` to another Helm 3 or 4 executable.
HTTP installs need no post-renderer. TLS installs use Helm 3's executable
post-renderer or Helm 4's temporary `postrenderer/v1` subprocess plugin.
It discovers the cluster's default StorageClass (Kind uses
`csi-hostpath-sc`); set `RUSTFS_STORAGE_CLASS` if no default is configured,
multiple defaults exist, or you want to override it. On OpenShift it leaves
UID/GID and `fsGroup` assignment to the restricted SCC while retaining explicit
security settings for RustFS and its bucket-init Job. Other clusters use UID/GID
and `fsGroup` 10001 for writable volumes.

For an existing cluster, build the CLI plugin with `make cli` before running
CLI-tagged tests; the playbooks use `bin/kubectl-volsync`. The local setup above
builds the plugin as part of `run-in-kind.sh`. Install e2e dependencies with
`make test-e2e-install`, then run all playbooks with
`MAX_PARALLELISM=5 make test-e2e` (all `test_*.yml` playbooks), or run one case:

```console
cd test-e2e && pipenv run ansible-playbook test_rclone_normal.yml
```

See `test-e2e/README.md` for test tags.

**Cluster safety:** Before any cluster-mutating command, confirm authorization
and verify the current kube context. Use only a disposable local cluster; never
use a shared or production context. In particular, `hack/setup-kind-cluster.sh`
deletes the default Kind cluster. `hack/run-rustfs.sh` creates or upgrades the
configured namespace (`RUSTFS_NAMESPACE`, default `rustfs`) and installs a
disposable S3 fixture with fixed test credentials; it does not delete
namespaces. Running it with `RUSTFS_USE_TLS=1` also creates the configured
RustFS HTTPS fixture and a self-signed CA. `hack/ensure-volsync-test-runner.sh`
creates a `ClusterRoleBinding` granting `cluster-admin`.

## Pull requests and repository bots

Commits must include a DCO `Signed-off-by: Name <email>` trailer to pass the
DCO check; use `git commit -s` and ensure it matches the contributor identity.

Repository automation includes OpenShift Cherrypick Robot, openshift-ci bot,
and openshift-merge-bot; users interact with them using slash commands in PR
comments. AI agents must never issue `/approve`, `/lgtm`, or `/ok-to-test`;
these commands are reserved for humans. Agents may use `/retest` only to rerun
CI checks that failed due to infrastructure problems or flakiness.
