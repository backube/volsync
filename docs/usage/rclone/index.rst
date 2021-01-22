========================
Rclone-based replication
========================

.. toctree::
   :hidden:

   database_example
   rclone-secret

.. sidebar:: Contents

   .. contents:: Rclone-based replication

Rclone-based replication supports 1:many asynchronous replication of volumes for use
cases such as:

- High fan-out data replication from a central site to many (edge) sites

With this method, Scribe synchronizes data from a ReplicationSource to a ReplicationDestination
using `Rclone <https://rclone.org/>`_ using an intermediary storage system like AWS S3. 

----------------------------------

The Rclone method uses a "push" and "pull" model for the data replication. A schedule or other 
trigger is used on the source side of the relationship to trigger each replication iteration.

Following are the sequences of events happening in each iterations.

- A point-in-time (PiT) copy of the source volume is created using CSI drivers. It will be used as the source data .
- A temporary PVC is created out of the PiT copy and mounted on Rclone data mover job pod.
- The Scribe Rclone data mover then connects to the intermediary storage system (e.g. AWS S3) using configurations
  based on ``rclone-secret``. It uses ``rclone sync`` to copy source data to S3.
- At the conclusion of the transfer, the destination creates a PiT copy to preserve the incoming source data.

Scribe is configured via two CustomResources (CRs), one on the source side and
one on the destination side of the replication relationship.

Source configuration
=========================

Start by configuring the source; a minimal example is shown below:

.. code:: yaml

  ---
  apiVersion: scribe.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: database-source
    namespace: source
  spec:
    sourcePVC: mysql-pv-claim
    trigger:
      schedule: "*/6 * * * *"
    rclone:
      rcloneConfigSection: "aws-s3-bucket"
      rcloneDestPath: "scribe-test-bucket"
      rcloneConfig: "rclone-secret"
      copyMethod: Snapshot

Since the ``copyMethod`` specified above is ``Snapshot``, the Rclone data mover creates a ``VolumeSnapshot`` 
of the source pvc ``mysql-pv-claim`` using the cluster's default ``VolumeSnapshotClass``.

It then creates a temproray pvc ``scribe-src-database-source`` out of the VolumeSnapshot to transfer source
data to the intermediary storage system like AWS S3 using the configurations provided in ``rclone-secret``

Source status
-----------------
Once the ``ReplicationSource`` is deployed, Scribe updates the ``nextSyncTime`` in the ``ReplicationSource`` object.

.. code:: yaml

  ---
  apiVersion:  scribe.backube/v1alpha1
  kind:         ReplicationSource
  #  ... omitted ...
  spec:
    rclone:
      copyMethod:           Snapshot
      rcloneConfig:         rclone-secret
      rcloneConfigSection:  aws-s3-bucket
      rcloneDestPath:       scribe-test-bucket
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


In the above ``ReplicationSource`` object,

- The Rclone configuations are provided via ``rclone-secret``
- The PiT copy of the source data ``mysql-pv-claim`` will be created using cluster's default ``VolumeSnapshot``.
- ``rcloneDestPath`` indicates the location on the intermediary storage system where the source data
  will be copied
- The synchronization schedule, ``.spec.trigger.schedule``, is defined by a 
  `cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule very flexible. 
  Both intervals (shown above) as well as specific times and/or days can be specified.
- No errors were detected (the Reconciled condition is True)
- ``nextSyncTime`` indicates the time of the upcoming Rclone data mover job

----------------------------------

Destination configuration
=========================

A minimal destination configuration is shown here:

.. code:: yaml

  ---
  apiVersion: scribe.backube/v1alpha1
  kind: ReplicationDestination
  metadata:
    name: database-destination
    namespace: dest
  spec:
    rclone:
      rcloneConfigSection: "aws-s3-bucket"
      rcloneDestPath: "scribe-test-bucket"
      rcloneConfig: "rclone-secret"
      copyMethod: Snapshot
      accessModes: [ReadWriteMany]
      capacity: 10Gi


In the above example, a 10 GiB RWO volume will be provisioned using the default
``StorageClass`` to serve as the destination for replicated data. This volume is
used by the Rclone data mover to receive the incoming data transfers.

Since the ``copyMethod`` specified above is ``Snapshot``, a ``VolumeSnapshot`` of the incoming data
will be created at the end of each synchronization interval. It is this snapshot that
would be used to gain access to the replicated data. The name of the current ``VolumeSnapshot``
holding the latest synced data will be placed in ``.status.latestImage``.

Destination status
------------------

Scribe provides status information on the state of the replication via the
``.status`` field in the ReplicationDestination object:

.. code:: yaml

  ---
  API Version:  scribe.backube/v1alpha1
  Kind:         ReplicationDestination
  #  ... omitted ...
  Spec:
  Rclone:
    Access Modes:
      ReadWriteMany
    Capacity:               10Gi
    Copy Method:            Snapshot
    Rclone Config:          rclone-secret
    Rclone Config Section:  aws-s3-bucket
    Rclone Dest Path:       scribe-test-bucket
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
        Name:       scribe-dest-database-destination-20210119221601


In the above example,

- ``Rclone Dest Path`` indicates the intermediary storage system from where data will be
  transfered to the destination site. In the above example, the intermediary storage system is S3 bucket
- No errors were detected (the Reconciled condition is True)

After at least one synchronization has taken place, the following will also be
available:

- ``Last Sync Time`` contains the time of the last successful data synchronization.
- ``Latest Image`` references the object with the most recent copy of the data. If
  the copyMethod is Snapshot, this will be a VolumeSnapshot object. If the
  copyMethod is None, this will be the PVC that is used as the destination by
  Scribe.

For a concrete example, see the :doc:`database synchronization example <database_example>`.
