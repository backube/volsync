# verify_sync_status

Shared assertions for VolSync e2e synchronization status, conditions, and
temporary resource cleanup. Playbooks call this role after their existing
mover readiness/log/data checks. The role owns status-condition polling and
treats an observed `Error` condition as terminal, failing promptly rather than
waiting for recovery.

## Parameters

- `namespace` (required): Namespace containing the CR.
- `cr_kind` (required): `ReplicationSource` or `ReplicationDestination`.
- `cr_name` (required): CR name.
- `mode` (optional, default `completion`):
  - `completion`: Wait for the settled `Synchronizing=False` condition with
    reason `WaitingForManual` (manual or no schedule) or `WaitingForSchedule`,
    `latestMoverStatus.result=Successful`, valid timing fields, a live
    destination latest image, and no temporary resources owned by the CR.
  - `continuous`: Wait for active `Synchronizing=True` with no `Error`
    condition. Assert successful mover timing when the mover provides it, a live
    destination latest image for ReplicationDestinations, and no leaked
    temporary resources. Syncthing is ReplicationSource-only and uses
    `expect_mover_status: false`; no latest image is checked.
  - `error`: Wait for `Synchronizing=False`, reason `Error`, and a condition
    message containing `expected_error_message`. Its cleanup check assumes a
    pre-mover validation failure (such as a missing replication method) that
    creates no cleanup-labeled resources; it is not yet a general assertion for
    cleanup after a mid-sync mover failure.
- `expected_error_message` (required for `mode: error`): Literal message
  substring.
- `expect_mover_status` (optional, default `true`): Only applies to
  `mode: continuous`. Set `false` for Syncthing, which is always-on and does not
  populate `latestMoverStatus`, `lastSyncTime`, or `lastSyncDuration`.

## Continuous synchronization exception

Untriggered Restic/Rsync/Rsync-TLS movers and Syncthing immediately start
another synchronization after cleanup, so they are never reliably idle. Those
cases use `mode: continuous` and must not assert idle/no-current-Job behavior.
Temporary PVCs, VolumeSnapshots, and mover Pods are allowed only while this CR
has a live, nonterminal, nondeleting cleanup-labeled mover Job and no
cleanup-labeled resource is deleting. Completed, failed, or deleting Jobs do not
count as active and must disappear. Cleanup labels created before the current
`status.lastSyncStartTime` are stale even when a newer Job is active, so a
current Job cannot mask leftovers from a prior iteration. Stale resources and
GC/deletion lag keep the role retrying for the cleanup budget
(`verify_sync_continuous_cleanup_retries`, 60 one-second checks) and only fail
if still present after that budget. Persistent resources (destination/cache
PVCs, Syncthing Deployment/config/PVCs/Services/Secrets) and latest images are
not temporary leaks.

Timestamp comparisons use `metadata.creationTimestamp` from the API server and
`status.lastSyncStartTime` from the controller. They assume reasonably
synchronized clocks; material clock skew could mark a current resource as stale,
which is a rare false-positive limitation.

All cleanup queries are scoped to `volsync.backube/cleanup=<CR UID>`, so tests
that share a namespace cannot see another CR's resources.

## Upgrade compatibility

The cleanup label is now part of the immutable mover Job Pod template. When an
operator with this change reconciles an existing mover Job created by an older
operator, `CreateOrUpdateDeleteOnImmutableErr` may replace/re-run that Job once
so its Pod can inherit the UID cleanup label. The Job is per-sync and the
replacement is safe/recoverable. The Pod cleanup assertion therefore requires
an operator carrying this change; older operator versions do not label mover
Pods with the CR UID.

## Idempotence check

This role does not force a metadata-only reconcile to verify status/latest-image
identity. VolSync CRs have no `status.observedGeneration`, so there is no
reliable signal that a metadata-only reconcile completed; proving it would need
a fixed sleep, which adds wall time and can be flaky. Existing controller tests
cover reconcile idempotence without a live sync.
