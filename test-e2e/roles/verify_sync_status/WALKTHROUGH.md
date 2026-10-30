# Issue 2188: Sync status and cleanup assertions

*2026-10-28T16:28:57Z by Showboat 0.6.1*
<!-- showboat-id: 3f52f248-5fe6-43af-80a3-fc426c18fe2b -->

## Why and what changed

Issue #2188 calls for end-to-end coverage of controller status and temporary-
resource cleanup in addition to data comparisons. The shared
`verify_sync_status` role checks settled success (condition reason, mover
result, timing, scheduled next time, and live latest image), continuous mode
for always-on/looping movers, and a negative validation-error mode. It is
called from every playbook that creates a ReplicationSource or
ReplicationDestination; only CR-free utility playbooks are excluded. A small
no-mover ReplicationSource test verifies the `Synchronizing=False/Error`
condition.

Cleanup is scoped to `volsync.backube/cleanup=<CR UID>`. Mover Job Pod
templates carry that UID label so Pods can be checked per CR. Continuous mode
distinguishes active iteration resources from stale resources and retries
through Kubernetes deletion/GC lag.

This is test coverage and test infrastructure, not a user-facing application
feature. The walkthrough shows safe syntax verification only; no live e2e or
cluster mutation was performed.

```bash
cd test-e2e && checked=0 && for f in test_*.yml; do pipenv run ansible-playbook --syntax-check "$f" >/dev/null 2>&1 || exit 1; checked=$((checked + 1)); done && printf "Ansible syntax check passed for %s playbooks.\n" "$checked"
```

```output
Ansible syntax check passed for 39 playbooks.
```

```bash
files=$(grep -l "name: verify_sync_status" test-e2e/test_*.yml | wc -l | tr -d " "); calls=$(grep -h "name: verify_sync_status" test-e2e/test_*.yml | wc -l | tr -d " "); printf "%s playbooks use the shared role in %s call sites.\n" "$files" "$calls"
```

```output
36 playbooks use the shared role in 82 call sites.
```
