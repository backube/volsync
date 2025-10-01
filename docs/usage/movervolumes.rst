===========================
Additional Mover Volumes
===========================

.. toctree::
   :hidden:

For advanced users, VolSync's data movers can be run mounting additional
PVCs or secrets. These PVCs and secrets must be in the same namespace as the
corresponding ReplicationSource or ReplicationDestination.

Note: This feature is not available for the ``rsync`` mover - use the ``rsync-tls``
mover instead.

Each mover has a ``spec`` section where ``moverVolumes`` can be specified.
Each moverVolume has a ``name`` field, and the corresponding secret or PVC will be
mounted in the mover pod at ``/mnt/<name>``.

Here is an example restic ``replicationsource`` that sets a moverVolume to mount
an additional PVC.  In the example the PVC named ``repo-pvc`` will be mounted to
the mover pod at the path ``/mnt/repo``:

.. code-block:: yaml

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: source
    namespace: "test-ns"
  spec:
    sourcePVC: data-source
    trigger:
      manual: once
    restic:
      pruneIntervalDays: 1
      repository: restic-secret
      retain:
        hourly: 3
        daily: 2
        monthly: 1
      copyMethod: Snapshot
      cacheCapacity: 1Gi
      # Additional PVC to mount to the mover pod at /mnt/repo
      moverVolumes:
        - name: repo
          volumeSource:
            persistentVolumeClaim:
              claimName: repo-pvc
