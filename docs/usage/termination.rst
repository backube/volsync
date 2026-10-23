======================================
Mover Pod termination and interruption
======================================

How VolSync stops a mover
=========================

When Kubernetes stops a mover Pod, it sends ``SIGTERM`` to the container's
entrypoint (process ID 1). The entrypoint forwards the signal to the running
mover process group, waits for the process to finish its cleanup, and exits with
the mover process's status. Kubernetes sends ``SIGKILL`` when the Pod's
termination grace period expires.

Job-based mover Pods use a ``terminationGracePeriodSeconds`` of 120 seconds.
This gives Restic up to 60 seconds for its bounded lock cleanup, with an
additional 60-second margin. The same period is used for all Job-based movers
for predictable behavior.

Data consistency after an interruption
======================================

An interrupted transfer is not necessarily a consistent copy of the source.
The behavior depends on the mover:

Restic
------

A completed backup produces an atomic, complete, consistent snapshot, which Restic
publishes only when the backup finishes. An interrupted backup does not create
a new snapshot, and the repository remains structurally valid. Data uploaded
before the interruption but not referenced by a snapshot is harmless and can be
removed by a later ``prune``. On a graceful stop, Restic releases its repository
lock as part of cleanup.

If cleanup cannot finish, a stale lock may remain. See
:doc:`the Restic mover documentation <restic/index>` for the ``unlock`` option
and recovery guidance below.

Rclone
------

Rclone ``sync`` and ``copy`` are not atomic. An interrupted operation can leave
the destination partially updated, including with deletions that were already
applied. A later sync can converge the destination toward the source, but the
destination should not be considered a consistent copy until a sync completes.

Rsync over SSH
--------------

Rsync over SSH is not atomic. A completed run transfers a consistent copy, but
an interrupted run can leave the destination partially updated. A later run
converges the destination toward the source. If the source Pod stops before it
can tell the destination to shut down, the destination Pod may continue waiting
until VolSync cleans up the operation. See the
:doc:`rsync mover documentation <rsync/index>` for details about this mover.

Rsync-TLS
---------

Rsync-TLS has the same non-atomic transfer behavior as rsync over SSH: an
interrupted run can leave the destination partially updated, and a later run
converges it toward the source. The destination Pod may wait for the transfer's
completion control file until VolSync cleans up the operation. On shutdown,
stunnel, rsync, and diskrsync helper processes are stopped. See the
:doc:`Rsync-TLS mover documentation <rsync-tls/index>` for details.

Limitations and stale Restic locks
==================================

``SIGKILL`` cannot be handled by the entrypoint. Node loss, power failure, or a
process crash can also bypass cleanup. A forced Pod deletion
(``--grace-period=0 --force``), eviction during node failure, or a slow
repository backend whose cleanup takes longer than the grace period can
therefore leave a stale Restic lock. There is currently no user-facing setting
to change the mover grace period.

If a Restic lock remains, first make sure no Restic operation is still using
the repository. Then use the Restic mover's ``unlock`` option to run
``restic unlock`` before a subsequent backup, or run ``restic unlock`` manually
against the repository. VolSync does not run ``restic unlock --remove-all``
automatically: that command can remove locks that belong to another active
client.

Interrupted rclone and rsync transfers do not provide an atomic, resumable
transaction. Allow a subsequent synchronization to complete before relying on
the destination as a complete copy. Syncthing is not covered here; it runs as a
Deployment and follows its own lifecycle.
