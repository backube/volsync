========================
Rclone-based replication
========================

.. toctree::
   :hidden:

   database_example
   rclone-secret

.. sidebar:: Contents

   .. contents:: Rclone-based replication
      :local:

Rclone-based replication supports 1:many asynchronous replication of volumes for use
cases such as:

- High fan-out data replication from a central site to many (edge) sites

With this method, VolSync synchronizes data from a ReplicationSource to a ReplicationDestination
using `Rclone <https://rclone.org/>`_ via an intermediary object storage location like AWS S3.

----------------------------------

The Rclone method uses a "push" and "pull" model for the data replication. This requires a schedule or other
trigger on both the source and destination sides to trigger the replication iterations.

During each synchronization iteration:

- A point-in-time (PiT) copy of the source volume is created using CSI drivers. This copy will be used as the source data.
- The copy is attached to an Rclone data mover job pod which uses the contents of the ``rclone-secret`` to connect to the intermediary object storage target (e.g., AWS S3).
- The source pod uses ``rclone sync`` to copy the data to S3.
- On the destination side, a corresponding Rclone mover pod syncs the data from the intermediate object storage into a volume on the destination.
- At the conclusion of the transfer, the destination creates a snapshot copy to preserve a point-in-time copy of the incoming source data.

VolSync is configured via two CustomResources (CRs), one on the source side and
one on the destination side of the replication relationship. While there should
only be one ReplicationSource pushing data to the intermediate storage, there
may be an arbitrary number of ReplicationDestination instances syncing data from
the intermediate storage to destination clusters. This enables the model of high
fan-out data distribution.

Source configuration
=========================

An example source configuration is shown below:

.. code:: yaml

  ---
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: database-source
    namespace: source
  spec:
    # The PVC to sync
    sourcePVC: mysql-pv-claim
    trigger:
      # Synchronize every 6 minutes
      schedule: "*/6 * * * *"
    rclone:
      # The configuration section of the rclone config file to use
      rcloneConfigSection: "aws-s3-bucket"
      # The path to the object bucket
      rcloneDestPath: "volsync-test-bucket"
      # Secret holding the rclone configuration
      rcloneConfig: "rclone-secret"
      # Method used to generate the PiT copy
      copyMethod: Snapshot
      # The StorageClass to use when creating the PiT copy (same as source PVC if omitted)
      storageClassName: my-sc-name
      # The VSC to use if the copy method is Snapshot (default if omitted)
      volumeSnapshotClassName: my-vsc-name

Since the ``copyMethod`` specified above is ``Snapshot``, the Rclone data mover creates a ``VolumeSnapshot`` 
of the source pvc ``mysql-pv-claim``. Then it converts this snapshot back into a PVC.
If ``copyMethod: Clone`` were used, the temporary, point-in-time copy would be
created by cloning the source PVC to a new PVC directly. This is more efficient,
but it is not supported by all CSI drivers.

The synchronization schedule, ``.spec.trigger.schedule``, is defined by a
`cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule
very flexible. Both intervals (shown above) as well as specific times and/or
days can be specified.

Source status
-----------------
Once the ``ReplicationSource`` is deployed, VolSync updates the ``nextSyncTime`` in the ``ReplicationSource`` object.

.. code:: yaml

  ---
  apiVersion:  volsync.backube/v1alpha1
  kind:         ReplicationSource
  #  ... omitted ...
  spec:
    rclone:
      copyMethod:               Snapshot
      rcloneConfig:             rclone-secret
      rcloneConfigSection:      aws-s3-bucket
      rcloneDestPath:           volsync-test-bucket
      storageClassName:         my-sc-name
      volumeSnapshotClassName:  my-vsc-name
    sourcePVC:              mysql-pv-claim
    trigger:
      schedule:  "*/6 * * * *"
    status:
      conditions:
        lastTransitionTime:  2021-01-18T21:50:59Z
        message:               Reconcile complete
        reason:                ReconcileComplete
        status:                True
        type:                  Reconciled
      nextSyncTime:          2021-01-18T22:00:00Z


Additional source options
-------------------------

There are a number of more advanced configuration parameters that are supported
for configuring the source. All of the following options would be placed within
the ``.spec.rclone`` portion of the ReplicationSource CustomResource.

.. include:: ../inc_src_opts.rst

rcloneConfigSection
   This is used to identify the configuration section within
   ``rclone.conf`` to use.

rcloneDestPath
   This is the remote storage location in which the persistent data will
   be uploaded.

rcloneConfig
   This specifies the name of a secret to be used to retrieve the Rclone
   configuration. The :doc:`content of the Secret<./rclone-secret>` is an
   ``rclone.conf`` file.

----------------------------------

Destination configuration
=========================

An example destination configuration is shown here:

.. code:: yaml

  ---
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationDestination
  metadata:
    name: database-destination
    namespace: dest
  spec:
    trigger:
      # Every 6 minutes, offset by 3 minutes
      schedule: "3,9,15,21,27,33,39,45,51,57 * * * *"
    rclone:
      rcloneConfigSection: "aws-s3-bucket"
      rcloneDestPath: "volsync-test-bucket"
      rcloneConfig: "rclone-secret"
      copyMethod: Snapshot
      accessModes: [ReadWriteOnce]
      capacity: 10Gi
      storageClassName: my-sc
      volumeSnapshotClassName: my-vsc

Similar to the replication source, a synchronization schedule is defined
``.spec.trigger.schedule``. This indicates when persistent data should be pulled
from the remote storage location. It is important that the schedule for the
destinations are offset from that of the source to allow the source to finish
pushing updates for an iteration prior to the the destination attempting to pull
them.

In the above example, a 10 GiB RWO volume will be provisioned using the ``my-sc`` StorageClass to serve as the destination for replicated data. This volume is
used by the Rclone data mover to receive the incoming data transfers.

Since the ``copyMethod`` specified above is ``Snapshot``, a ``VolumeSnapshot`` of the incoming data
will be created at the end of each synchronization interval. It is this snapshot that
would be used to gain access to the replicated data. The name of the current ``VolumeSnapshot``
holding the latest synced data will be placed in ``.status.latestImage``.

Destination status
------------------

VolSync provides status information on the state of the replication via the
``.status`` field in the ReplicationDestination object:

.. code:: yaml

  ---
  API Version:  volsync.backube/v1alpha1
  Kind:         ReplicationDestination
  #  ... omitted ...
  Spec:
    Rclone:
      Access Modes:
        ReadWriteOnce
      Capacity:                    10Gi
      Copy Method:                 Snapshot
      Rclone Config:               rclone-secret
      Rclone Config Section:       aws-s3-bucket
      Rclone Dest Path:            volsync-test-bucket
      Storage Class Name:          my-sc
      Volume Snapshot Class Name:  my-vsc
    Status:
      Conditions:
        Last Transition Time:  2021-01-19T22:16:02Z
        Message:               Reconcile complete
        Reason:                ReconcileComplete
        Status:                True
        Type:                  Reconciled
      Last Sync Duration:      7.066022293s
      Last Sync Time:          2021-01-19T22:16:02Z
      Latest Image:
        API Group:  snapshot.storage.k8s.io
        Kind:       VolumeSnapshot
        Name:       volsync-dest-database-destination-20210119221601


In the above example,

- ``Rclone Dest Path`` indicates the intermediary storage system from where data will be
  transferred to the destination site. In the above example, the intermediary storage system is an S3 bucket.
- No errors were detected (the Reconciled condition is True).

After at least one synchronization has taken place, the following will also be
available:

- ``Last Sync Time`` contains the time of the last successful data synchronization.
- ``Latest Image`` references the object with the most recent copy of the data. If
  the copyMethod is ``Snapshot``, this will be a VolumeSnapshot object. If the
  copyMethod is ``Direct``, this will be the PVC that is used as the destination by
  VolSync.

Additional destination options
------------------------------

There are a number of more advanced configuration parameters that are supported
for configuring the destination. All of the following options would be placed
within the ``.spec.rclone`` portion of the ReplicationDestination CustomResource.

.. include:: ../inc_dst_opts.rst

rcloneConfigSection
   This is used to identify the configuration section within
   ``rclone.conf`` to use.

rcloneDestPath
   This is the remote storage location in which the persistent data will
   be downloaded.

rcloneConfig
   This specifies the secret to be used. The secret contains an ``rclone.conf``
   file with the configuration and credentials for the object target.

For a concrete example, see the :doc:`database synchronization example <database_example>`.
