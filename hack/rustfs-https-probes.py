#!/usr/bin/env python3

import re
import sys


# shortcut: patches chart 1.0.0's two RustFS Deployment HTTP probes; remove when
# the chart exposes native-TLS probe configuration.
# The chart selects HTTPS only for mTLS, but this fixture uses native TLS.
EXPECTED_PROBE_REPLACEMENTS = 2
HTTP_PROBE_SCHEME = re.compile(
    r"^([ \t]*scheme:[ \t]*)HTTP(?=[ \t]*(?:#.*)?(?:\r?\n)?$)"
)

in_deployment = False
probe_replacements = 0
for line in sys.stdin:
    if line.strip() == "---":
        in_deployment = False
    elif line.strip() == "kind: Deployment":
        in_deployment = True
    if in_deployment:
        line, replacements = HTTP_PROBE_SCHEME.subn(r"\1HTTPS", line)
        probe_replacements += replacements
    sys.stdout.write(line)

if probe_replacements != EXPECTED_PROBE_REPLACEMENTS:
    print(
        "rustfs-https-probes: expected "
        f"{EXPECTED_PROBE_REPLACEMENTS} Deployment probe scheme replacements "
        f"(HTTP -> HTTPS), found {probe_replacements}; RustFS chart output changed",
        file=sys.stderr,
    )
    sys.exit(1)
