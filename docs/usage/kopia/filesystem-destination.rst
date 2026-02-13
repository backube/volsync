=====================================
Filesystem Repository for Kopia
=====================================

.. contents:: Table of Contents
   :local:
   :depth: 2

Overview
========

The filesystem repository feature allows you to use a PersistentVolumeClaim (PVC) as a backup repository for Kopia using the generic ``moverVolumes`` pattern. This provides an alternative to remote storage backends like S3, Azure, or GCS, enabling backups to local or network-attached storage.

.. important::
   **Breaking Change**: As of VolSync v0.11.0, the Kopia-specific ``repositoryPVC`` field has been replaced with the generic ``moverVolumes`` pattern for consistency with other movers (restic, rclone, rsync-tls, syncthing).

Use Cases
=========

The filesystem repository is ideal for:

* **Local Backups**: Store backups on local storage for quick access and recovery
* **NFS/Network Storage**: Use existing NFS or other network storage infrastructure
* **Air-gapped Environments**: Environments without internet connectivity or cloud storage access
* **Multi-tier Backup Strategy**: Combine with remote backups for a comprehensive backup solution
* **Cost Optimization**: Reduce cloud storage costs for frequently accessed backups
* **Compliance Requirements**: Keep backups within specific geographic or infrastructure boundaries

Prerequisites
=============

Before using filesystem repositories, ensure:

1. **PVC Availability**: The repository PVC must exist in the same namespace as the ReplicationSource
2. **PVC Status**: The PVC must be bound to a PersistentVolume before the backup job starts
3. **Storage Capacity**: The PVC must have sufficient capacity for your backup data and repository metadata
4. **Access Mode**: The PVC must support the access mode required by your backup jobs (typically ReadWriteOnce)
5. **Password Secret**: A Kopia password must be provided via the repository secret for encryption

Migration from repositoryPVC
=============================

.. warning::
   **Breaking API Change**: The ``repositoryPVC`` field has been removed and replaced with ``moverVolumes``.

The new ``moverVolumes`` pattern provides:

- **Consistency**: Same configuration pattern across all movers (restic, rclone, rsync-tls, syncthing, kopia)
- **Flexibility**: Standard volume mounting with explicit mount paths
- **Clarity**: Clear mount point specification at ``/mnt/<mountPath>``

Quick Migration Guide
---------------------

**Old Format (repositoryPVC)**:

.. code-block:: yaml

   spec:
     kopia:
       repositoryPVC: backup-storage-pvc

**New Format (moverVolumes)**:

.. code-block:: yaml

   spec:
     kopia:
       moverVolumes:
       - mountPath: kopia-repo
         volumeSource:
           persistentVolumeClaim:
             claimName: backup-storage-pvc

Key Differences
---------------

+----------------------------+----------------------------------------+----------------------------------------+
| Aspect                     | Old (repositoryPVC)                    | New (moverVolumes)                     |
+============================+========================================+========================================+
| **Mount Location**         | ``/kopia`` (hardcoded)                 | ``/mnt/<mountPath>``                   |
| **Repository Path**        | ``/kopia/repository`` (fixed)          | ``/mnt/<mountPath>`` (first PVC)       |
| **Configuration**          | Single field                           | Volume array with explicit mount paths |
| **Multiple Volumes**       | Not supported                          | Supported (first is repository)        |
| **Consistency**            | Kopia-specific                         | Generic across all movers              |
+----------------------------+----------------------------------------+----------------------------------------+

Configuration
=============

Basic Configuration
-------------------

To use a filesystem repository, specify the ``moverVolumes`` field in your ReplicationSource:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: backup-to-pvc
     namespace: myapp
   spec:
     sourcePVC: myapp-data
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM
     kopia:
       # Reference to the repository configuration secret
       repository: kopia-password-secret
       # PVC to use as the filesystem repository
       moverVolumes:
       - mountPath: kopia-repo
         volumeSource:
           persistentVolumeClaim:
             claimName: backup-storage-pvc

Password Secret Configuration
-----------------------------

For filesystem repositories, the secret must contain the repository password for encryption:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-password-secret
     namespace: myapp
   type: Opaque
   stringData:
     # Optional: Explicitly specify filesystem repository type
     # When moverVolumes is used for the repository, you can set this to "filesystem:"
     # VolSync will automatically detect the repository path from the first moverVolume
     KOPIA_REPOSITORY: "filesystem:"
     # Required: Password for repository encryption
     KOPIA_PASSWORD: "your-secure-repository-password"

.. note::
   When using ``moverVolumes`` for a filesystem repository, VolSync automatically detects the first PVC in the list as the repository location. The repository is accessed at ``/mnt/<mountPath>`` where ``<mountPath>`` is the value specified in the first moverVolume entry.

API Reference
=============

moverVolumes Field
------------------

The ``moverVolumes`` field provides a generic, consistent API for mounting additional volumes to mover pods:

**moverVolumes** (array, optional)
   Array of volume mounts to add to the mover pod. Each volume is mounted at ``/mnt/<mountPath>``.

   For Kopia filesystem repositories:

   * The **first PVC** in the moverVolumes list is automatically detected as the Kopia repository
   * The repository path is the mount point: ``/mnt/<mountPath>``
   * If multiple PVCs are present, only the first is used as the repository (a warning is logged)
   * The PVC must exist in the same namespace as the ReplicationSource
   * PVCs are mounted in read-write mode for repository operations

**moverVolume structure**:

.. code-block:: yaml

   moverVolumes:
   - mountPath: <string>           # Required: Name used in mount path /mnt/<mountPath>
     volumeSource:                 # Required: Volume source definition
       persistentVolumeClaim:
         claimName: <string>       # Required: PVC name
       # OR other volume source types (secret, configMap, etc.)

.. important::
   **Repository Detection**: VolSync automatically uses the first PVC in ``moverVolumes`` as the repository.
   The repository path is constructed as ``filesystem:///mnt/<mountPath>`` internally.

   **Best Practice**: Use descriptive mount path names like ``kopia-repo`` or ``repository`` to clearly indicate the purpose.

Examples
========

Example 1: Local Storage Backup
--------------------------------

This example backs up application data to a local storage PVC:

.. code-block:: yaml

   # Create the repository PVC
   ---
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: local-backup-storage
     namespace: myapp
   spec:
     accessModes:
       - ReadWriteOnce
     resources:
       requests:
         storage: 100Gi
     storageClassName: local-storage

   # Create the password secret
   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-password
     namespace: myapp
   type: Opaque
   stringData:
     KOPIA_PASSWORD: "change-this-to-a-secure-password"

   # Configure the backup
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: myapp-backup
     namespace: myapp
   spec:
     sourcePVC: myapp-data
     trigger:
       schedule: "0 */6 * * *"  # Every 6 hours
     kopia:
       repository: kopia-password
       # Mount the PVC as a mover volume for the repository
       moverVolumes:
       - mountPath: kopia-repo
         volumeSource:
           persistentVolumeClaim:
             claimName: local-backup-storage
       retain:
         hourly: 24
         daily: 7
         weekly: 4
         monthly: 3

Example 2: NFS Backup Repository
---------------------------------

This example uses an NFS-backed PVC as the backup repository:

.. code-block:: yaml

   # Create NFS-backed PVC
   ---
   apiVersion: v1
   kind: PersistentVolume
   metadata:
     name: nfs-backup-pv
   spec:
     capacity:
       storage: 500Gi
     accessModes:
       - ReadWriteMany
     nfs:
       server: nfs-server.example.com
       path: /exports/backups
     persistentVolumeReclaimPolicy: Retain

   ---
   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: nfs-backup-pvc
     namespace: production
   spec:
     accessModes:
       - ReadWriteMany
     resources:
       requests:
         storage: 500Gi
     volumeName: nfs-backup-pv

   # Configure backup to NFS
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: production-backup
     namespace: production
   spec:
     sourcePVC: production-database
     trigger:
       schedule: "0 1 * * *"  # Daily at 1 AM
     kopia:
       repository: kopia-secret
       # NFS PVC as repository using moverVolumes
       moverVolumes:
       - mountPath: repository
         volumeSource:
           persistentVolumeClaim:
             claimName: nfs-backup-pvc
       compression: "zstd"
       retain:
         daily: 30
         weekly: 12
         monthly: 6

Example 3: Multi-tier Backup Strategy
--------------------------------------

This example combines filesystem and remote backups for comprehensive data protection:

.. code-block:: yaml

   # Local fast backup for quick recovery
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup-local
     namespace: myapp
   spec:
     sourcePVC: app-data
     trigger:
       schedule: "0 */4 * * *"  # Every 4 hours
     kopia:
       repository: kopia-password-local
       moverVolumes:
       - mountPath: local-repo
         volumeSource:
           persistentVolumeClaim:
             claimName: fast-local-storage
       retain:
         hourly: 24
         daily: 3

   # Remote backup for disaster recovery
   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-s3-config
     namespace: myapp
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: s3://backup-bucket/myapp
     KOPIA_PASSWORD: "secure-password"
     AWS_ACCESS_KEY_ID: "AKIAIOSFODNN7EXAMPLE"
     AWS_SECRET_ACCESS_KEY: "wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY"

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup-remote
     namespace: myapp
   spec:
     sourcePVC: app-data
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM
     kopia:
       repository: kopia-s3-config
       retain:
         daily: 30
         weekly: 12
         monthly: 12

Benefits of moverVolumes Pattern
=================================

The ``moverVolumes`` pattern provides several advantages:

**Consistency Across Movers**
   * Same configuration pattern for all movers (restic, rclone, rsync-tls, syncthing, kopia)
   * Familiar to users of other VolSync movers
   * Reduces learning curve when switching between movers

**Flexibility**
   * Support for multiple volume mounts (though only first PVC is used for repository)
   * Standard Kubernetes volume source types supported
   * Explicit mount path configuration at ``/mnt/<mountPath>``

**Clarity**
   * Descriptive mount path names (e.g., ``kopia-repo``, ``repository``)
   * Clear volume source specification
   * Predictable mount locations

**Better Integration**
   * Aligns with Kubernetes volume mounting conventions
   * Consistent with other VolSync mover implementations
   * Standard ``KOPIA_REPOSITORY`` environment variable as other backends

Switching from Remote to Filesystem Repository
===============================================

To switch from remote storage to a filesystem repository using moverVolumes:

1. **Create Repository PVC**

   .. code-block:: yaml

      apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: backup-repository
        namespace: myapp
      spec:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 200Gi

2. **Update ReplicationSource**

   Add the ``moverVolumes`` field and update the secret:

   .. code-block:: yaml

      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationSource
      metadata:
        name: myapp-backup
        namespace: myapp
      spec:
        sourcePVC: myapp-data
        trigger:
          schedule: "0 2 * * *"
        kopia:
          repository: kopia-password  # Now only contains KOPIA_PASSWORD
          moverVolumes:
          - mountPath: kopia-repo
            volumeSource:
              persistentVolumeClaim:
                claimName: backup-repository

3. **Verify Operation**

   Monitor the ReplicationSource status:

   .. code-block:: bash

      kubectl get replicationsource myapp-backup -n myapp -o yaml
      kubectl logs -l app.kubernetes.io/component=volsync-kopia -n myapp

Security Considerations
=======================

Security Design
---------------

The ``moverVolumes`` implementation provides security through:

* **Standard Volume Mounts**: Uses Kubernetes standard volume mounting patterns
* **Namespace Isolation**: PVCs must exist in the same namespace as the ReplicationSource
* **Read-Write Access**: PVCs mounted with appropriate permissions for repository operations
* **Predictable Paths**: Mount paths are at ``/mnt/<mountPath>`` - explicit and consistent

Best Practices
--------------

1. **Access Control**: Restrict access to repository PVCs using appropriate RBAC policies
2. **Encryption**: Always use strong passwords (minimum 16 characters) for repository encryption
3. **Network Storage**: When using network storage, ensure proper network segmentation
4. **Regular Testing**: Periodically test restore operations to verify backup integrity
5. **Capacity Monitoring**: Monitor PVC usage to prevent repository corruption from full storage
6. **Descriptive Names**: Use clear mount path names like ``kopia-repo`` or ``repository``

Technical Implementation
-------------------------

The Kopia mover handles filesystem repositories with moverVolumes as follows:

* PVCs in ``moverVolumes`` are mounted at ``/mnt/<mountPath>``
* The **first PVC** in the moverVolumes list is detected as the repository
* VolSync automatically sets ``KOPIA_REPOSITORY=filesystem:///mnt/<mountPath>``
* If multiple PVCs are present, only the first is used (warning logged)
* The same parsing logic handles filesystem URLs as other backend types (s3://, azure://, etc.)

.. note::
   For more information on moverVolumes, see :doc:`../movervolumes`.

Troubleshooting
===============

Common Issues
-------------

**PVC Not Found**

.. code-block:: text

   Error: PersistentVolumeClaim "backup-pvc" not found

**Solution**: Ensure the PVC exists in the same namespace as the ReplicationSource:

.. code-block:: bash

   kubectl get pvc -n <namespace>
   kubectl create -f backup-pvc.yaml -n <namespace>

**PVC Not Bound**

.. code-block:: text

   Error: PVC backup-pvc is not bound

**Solution**: Check PVC status and ensure a suitable PersistentVolume is available:

.. code-block:: bash

   kubectl describe pvc backup-pvc -n <namespace>
   kubectl get pv

**Repository Not Initialized**

.. code-block:: text

   Error: repository not initialized at /mnt/<mountPath>

**Solution**: The repository will be automatically initialized on first backup. If initialization fails, check:

* PVC has sufficient space
* PVC is writable
* Password secret is properly configured
* moverVolumes configuration is correct with valid mountPath

**Permission Denied**

.. code-block:: text

   Error: unable to create repository: permission denied

**Solution**: Verify the PVC supports write operations and has sufficient permissions:

.. code-block:: bash

   # Check PVC access modes
   kubectl get pvc <pvc-name> -n <namespace> -o jsonpath='{.spec.accessModes}'
   
   # Verify storage class supports dynamic provisioning if applicable
   kubectl get storageclass <storage-class-name>

**Insufficient Storage**

.. code-block:: text

   Error: no space left on device

**Solution**: Monitor PVC usage and expand capacity if needed:

.. code-block:: bash

   kubectl exec -it <kopia-pod> -- df -h /mnt/<mountPath>
   kubectl patch pvc backup-pvc -n <namespace> -p '{"spec":{"resources":{"requests":{"storage":"200Gi"}}}}'

Debugging Commands
------------------

Check ReplicationSource status:

.. code-block:: bash

   kubectl describe replicationsource <name> -n <namespace>

View Kopia mover logs:

.. code-block:: bash

   kubectl logs -n <namespace> -l app.kubernetes.io/component=volsync-kopia

Inspect the repository structure:

.. code-block:: bash

   # Check if repository is properly mounted (replace <mountPath> with your value)
   kubectl exec -it <kopia-pod> -n <namespace> -- ls -la /mnt/<mountPath>

   # Verify repository initialization
   kubectl exec -it <kopia-pod> -n <namespace> -- ls -la /mnt/<mountPath>

   # Check repository status
   kubectl exec -it <kopia-pod> -n <namespace> -- kopia repository status

   # View repository configuration
   kubectl exec -it <kopia-pod> -n <namespace> -- env | grep KOPIA_REPOSITORY

Performance Considerations
==========================

Storage Selection
-----------------

Choose appropriate storage based on your requirements:

* **Local SSD**: Best performance for frequent backups and quick restores
* **Network Storage (NFS)**: Good for shared access across multiple nodes
* **Object Storage**: Better for long-term retention and disaster recovery

Optimization Tips
-----------------

1. **Compression**: Use ``zstd`` compression for the best balance of speed and ratio
2. **Parallelism**: Adjust parallelism based on storage capabilities
3. **Scheduling**: Stagger backup schedules to avoid resource contention
4. **Capacity Planning**: Monitor storage usage trends and plan for growth

Limitations
===========

Current Limitations
-------------------

* **Namespace Scope**: Repository PVC must exist in the same namespace as the ReplicationSource
* **Single Repository**: The first PVC in moverVolumes is used as the repository; additional PVCs are supported but only the first is used for the repository
* **ReplicationSource Only**: moverVolumes for filesystem repositories is only supported in ReplicationSource, not ReplicationDestination (restores require explicit repository configuration in secret)
* **Repository Detection**: Only PVCs (not other volume types) are detected as potential repositories

Compatibility Notes
--------------------

* The ``moverVolumes`` pattern is available in VolSync v0.11.0 and later
* The ``repositoryPVC`` field was removed in v0.11.0 - migration to moverVolumes is required
* Repositories created with moverVolumes use standard Kopia filesystem format
* Compatible with all Kopia retention and compression settings
* Works with existing Kopia tooling for repository maintenance
* Existing repositories from repositoryPVC can be accessed with moverVolumes (just update the configuration)

Related Documentation
=====================

* :doc:`index` - Main Kopia documentation
* :doc:`backup-configuration` - Detailed backup configuration options including ``moverVolumes``
* :doc:`restore-configuration` - Restore operations from filesystem repositories
* :doc:`troubleshooting` - Comprehensive troubleshooting guide
* :doc:`backends` - Alternative remote storage backend configuration
* :doc:`../movervolumes` - Generic moverVolumes documentation for all movers