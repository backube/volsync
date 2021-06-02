#! /bin/bash
# vim: set ts=4 sw=4 et :

# Usage: pre-commit.sh [--require-all]
#   --require-all  Fail instead of warn if a checker is not found

set -e

# Run checks from root of the repo
scriptdir="$(dirname "$(realpath "$0")")"
cd "$scriptdir/.."

# run_check <file_regex> <checker_exe> [optional args to checker...]
function run_check() {
    regex="$1"
    shift
    exe="$1"
    shift

    if [ -x "$(command -v "$exe")" ]; then
        echo "=====  $exe  ====="
        find . \
            -path ./vendor -prune -o \
            -path ./.venv -prune -o \
            -path ./testbin -prune -o \
            -regextype egrep -iregex "$regex" -print0 | \
            xargs -0rt "$exe" "$@"
        echo
        echo
    elif [ "$all_required" -eq 0 ]; then
        echo "Warning: $exe not found... skipping some tests."
    else
        echo "FAILED: All checks required, but $exe not found!"
        exit 1
    fi
}

all_required=0
if [ "$1" == "--require-all" ]; then
    all_required=1
fi

# Install via: gem install asciidoctor
run_check '.*\.adoc' asciidoctor -o /dev/null -v --failure-level WARN

# markdownlint: https://github.com/markdownlint/markdownlint
# https://github.com/markdownlint/markdownlint/blob/master/docs/RULES.md
# Install via: gem install mdl
run_check '.*\.md' mdl --style "${scriptdir}/mdl-style.rb"

# Install via: dnf install shellcheck
run_check '.*\.(ba)?sh' shellcheck

# Install via: pip install yamllint
run_check '.*\.ya?ml' yamllint -s -c "${scriptdir}/yamlconfig.yaml"

# CRDs in the Helm chart must match generated CRDs
diff -qr config/crd/bases helm/scribe/crds

echo "ALL OK."
