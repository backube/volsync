#!/bin/bash

CUSTOM_SCORECARD_IMG=$1

if [ -z $CUSTOM_SCORECARD_IMG ]; then
  echo "Usage: generateE2ETestsConfig <custom-scorecard-image-name>"
  exit 1
fi

TESTS="$(find ../test-e2e -maxdepth 1 -type f -name 'test_*.yml' -exec basename {} \;)"
if [ $? -ne 0 ]; then
  echo "Unable to get list of e2e tests"
  exit 1
fi

echo "# E2E test list is: "
echo "$TESTS"

prereqs_patchfile="scorecard/patches/deploy-prereqs.yaml"
e2e_tests_patchfile="scorecard/patches/e2e-tests.yaml"

rm -rf ${prereqs_patchfile}
rm -rf ${e2e_tests_patchfile}

# Prereqs
cat <<EOF > ${prereqs_patchfile}
- op: add
  path: /stages/0/tests/-
  value:
    entrypoint:
    - volsync-custom-scorecard-tests
    - deploy-prereqs
    image: $CUSTOM_SCORECARD_IMG
    labels:
      suite: volsync-e2e
      test: deploy-prereqs
EOF

# E2E tests
cat <<EOF > ${e2e_tests_patchfile}
- op: add
  path: /stages/1/tests
  value:
EOF

for file in $TESTS; do
  cat <<EOF >> ${e2e_tests_patchfile}
  - entrypoint:
    - volsync-custom-scorecard-tests
    - $file
    image: $CUSTOM_SCORECARD_IMG
    labels:
      suite: volsync-e2e
      test: $file
    storage:
      spec:
        mountPath: {}
EOF
done
