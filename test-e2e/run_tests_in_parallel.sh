#! /bin/bash

# 0 is unlimited
MAX_PARALLELISM=${MAX_PARALLELISM:-0}
BATCHES=${BATCHES:-2}

SCRIPT_DIR="$(dirname "$(realpath "$0")")"
cd "$SCRIPT_DIR" || exit 1

# Tests fit the pattern test_*.xml and are in the current directory
TESTS="$(find . -maxdepth 1 -type f -name 'test_*.yml' -exec basename {} \;)"
NUM_TESTS=$(echo "$TESTS" | wc -w)

if [[ $MAX_PARALLELISM == 0 ]] && [[ $BATCHES -gt 0 ]]; then
    MAX_PARALLELISM=$(( (NUM_TESTS + BATCHES - 1) / BATCHES ))
fi

echo "Tests found: $NUM_TESTS"
echo "Number of batches: $BATCHES"
echo "Maximum parallelism: $MAX_PARALLELISM"

# Use xargs to run tests in parallel
# Output is logged to a file .xml -> .log
# Output is also sent directly to stdout prefixed by the test name
# shellcheck disable=SC2016
echo "$TESTS" | xargs -P"$MAX_PARALLELISM" -I{} bash -c 'set -e -o pipefail; pipenv run ansible-playbook "{}" | tee "$(basename -s .yml "{}").log" | sed -e "s/^/{}>\t/"'
TESTRC=$?

if [[ $TESTRC == 0 ]]; then
    echo; echo "Tests completed successfully"
else
    FAILURES=""
    # Dump the log files so they are easier to read than the above interleaved output
    for test in $TESTS; do
        logfile="$(basename -s .yml "$test").log"
        if grep -q 'failed=1' "$logfile"; then
            FAILURES="$FAILURES $test"
            echo; echo; echo
            echo "==================== $logfile ===================="
            cat "$logfile"
        fi
    done

    # Dump cluster state for debugging
    pipenv run ansible-playbook dump_logs.yml | tee dump_logs.log

    echo "Failures:$FAILURES"
    echo; echo "!!! TESTS FAILED !!!"
fi

# Exit w/ the return code of xargs which will be non-zero if any of the sub-commands failed
exit $TESTRC
