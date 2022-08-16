#! /bin/bash

# Usage: retry.sh <command>
#   Optional env parameters:
#     - MAX_RETRIES: default=3
#     - PAUSE_SECONDS: default=1

# Based on travis_retry
# https://github.com/travis-ci/travis-build/blob/master/lib/travis/build/bash/travis_retry.bash

retries="${MAX_RETRIES:-3}"
pause="${PAUSE_SECONDS:-1}"

rc=0
while [[ $retries -gt 0 ]]; do
    if [[ $rc -ne 0 ]]; then
        echo "Command failed, waiting ${pause} seconds; retries remaining: $retries"
        sleep "$pause"
    fi
    "$@"
    rc="$?"
    if [[ $rc -eq 0 ]]; then
        break
    fi
    retries="$((retries - 1))"
done

if [[ $rc -ne 0 ]]; then
    echo "Command failed. No more retries."
fi
exit $rc
