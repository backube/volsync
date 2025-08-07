======================
Restore Configuration
======================

.. contents:: Restore Operations and Configuration
   :local:

Data from a backup can be restored using the ReplicationDestination CR. In most
cases, it is desirable to perform a single restore into an empty
PersistentVolume.

Basic Restore Example
---------------------

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
     kopia:
       repository: kopia-config
       # Use an existing PVC, don't provision a new one
       destinationPVC: datavol
       copyMethod: Direct

In the above example, the data will be written directly into the new PVC since
it is specified via ``destinationPVC``, and no snapshot will be created since a
``copyMethod`` of ``Direct`` is used.

The restore operation only needs to be performed once, so instead of using a
cronspec-based schedule, a :doc:`manual trigger<../triggers>` is used. After the
restore completes, the ReplicationDestination object can be deleted.

Point-in-Time and Previous Snapshot Restores
--------------------------------------------

The example, shown above, will restore the data from the most recent backup. To
restore an older version of the data, the ``previous``, ``shallow`` and ``restoreAsOf``
fields can be used. See below for more information on their meaning.

**Example: Restoring from a previous snapshot**

To restore from the second-most-recent snapshot (skipping the latest backup):

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: datavol-dest-previous
   spec:
     trigger:
       manual: restore-previous
     kopia:
       repository: kopia-config
       destinationPVC: datavol
       copyMethod: Direct
       # Skip 1 snapshot to get the previous one
       previous: 1

**Example: Combining previous with restoreAsOf**

To restore from 2 snapshots before a specific point in time:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: datavol-dest-timepoint
   spec:
     trigger:
       manual: restore-timepoint
     kopia:
       repository: kopia-config
       destinationPVC: datavol
       copyMethod: Direct
       # Find snapshots before this time, then skip 2 to get the 3rd oldest
       restoreAsOf: "2024-01-15T14:30:00Z"
       previous: 2

Restore options
---------------

There are a number of additional configuration options not shown in the above
example.

.. include:: ../inc_dst_opts.rst

cacheCapacity
   This determines the size of the Kopia metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the repository metadata. The default is ``1 Gi``.

   **Cache Volume Behavior:**

   - **When specified**: A PersistentVolumeClaim is created with the specified capacity
   - **When not specified**: An EmptyDir volume is used as fallback, providing temporary cache storage

   .. important::
      With EmptyDir fallback, cache contents are lost when the pod restarts, which may
      impact performance for subsequent restore operations as the cache needs to be rebuilt.

cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.

   .. note::
      This setting only applies when ``cacheCapacity`` is specified. EmptyDir volumes
      do not use StorageClasses.

cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.

   .. note::
      This setting only applies when ``cacheCapacity`` is specified. EmptyDir volumes
      do not use access modes.

cleanupCachePVC
   This optional boolean determines if the cache PVC should be cleaned up at
   the end of the restore. Cache PVCs will always be deleted if the owning
   ReplicationDestination is removed, even if this setting is false.
   Defaults to ``false``.

customCA
   This option allows a custom certificate authority to be used when making TLS
   (https) connections to the remote repository.

   key
      This is the name of the field within the Secret that holds the CA
      certificate
   secretName
      This is the name of a Secret containing the CA certificate
   configMapName
      This is the name of a ConfigMap containing the CA certificate

repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository.

previous
   Non-negative integer that specifies how many snapshots to skip before 
   selecting one to restore from. When set to ``0`` (the default), the latest 
   snapshot is used. When set to ``1``, the second newest snapshot is used 
   (skipping 1 snapshot), and so on. This parameter can be combined with 
   ``restoreAsOf`` to skip snapshots from a specific point in time.
   
   Examples:
   
   - ``previous: 0`` or omitted: Use the latest snapshot
   - ``previous: 1``: Skip the latest snapshot, use the previous one
   - ``previous: 2``: Skip 2 snapshots, use the 3rd newest
   
   When used with ``restoreAsOf``, the behavior is the same, but counting 
   starts from the first snapshot taken before the ``restoreAsOf`` timestamp.

restoreAsOf
   An RFC-3339 timestamp which specifies an upper-limit on the snapshots that we
   should be looking through when preparing to restore. Snapshots made after
   this timestamp will not be considered. Note: though this is an RFC-3339
   timestamp, Kubernetes will only accept ones with the day and hour fields
   separated by a ``T``. E.g, ``2022-08-10T20:01:03-04:00`` will work but
   ``2022-08-10 20:01:03-04:00`` will fail.

shallow
   Non-negative integer which specifies how many recent snapshots to consider
   for restore. When ``restoreAsOf`` is provided, the behavior is the same,
   however the starting snapshot considered will be the first one taken
   before ``restoreAsOf``. This is similar to Restic's ``previous`` option
   but uses Kopia's shallow clone concept.