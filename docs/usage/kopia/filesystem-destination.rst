=====================================
Filesystem Destination for Kopia
=====================================

.. contents:: Table of Contents
   :local:
   :depth: 2

Overview
========

The filesystem destination feature allows you to use a PersistentVolumeClaim (PVC) as a backup destination for Kopia repositories. This provides an alternative to remote storage backends like S3, enabling backups to local or network-attached storage.

Use Cases
=========

The filesystem destination is ideal for:

* **Local Backups**: Store backups on local storage for quick access and recovery
* **NFS/Network Storage**: Use existing NFS or other network storage infrastructure
* **Air-gapped Environments**: Environments without internet connectivity or cloud storage access
* **Multi-tier Backup Strategy**: Combine with remote backups for a comprehensive backup solution
* **Cost Optimization**: Reduce cloud storage costs for frequently accessed backups
* **Compliance Requirements**: Keep backups within specific geographic or infrastructure boundaries

Prerequisites
=============

Before using filesystem destinations, ensure:

1. **PVC Availability**: The destination PVC must exist in the same namespace as the ReplicationSource
2. **PVC Status**: The PVC must be bound to a PersistentVolume before the backup job starts
3. **Storage Capacity**: The PVC must have sufficient capacity for your backup data
4. **Access Mode**: The PVC must support the access mode required by your backup jobs (typically ReadWriteOnce)
5. **Password Secret**: A Kopia password must still be provided via the repository secret

Configuration
=============

Basic Configuration
-------------------

To use a filesystem destination, configure the ``filesystemDestination`` field in your ReplicationSource:

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
       # Password is still required for repository encryption
       repository: kopia-password-secret
       # Configure filesystem destination
       filesystemDestination:
         claimName: backup-storage-pvc
         path: "backups"  # Optional: defaults to "backups"
         readOnly: false  # Optional: defaults to false

Password Secret Configuration
-----------------------------

Even with filesystem destinations, you must provide a password secret for repository encryption:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-password-secret
     namespace: myapp
   type: Opaque
   stringData:
     KOPIA_PASSWORD: "your-secure-repository-password"

Note that when using filesystem destinations, you do not need to provide repository URL or cloud credentials in the secret.

API Reference
=============

FilesystemDestinationSpec Fields
---------------------------------

The ``filesystemDestination`` field accepts the following configuration:

**claimName** (string, required)
   The name of the PersistentVolumeClaim to mount as the backup destination.
   The PVC must exist in the same namespace as the ReplicationSource.

**path** (string, optional)
   The repository path within the mounted PVC. The PVC will be mounted at ``/kopia`` 
   and the repository will be created at ``/kopia/<path>``. 
   
   * Default: ``"backups"``
   * Pattern: ``^[a-zA-Z0-9][a-zA-Z0-9/_-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$``
   * Maximum length: 100 characters
   * Allowed characters: alphanumeric, hyphens, underscores, forward slashes

**readOnly** (boolean, optional)
   Specifies whether to mount the PVC in read-only mode.
   
   * Default: ``false`` (read-write mode)
   * Use ``true`` for restore operations or read-only access

Examples
========

Example 1: Local Storage Backup
--------------------------------

This example backs up application data to a local storage PVC:

.. code-block:: yaml

   # Create the destination PVC
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
       filesystemDestination:
         claimName: local-backup-storage
         path: "myapp/backups"
       retain:
         hourly: 24
         daily: 7
         weekly: 4
         monthly: 3

Example 2: NFS Backup Destination
----------------------------------

This example uses an NFS-backed PVC as the backup destination:

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
       filesystemDestination:
         claimName: nfs-backup-pvc
         path: "production/database"
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
       repository: kopia-password
       filesystemDestination:
         claimName: fast-local-storage
         path: "hourly"
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

Migration Guide
===============

Migrating from Remote to Filesystem
------------------------------------

To migrate existing backups from remote storage to filesystem:

1. **Create Destination PVC**

   .. code-block:: yaml

      apiVersion: v1
      kind: PersistentVolumeClaim
      metadata:
        name: migrated-backups
        namespace: myapp
      spec:
        accessModes:
          - ReadWriteOnce
        resources:
          requests:
            storage: 200Gi

2. **Update ReplicationSource**

   Remove the remote repository configuration from your secret and add the filesystem destination:

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
          filesystemDestination:
            claimName: migrated-backups
            path: "repository"

3. **Verify Migration**

   Monitor the ReplicationSource status to ensure successful backup operations:

   .. code-block:: bash

      kubectl get replicationsource myapp-backup -n myapp -o yaml

Security Considerations
=======================

Path Validation
---------------

The filesystem destination feature includes built-in security measures:

* **Path Sanitization**: All paths are sanitized to prevent directory traversal attacks
* **CRD Validation**: Path patterns are validated at the API level
* **Mount Isolation**: Each PVC is mounted in an isolated directory

Best Practices
--------------

1. **Access Control**: Restrict access to backup PVCs using appropriate RBAC policies
2. **Encryption**: Always use strong passwords for repository encryption
3. **Network Storage**: When using network storage, ensure proper network segmentation
4. **Regular Testing**: Periodically test restore operations to verify backup integrity

Permissions
-----------

The Kopia mover requires write access to the parent directory for repository operations:

* The PVC is mounted at ``/kopia`` (parent directory)
* The repository is created at ``/kopia/<path>`` (default: ``/kopia/backups``)
* An emptyDir volume is mounted at ``/kopia`` to provide the required write permissions

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

**Path Validation Error**

.. code-block:: text

   Error: Invalid path: contains invalid characters or patterns

**Solution**: Ensure the path only contains allowed characters (alphanumeric, hyphens, underscores, forward slashes) and doesn't exceed 100 characters.

**Permission Denied**

.. code-block:: text

   Error: unable to create repository: permission denied

**Solution**: Verify the PVC is mounted with write permissions (``readOnly: false``) and the storage supports the required operations.

**Insufficient Storage**

.. code-block:: text

   Error: no space left on device

**Solution**: Monitor PVC usage and expand capacity if needed:

.. code-block:: bash

   kubectl exec -it <kopia-pod> -- df -h /kopia
   kubectl patch pvc backup-pvc -n <namespace> -p '{"spec":{"resources":{"requests":{"storage":"200Gi"}}}}'

Debugging Commands
------------------

Check ReplicationSource status:

.. code-block:: bash

   kubectl describe replicationsource <name> -n <namespace>

View Kopia mover logs:

.. code-block:: bash

   kubectl logs -n <namespace> -l app.kubernetes.io/component=volsync-kopia

Inspect mounted volumes in the mover pod:

.. code-block:: bash

   kubectl exec -it <kopia-pod> -n <namespace> -- ls -la /kopia
   kubectl exec -it <kopia-pod> -n <namespace> -- kopia repository status

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

* **Namespace Scope**: PVC must exist in the same namespace as the ReplicationSource
* **Mount Conflicts**: Cannot use both ``filesystemDestination`` and remote repository configuration
* **Single PVC**: Only one PVC can be mounted per ReplicationSource

Future Enhancements
-------------------

Potential future improvements may include:

* Cross-namespace PVC references
* Multiple PVC support for tiered storage
* Automatic PVC provisioning
* Built-in storage metrics and monitoring

Related Documentation
=====================

* :doc:`index` - Main Kopia documentation
* :doc:`backup-configuration` - Detailed backup configuration options
* :doc:`restore-configuration` - Restore operations and recovery
* :doc:`troubleshooting` - Comprehensive troubleshooting guide
* :doc:`backends` - Remote storage backend configuration