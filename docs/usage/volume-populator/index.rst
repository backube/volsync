=======================================
ReplicationDestination Volume Populator
=======================================

.. toctree::
   :hidden:

.. sidebar:: Contents

   .. contents:: ReplicationDestination Volume Populator
      :local:

When a PVC is created that directly references a ReplicationDestination object, VolSync's Volume Populator controller will automatically fill the PVC with the most recent replicated data, alleviating the need to manually specify the name of the Snapshot.

.. note::
    The VolumePopulator feature of VolSync is available with kubernetes v1.22 and
    above with the `AnyVolumeDataSource` feature gate enabled. The `AnyVolumeDataSource` feature gate is
    enabled by default as of v1.24.

When replicating or restoring a PVC via a ReplicationDestination using a VolumeSnapshot,
the end result is a VolumeSnapshot that contains the latestImage from the last successful synchronization.
Previously to create a PVC with the synchronized data, you needed to create a PVC with a dataSourceRef that points to
the VolumeSnapshot created by the ReplicationDestination.
The VolSync volume populator means you can instead point the dataSourceRef of a PVC to the VolSync
ReplicationDestination resource. VolSync will take care of finding the latestImage available in the
ReplicationDestination (or waiting for it to appear after replication completes) and then populating the PVC with
the contents of the VolumeSnapshot.


Configuring a PVC with ReplicationDestination Volume Populator
==============================================================

This example will assume you have a working ReplicationDestination setup that is replicating/synchronizing data.
The example used here uses the Rclone mover, but other movers could be used. For more information about setting up the
replication itself, refer to the docs for the mover you are interested in.

.. note::
    The ReplicationDestination used with the volume populator must use a copyMethod of `Snapshot`.

Here's an example of a ReplicationDestination.

.. code-block:: yaml
    :caption: ReplicationDestination object

    ---
    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationDestination
    metadata:
      name: rclone-replicationdestination
      namespace: dest
    spec:
      trigger:
        # Every 6 minutes, offset by 3 minutes
        schedule: "3,9,15,21,27,33,39,45,51,57 * * * *"
      rclone:
        rcloneConfigSection: "aws-s3-bucket"
        rcloneDestPath: "volsync-test-bucket/mysql-pvc-claim"
        rcloneConfig: "rclone-secret"
        copyMethod: Snapshot
        accessModes: [ReadWriteOnce]
        capacity: 10Gi
        storageClassName: my-sc
        volumeSnapshotClassName: my-vsc
    status:
      lastSyncDuration: 30.038338887s
      lastSyncTime: "2023-08-13T07:29:36Z"
      latestImage:
        apiGroup: snapshot.storage.k8s.io
        kind: VolumeSnapshot
        name: volsync-rclone-replicationdestination-dest-20230813072935

Now a PVC can be created that uses this ReplicationDestination. This PVC will be populated with the contents of the
VolumeSnapshot indicated in the ``status.latestImage`` of the ReplicationDestination.

.. code-block:: yaml
    :caption: PVC object

    ---
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      name: restored-pvc
      namespace: dest
    spec:
      accessModes: [ReadWriteOnce]
      dataSourceRef:
        kind: ReplicationDestination
        apiGroup: volsync.backube
        name: rclone-replicationdestination
      resources:
        requests:
          storage: 10Gi
      storageClassName: my-vsc

The ``.spec.dataSourceRef.kind`` must be set to ``ReplicationDestination`` and the ``.spec.dataSourceRef.apiGroup``
must be set to ``volsync.backube``.

``.spec.dataSourceRef.name`` should be the name of your ReplicationDestination.

The VolSync volume populator controller will start to populate the volume with the latest available snapshot
indicated in the ReplicationDestination at ``.status.latestImage``.  In the above example ReplicationDestination this
would be: ``volsync-rclone-replicationdestination-dest-20230813072935``.

.. note::
  If no latestImage exists yet, then the PVC will remain in pending state until the ReplicationDestination completes
  and a snapshot is available.  In this way you could create a ReplicationDestination and a PVC that uses the
  ReplicationDestination at the same time. The PVC will only start the volume population process after the
  ReplicationDestination has completed a replication and a snapshot is available (as seen in ``.status.latestImage``).

  Additionally, if the storage class used (``my-vsc`` in the example) has a ``volumeBindingMode`` of
  ``WaitForFirstConsumer``, the volume populator will need to wait until there is a consumer of the PVC before it gets
  populated. When a consumer does come along (for example a pod that wants to mount the PVC), then the volume will be
  populated at that time. At this point the VolSync volume populator controller will take the latestImage from the
  ReplicationDestination, which may have been updated if additional replications have occurred since the PVC was
  created.

Once the PVC has been populated, the status should be updated and it can be used. Here is an example of the PVC
after it has been populated.

.. code-block:: yaml
    :caption: PVC object after volume population complete

    ---
    apiVersion: v1
    kind: PersistentVolumeClaim
    metadata:
      annotations:
        pv.kubernetes.io/bind-completed: "yes"
        pv.kubernetes.io/bound-by-controller: "yes"
        volume.beta.kubernetes.io/storage-provisioner: hostpath.csi.k8s.io
        volume.kubernetes.io/storage-provisioner: hostpath.csi.k8s.io
      creationTimestamp: "2023-08-13T07:24:23Z"
      finalizers:
      - kubernetes.io/pvc-protection
      name: restored-pvc
      namespace: dest
      resourceVersion: "4520"
      uid: 55f748d8-538c-4457-b36f-ad1f956290d2
    spec:
      accessModes: [ReadWriteOnce]
      dataSource:
        kind: ReplicationDestination
        apiGroup: volsync.backube
        name: rclone-replicationdestination
      dataSourceRef:
        kind: ReplicationDestination
        apiGroup: volsync.backube
        name: rclone-replicationdestination
      resources:
        requests:
          storage: 10Gi
      storageClassName: my-vsc
      volumeMode: Filesystem
      volumeName: pvc-dec29e3a-6f21-4eb0-84b2-b9d9d875f486
    status:
      accessModes:
      - ReadWriteOnce
      capacity:
        storage: 10Gi
      phase: Bound
