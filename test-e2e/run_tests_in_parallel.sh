#! /bin/bash

# 0 is unlimited
MAX_PARALLELISM=0

SCRIPT_DIR="$(dirname "$(realpath "$0")")"
cd "$SCRIPT_DIR" || exit 1

# Tests fit the pattern test_*.xml and are in the current directory
TESTS="$(find . -maxdepth 1 -type f -name 'test_*.yml' -exec basename {} \;)"
echo "Tests found: $(echo "$TESTS" | wc -w)"

# Use xargs to run tests in parallel
# Output is logged to a file .xml -> .log
# Output is also sent directly to stdout prefixed by the test name
# shellcheck disable=SC2016
echo "$TESTS" | xargs -P"$MAX_PARALLELISM" -I{} bash -c 'set -e -o pipefail; pipenv run ansible-playbook "{}" | tee "$(basename -s .yml "{}").log" | sed -e "s/^/{}>\t/"'
TESTRC=$?

if [[ $TESTRC == 0 ]]; then
    echo; echo "Tests completed successfully"
else
    # Dump the log files so they are easier to read than the above interleaved output
    for test in $TESTS; do
        logfile="$(basename -s .yml "$test").log"
        echo; echo; echo
        echo "==================== $logfile ===================="
        cat "$logfile"
    done

    # Dump cluster state for debugging
    pipenv run ansible-playbook dump_logs.yml | tee dump_logs.log

    echo; echo "!!! TESTS FAILED !!!"
fi

# Exit w/ the return code of xargs which will be non-zero if any of the sub-commands failed
exit $TESTRC
