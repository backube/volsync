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

# Group tests into 2 stages (each stage gets run sequentially but
# all tests in a stage can run in parallel)
E2E_TESTS_GROUP1=$(echo "${TESTS}" | grep -v -e role -e syncthing)
E2E_TESTS_GROUP2=$(echo "${TESTS}" | grep -e role -e syncthing)

echo "####################"
echo "# E2E test list is: "
echo "## Stage 1: "
echo "${E2E_TESTS_GROUP1}"
echo "## Stage 2: "
echo "${E2E_TESTS_GROUP2}"
echo "####################"

prereqs_patchfile="scorecard/patches/deploy-prereqs-stage0.yaml"
e2e_tests_patchfile1="scorecard/patches/e2e-tests-stage1.yaml"
e2e_tests_patchfile2="scorecard/patches/e2e-tests-stage2.yaml"

rm -rf "${prereqs_patchfile}"
rm -rf "${e2e_tests_patchfile1}"
rm -rf "${e2e_tests_patchfile2}"

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
