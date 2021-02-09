=======================
Restic-based data mover
=======================

.. admonition:: Enhancement status

   Status: Proposed

This is a proposal to add `Restic <https://restic.readthedocs.io/en/stable/>`_
as an additional data mover within Scribe. Restic is a data backup utility that
copies the data to an object store (among other options).

While the main purpose of Scribe is to perform asynchronous data replication,
there are some use cases that are more "backup oriented" but that don't require
a full backup application (such as Velero). For example, some users may deploy
and version control their application via GitOps techniques. These users may be
looking for a simple method that allows preserving (off-cluster) snapshots of
their storage so that it can be restored if necessary.

Considerations
==============

The ReplicationSource and ReplicationDestination CRs of Scribe would correspond
to the ``backup`` and ``restore`` operations, respectively, of Restic.
Furthermore, there are repository maintenance operations that need to be
addressed. For example, Restic manages the retention of old backups (via its
``forget`` operation) as well as freeing objects that are no longer used (via
its ``prune`` operation).

While both Restic and Rclone read/write to object storage, their strengths are
significantly different. The Rclone data mover is primarily designed for
managing 1-to-many replication relationships, using the object store as an
intermediary. On each sync, Rclone updates the object bucket to be identical to
the current version of the source volume, making no attempt to preserve previous
images. This works well for replication scenarios, but it may not be desirable
when protection from accidental data deletion is desired. On the other hand,
Restic is well suited for maintaining a series of historical versions in an
efficient manner, but it is not designed for syncing data. The restore operation
makes no allowance for small delta transfers.

CRD for Restic mover
====================

In the normal case, the expected usage would be to have a ReplicationSource that
controls the periodic backups of the data. It would use the same "common volume
options" that Rsync and Rclone use to create a point-in-time image prior to
copying the data.

Backup
------

Given that in the normal case, only the ReplicationSource would be used, the
repository maintenance options should be set there.

.. code:: yaml

   ---
   apiVersion: scribe/v1alpha1
   kind: ReplicationSource
   metadata:
     name: source
     namespace: myns
   spec:
     sourcePVC: pvcname
     trigger:
       schedule: "0 * * * *"  # hourly backups
     restic:
       ### Standard volume options
       # ReplicationSourceVolumeOptions

       ### Restic-specific options
       pruneIntervalDays:  # How often to prune the repository (*int)
       repository:  # Secret name containing repository info (string)
       # Retention policy for the backups
       retain:
         last:  # Keep the last n snapshots (*int)
         hourly:  # Keep n hourly (*int)
         daily:  # Keep n daily (*int)
         weekly:  # Keep n weekly (*int)
         monthly:  # Keep n monthly (*int)
         yearly:  # Keep n yearly (*int)
         within: # Keep all within this duration (e.g., "3w15h") (*string)

The ``.spec.restic.repository`` Secret reference in the above structure refers
to a Secret in the same Namespace of the following format. The Secret's "keys"
correspond directly to the environment variables supported by Restic.

.. code:: yaml

   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: resticRepo
   type: Opaque
   data:
     # The repository url
     RESTIC_REPOSITORY: s3:s3.amazonaws.com/bucket_name
     # The repository encryption key
     RESTIC_PASSWORD: XXXXX
     # ENV vars specific to the back end
     # https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html
     AWS_ACCESS_KEY_ID: (access key)
     AWS_SECRET_ACCESS_KEY: (secret key)

Restore
-------

For now, with Scribe, the intention is to only support restoring the latest
version of the backed-up data. For retrieving previous backups (that are still
retained), Restic can be directly run against the repository, using the same
information as in the Secret, above.

Restore would be handled by the following ReplicationDestination:

.. code:: yaml

   ---
   apiVersion: scribe.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: dest-sample
   spec:
     trigger:
       schedule: "30 * * * *"
     restic:
       ### Standard volume options
       # ReplicationDestinationVolumeOptions

       ### Restic-specific options
       repository:  # Secret name containing repository info (string)

There are comparatively few configuration options for Restore.

Open issues
===========

The following items are open questions:

- Should ReplicationDestination support scheduling or should it be based on a
  single restore (i.e., it "syncs" once then never again)? This could also be
  simulated by having an arbitrarily long schedule since the 1st sync is
  immediate.

- Are Restic operations fast enough to make this viable?

  - The ``prune`` operation is documented as being rather slow

  - How long does it take to scan the storage to determine what needs to be
    backed up?

- Restic uses locks on the repository. Does the lack of concurrency present a
  problem for us? (Some can be done w/o locks... which ones?)

- What is the right way to expose ``prune``?

  - It is the method for freeing space in the repo, but may be too expensive to
    run frequently
