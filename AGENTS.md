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
- `make test` is the canonical CI unit-test command used by
  `.github/workflows/operator.yml`. It defaults to `TEST_PACKAGES=./...`; its
  Ginkgo options randomize tests, run in parallel, collect coverage, and skip
  `mover-restic`. Controller suites use controller-runtime envtest local API
  servers, not a live cluster. The envtest tools or required Kubernetes API
  server assets may need to be downloaded on first run.

## Live end-to-end tests

These Ansible tests require a Kubernetes cluster with VolSync and MinIO already
installed. `make test-e2e-install` installs Pipenv and Ansible/Python
dependencies; use a Python version supported by `test-e2e/Pipfile.lock` and CI.
For an existing cluster, install VolSync and MinIO before running the tests. For
a disposable local Kind cluster, these scripts create the cluster and install
VolSync and MinIO (requires Docker, Kind, kubectl, and Helm):

```console
./hack/setup-kind-cluster.sh
./hack/run-in-kind.sh
./hack/run-minio.sh
```

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
deletes the default Kind cluster, `hack/run-minio.sh` deletes the configured
MinIO namespace (`MINIO_NAMESPACE`, default `minio`) and installs MinIO with
fixed test credentials (TLS is disabled by default), and
`hack/ensure-volsync-test-runner.sh` creates a `ClusterRoleBinding` granting
`cluster-admin`.

## Pull requests and repository bots

Commits must include a DCO `Signed-off-by: Name <email>` trailer to pass the
DCO check; use `git commit -s` and ensure it matches the contributor identity.

Repository automation includes OpenShift Cherrypick Robot, openshift-ci bot,
and openshift-merge-bot; users interact with them using slash commands in PR
comments. AI agents must never issue `/approve`, `/lgtm`, or `/ok-to-test`;
these commands are reserved for humans. Agents may use `/retest` only to rerun
CI checks that failed due to infrastructure problems or flakiness.
