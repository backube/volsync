# VolSync Custom Scorecard Tests

These tests are designed to package the end-to-end tests in /test-e2e but be able
to run them via operator-sdk scorecard tests.

More info about scorecard tests [here](https://sdk.operatorframework.io/docs/testing-operators/scorecard/custom-tests/)

For more info (this is downstream specific) for CVP running custom scorecard tests see
[here](https://docs.engineering.redhat.com/display/CVP/Operator+Verification+Pipeline+Documentation#operator-custom-scorecard-tests)

The intention is these tests will also be run by downstream CVP E2E tests.  Downstream the scorecard config.yaml
can be used to specify which e2e tests should be run against a downstream operator installation.

## Building the custom scorecard test image

Because the test image packages files in /test-e2e, the dockerfile itself is in the parent directory
[here](../Dockerfile.volsync-custom-scorecard-tests).

To build the image, use the make target:

```bash
make custom-scorecard-tests-build
```

## Custom scorecard test metadata

After adding/removing e2e tests in test-e2e, run the generateE2ETestsConfig.sh script to generate metadata
so the custom scorecard tests can target each test.  Currently each e2e test will have a separate test in the
scorecard config.yaml.

To run this and generate a config.yaml run the make target:

```bash
make custom-scorecard-tests-generate-config
```

This will re-generate the [config.yaml](config.yaml).  This file is what should be copied to the midstream
volsync operator bundle as the scorecard config.yaml.  Before copying some edits may need to be made if certain
e2e tests should/should not be run.

## Running the scorecard tests manually

### Prereqs

The scorecard tests can be pointed at any cluster.

- Setup KUBECONFIG to point to the cluster you want to run against.

- These scorecard tests do not install the operator itself, so a prerequisite is that VolSync needs to be running in
  the cluster.

- Examples below assume the tests will be run with a service account with cluster admin privileges as these e2e tests
  create/delete namespaces etc.  A service account will need to be created to run the tests. Examples below use a
  service account named `volsync-test-runner`.

### Run all e2e tests (run from the root of the volsync project)

```bash
operator-sdk scorecard ./bundle --config custom-scorecard-tests/config.yaml --selector=suite=volsync-e2e -o text --wait-time=3600s --skip-cleanup=false --service-account=volsync-test-runner 2>&1 | tee /tmp/custom-scorecard-tests.log
```

- The example above sends the resulting output to a log.
- The --selector in this case selects all tests with `suite=volsync-e2e`
- This will also run the `deploy-prereqs` step which runs the /hack/run-minio.sh script to setup minio in the cluster.
- After deploying prereqs it will run all e2e tests in parallel (1 pod gets started for each).

### Run just one specific test

```bash
operator-sdk scorecard ./bundle --config custom-scorecard-tests/config.yaml --selector=test=test_restic_with_previous.yml -o text --wait-time=300s --skip-cleanup=false --service-account=volsync-test-runner
```
