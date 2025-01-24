#!/bin/bash

CUSTOM_SCORECARD_IMG=$1

if [[ -z "${CUSTOM_SCORECARD_IMG}" ]]; then
  echo "Usage: generateE2ETestsConfig <custom-scorecard-image-name>"
  exit 1
fi

if ! TESTS_UNSORTED="$(find ../test-e2e -maxdepth 1 -type f -name 'test_*.yml' -exec basename {} \;)"; then
  echo "Unable to get list of e2e tests"
  exit 1
fi

TESTS=$(echo "${TESTS_UNSORTED}" | LC_ALL=C sort)

TESTS_COUNT=$(echo "${TESTS}" | wc -w)

# Group tests into 2 stages (each stage gets run sequentially but
# all tests in a stage can run in parallel)
E2E_TESTS_GROUP1=$(echo "${TESTS}" | grep -v -e role -e syncthing -e restic)
E2E_TESTS_GROUP2=$(echo "${TESTS}" | grep -e role -e syncthing -e restic)
E2E_TESTS_GROUP3="" # If we want to exclude specific tests downstream - put them in this group

E2E_TESTS_GROUP1_COUNT=$(echo "${E2E_TESTS_GROUP1}" | wc -w)
E2E_TESTS_GROUP2_COUNT=$(echo "${E2E_TESTS_GROUP2}" | wc -w)
E2E_TESTS_GROUP3_COUNT=$(echo "${E2E_TESTS_GROUP3}" | wc -w)

echo "####################"
echo "# E2E tests (${TESTS_COUNT} tests total)"
echo "## Stage 1: (${E2E_TESTS_GROUP1_COUNT} tests)"
echo "${E2E_TESTS_GROUP1}"
echo "## Stage 2: (${E2E_TESTS_GROUP2_COUNT} tests)"
echo "${E2E_TESTS_GROUP2}"
echo "## Stage 3: (upstream only, ${E2E_TESTS_GROUP3_COUNT} tests)"
echo "${E2E_TESTS_GROUP3}"
echo "####################"

E2E_TESTS_GROUP_COUNT=$((E2E_TESTS_GROUP1_COUNT + E2E_TESTS_GROUP2_COUNT + E2E_TESTS_GROUP3_COUNT))

if [[ "${E2E_TESTS_GROUP_COUNT}" -ne "${TESTS_COUNT}" ]]; then
  echo "Total tests count ${TESTS_COUNT} does not equal the number of tests in groups: ${E2E_TESTS_GROUP_COUNT}"
  echo "Some tests may have been missed."
  exit 1
fi

# Common tests that go in the base
prereqs_patchfile="scorecard/bases/patches/deploy-prereqs-stage0.yaml"
e2e_tests_patchfile1="scorecard/bases/patches/e2e-tests-stage1.yaml"
e2e_tests_patchfile2="scorecard/bases/patches/e2e-tests-stage2.yaml"

# Tests here are specific to the upstream overlay
e2e_tests_patchfile3="scorecard/overlays/upstream/patches/e2e-tests-stage3-upstreamonly.yaml"

mkdir -p "$(dirname "${prereqs_patchfile}")"
mkdir -p "$(dirname "${e2e_tests_patchfile3}")"

rm -rf "${prereqs_patchfile}"
rm -rf "${e2e_tests_patchfile1}"
rm -rf "${e2e_tests_patchfile2}"
rm -rf "${e2e_tests_patchfile3}"

# Prereqs (stage 0)
cat <<EOF > "${prereqs_patchfile}"
- op: add
  path: /stages/0/tests/-
  value:
    entrypoint:
    - volsync-custom-scorecard-tests
    - deploy-prereqs
    image: ${CUSTOM_SCORECARD_IMG}
    labels:
      suite: volsync-e2e
      test: deploy-prereqs
EOF

# E2E tests - Group of e2e to run in parallel in stage 1
cat <<EOF > "${e2e_tests_patchfile1}"
- op: add
  path: /stages/1/tests
  value:
EOF

for file in ${E2E_TESTS_GROUP1}; do
  cat <<EOF >> "${e2e_tests_patchfile1}"
  - entrypoint:
    - volsync-custom-scorecard-tests
    - ${file}
    image: ${CUSTOM_SCORECARD_IMG}
    labels:
      suite: volsync-e2e
      test: ${file}
    storage:
      spec:
        mountPath: {}
EOF
done

# E2E tests - Group of e2e to run in parallel in stage 2
cat <<EOF > "${e2e_tests_patchfile2}"
- op: add
  path: /stages/2/tests
  value:
EOF

for file in ${E2E_TESTS_GROUP2}; do
  cat <<EOF >> "${e2e_tests_patchfile2}"
  - entrypoint:
    - volsync-custom-scorecard-tests
    - ${file}
    image: ${CUSTOM_SCORECARD_IMG}
    labels:
      suite: volsync-e2e
      test: ${file}
    storage:
      spec:
        mountPath: {}
EOF
done

# E2E tests - Group of e2e to run in parallel in stage 3 - for upstream only, will be excluded from downstream config.yaml
cat <<EOF > "${e2e_tests_patchfile3}"
- op: add
  path: /stages/3/tests
  value:
EOF

for file in ${E2E_TESTS_GROUP3}; do
  cat <<EOF >> "${e2e_tests_patchfile3}"
  - entrypoint:
    - volsync-custom-scorecard-tests
    - ${file}
    image: ${CUSTOM_SCORECARD_IMG}
    labels:
      suite: volsync-e2e
      test: ${file}
    storage:
      spec:
        mountPath: {}
EOF
done

# Handle current case where group 3 has no tests
if [[ "${E2E_TESTS_GROUP3_COUNT}" -eq "0" ]]; then
  echo "    []" >> "${e2e_tests_patchfile3}"
fi
