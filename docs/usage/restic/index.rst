===================
Restic-based backup
===================

.. toctree::
   :hidden:

   database_example

.. sidebar:: Contents

   .. contents:: Backing up using Restic
      :local:

VolSync supports taking backups of PersistentVolume data using the Restic-based
data mover. A ReplicationSource defines the backup policy (target, frequency,
and retention), while a ReplicationDestination is used for restores.

The Restic mover is different than most of VolSync's other movers because it is
not meant for synchronizing data between clusters. This mover is specifically
designed for data backup.

Specifying a repository
=======================

For both backup and restore operations, it is necessary to specify a backup
repository for Restic. The repository and connection information are defined in
a ``restic-config`` Secret.

Below is an example showing how to use a repository stored on Minio.

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: restic-config
   type: Opaque
   stringData:
     # The repository url
     RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo
     # The repository encryption key
     RESTIC_PASSWORD: my-secure-restic-password
     # ENV vars specific to the chosen back end
     # https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html
     AWS_ACCESS_KEY_ID: access
     AWS_SECRET_ACCESS_KEY: password

This Secret will be referenced for both backup (ReplicationSource) and for
restore (ReplicationDestination). The key names in this configuration Secret
directly correspond to the environment variable names supported by Restic.

.. note::
   When providing credentials for Google Cloud Storage, the
   ``GOOGLE_APPLICATION_CREDENTIALS`` key should contain the actual contents of
   the json credential file, not just the path to the file.

The path used in the ``RESTIC_REPOSITORY`` is the s3 bucket but can optionally
contain a folder name within the bucket as well.  This can be useful
if multiple PVCs are to be backed up to the same S3 bucket.

As an example one restic-config secret could use:

.. code-block:: yaml

  RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo/pvc-1-backup

While another (saved in a separate restic-config secret) could use:

.. code-block:: yaml

  RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo/pvc-2-backup

.. note::
   If backing up multiple PVCs to the same S3 bucket, the path underneath the bucket must
   be unique for each PVC.  Each PVC will be backed up with a separate ReplicationSource,
   and each should use its own separate restic-config secret

   Note also by sharing the same s3 bucket this means write access to the s3 bucket will be
   granted to different replicationsources.

.. note::
   If necessary, the repository will be automatically initialized (i.e.,
   ``restic init``) during the first backup.

Configuring backup
==================

A backup policy is defined by a ReplicationSource object that uses the Restic
replication method.

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     # The PVC to be backed up
     sourcePVC: mydata
     trigger:
       # Take a backup every 30 minutes
       schedule: "*/30 * * * *"
   restic:
     # Prune the repository (repack to free space) every 2 weeks
     pruneIntervalDays: 14
     # Name of the Secret with the connection information
     repository: restic-config
     # Retention policy for backups
     retain:
       hourly: 6
       daily: 5
       weekly: 4
       monthly: 2
       yearly: 1
     # Clone the source volume prior to taking a backup to ensure a
     # point-in-time image.
     copyMethod: Clone
     # The StorageClass to use when creating the PiT copy (same as source PVC if omitted)
     #storageClassName: my-sc-name
     # The VSC to use if the copy method is Snapshot (default if omitted)
     #volumeSnapshotClassName: my-vsc-name

Backup options
--------------

There are a number of additional configuration options not shown in the above
example. VolSync's Restic mover options closely follow those of Restic itself.

.. include:: ../inc_src_opts.rst

cacheCapacity
   This determines the size of the Restic metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the non-pruned repository metadata. The default is ``1 Gi``.
cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.
cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.
pruneIntervalDays
   This determines the number of days between running ``restic prune`` on the
   repository. The prune operation repacks the data to free space, but it can
   also generate significant I/O traffic as a part of the process. Setting this
   option allows a trade-off between storage consumption (from no longer
   referenced data) and access costs.
repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository. The repository path should
   be unique for each PV. Shared backup repositories are not currently
   supported.
retain
   This has sub-fields for ``hourly``, ``daily``, ``weekly``, ``monthly``, and
   ``yearly`` that allow setting the number of each type of backup to retain.
   There is an additional field, ``within`` that can be used to specify a time
   period during which all backups should be retained. See Restic's
   `documentation on --keep-within
   <https://restic.readthedocs.io/en/stable/060_forget.html#removing-snapshots-according-to-a-policy>`_
   for more information.

   When more than the specified number of backups are present in the repository,
   they will be removed via Restic's ``forget`` operation, and the space will be
   reclaimed during the next prune.


Performing a restore
====================

Data from a backup can be restored using the ReplicationDestination CR. In most
cases, it is desirable to perform a single restore into an empty
PersistentVolume.

For example, create a PVC to hold the restored data:

.. code-block:: yaml

   ---
   kind: PersistentVolumeClaim
   apiVersion: v1
   metadata:
     name: datavol
   spec:
   accessModes:
     - ReadWriteOnce
   resources:
     requests:
       storage: 3Gi

Restore the data into ``datavol``:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: datavol-dest
   spec:
     trigger:
       manual: restore-once
     restic:
       repository: restic-repo
       # Use an existing PVC, don't provision a new one
       destinationPVC: datavol
       copyMethod: Direct

In the above example, the data will be written directly into the new PVC since
it is specified via ``destinationPVC``, and no snapshot will be created since a
``copyMethod`` of ``Direct`` is used.

The restore operation only needs to be performed once, so instead of using a
cronspec-based schedule, a :doc:`manual trigger<../triggers>` is used. After the
restore completes, the ReplicationDestination object can be deleted.

The example, shown above, will restore the data from the most recent backup. To
restore an older version of the data, the ``previous`` and ``restoreAsOf``
fields can be used. See below for more information on their meaning.

Restore options
---------------

There are a number of additional configuration options not shown in the above
example.

.. include:: ../inc_dst_opts.rst

cacheCapacity
   This determines the size of the Restic metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the non-pruned repository metadata. The default is ``1 Gi``.
cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.
cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.
previous
   Non-negative integer which specifies an offset for how many snapshots ago we
   want to restore from. When ``restoreAsOf`` is provided, the behavior is the
   same, however the starting snapshot considered will be the first one taken
   before ``restoreAsOf``.
repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository. The repository path should
   be unique for each PV.
restoreAsOf
   An RFC-3339 timestamp which specifies an upper-limit on the snapshots that we
   should be looking through when preparing to restore. Snapshots made after
   this timestamp will not be considered. Note: though this is an RFC-3339
   timestamp, Kubernetes will only accept ones with the day and hour fields
   separated by a ``T``. E.g, ``2022-08-10T20:01:03-04:00`` will work but
   ``2022-08-10 20:01:03-04:00`` will fail.
