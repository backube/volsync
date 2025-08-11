=====================================
Filesystem Repository for Kopia
=====================================

.. contents:: Table of Contents
   :local:
   :depth: 2

Overview
========

The filesystem repository feature allows you to use a PersistentVolumeClaim (PVC) as a backup repository for Kopia. This provides an alternative to remote storage backends like S3, Azure, or GCS, enabling backups to local or network-attached storage with a simplified and secure API.

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

Configuration
=============

Basic Configuration
-------------------

To use a filesystem repository, specify the ``repositoryPVC`` field in your ReplicationSource:

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
       repositoryPVC: backup-storage-pvc

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
     # When repositoryPVC is set, this will be automatically set to "filesystem:///kopia/repository"
     KOPIA_REPOSITORY: "filesystem:"
     # Required: Password for repository encryption
     KOPIA_PASSWORD: "your-secure-repository-password"

.. note::
   When using ``repositoryPVC``, VolSync automatically sets ``KOPIA_REPOSITORY=filesystem:///kopia/repository``. 
   You can optionally include ``KOPIA_REPOSITORY: "filesystem:"`` in the secret for clarity, but it's not required.

API Reference
=============

RepositoryPVC Field
-------------------

The ``repositoryPVC`` field provides a simplified API for filesystem-based repositories:

**repositoryPVC** (string, optional)
   The name of the PersistentVolumeClaim to use as the backup repository.
   
   * The PVC must exist in the same namespace as the ReplicationSource
   * The PVC is mounted at ``/kopia`` within the mover pod
   * The repository is created at the fixed path ``/kopia/repository``
   * Minimum length: 1 character
   * The PVC is always mounted in read-write mode for repository operations

.. important::
   **Security Enhancement**: The repository path is fixed at ``/kopia/repository`` and cannot be configured. 
   This design choice:
   
   * Eliminates path injection vulnerabilities
   * Removes the need for path sanitization
   * Provides consistent, predictable repository locations
   * Aligns with Kopia's standard filesystem URL format

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
       # Simply reference the PVC - no path configuration needed
       repositoryPVC: local-backup-storage
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
       # NFS PVC as repository - simple and secure
       repositoryPVC: nfs-backup-pvc
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
       repositoryPVC: fast-local-storage
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

Simplified API Benefits
========================

The new ``repositoryPVC`` field replaces the previous nested ``filesystemDestination`` structure, providing:

**Improved Security**
   * Fixed repository path eliminates directory traversal vulnerabilities
   * No user-configurable paths reduce attack surface
   * Consistent security model across all deployments

**Simplified Configuration**
   * Single field instead of nested structure
   * No path configuration or validation required
   * Clearer intent with descriptive field name

**Better Consistency**
   * Aligns with other VolSync fields (``sourcePVC``, ``destinationPVC``)
   * Uses standard Kopia filesystem URL format internally
   * Same ``KOPIA_REPOSITORY`` environment variable as other backends

**Easier Maintenance**
   * Reduced complexity in implementation
   * Fewer edge cases to handle
   * Simplified troubleshooting

Migration from Remote to Filesystem
====================================

To switch from remote storage to a filesystem repository:

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

   Add the ``repositoryPVC`` field and update the secret:

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
          repositoryPVC: backup-repository

3. **Verify Operation**

   Monitor the ReplicationSource status:

   .. code-block:: bash

      kubectl get replicationsource myapp-backup -n myapp -o yaml
      kubectl logs -l app.kubernetes.io/component=volsync-kopia -n myapp

Security Considerations
=======================

Enhanced Security Design
-------------------------

The ``repositoryPVC`` implementation includes several security enhancements:

* **Fixed Path**: Repository always at ``/kopia/repository`` - no user-configurable paths
* **No Path Injection**: Eliminates directory traversal attack vectors
* **Simplified Validation**: Fewer configuration options mean fewer potential misconfigurations
* **Consistent Isolation**: Each PVC is mounted in a dedicated, isolated directory

Best Practices
--------------

1. **Access Control**: Restrict access to repository PVCs using appropriate RBAC policies
2. **Encryption**: Always use strong passwords (minimum 16 characters) for repository encryption
3. **Network Storage**: When using network storage, ensure proper network segmentation
4. **Regular Testing**: Periodically test restore operations to verify backup integrity
5. **Capacity Monitoring**: Monitor PVC usage to prevent repository corruption from full storage

Technical Implementation
-------------------------

The Kopia mover handles filesystem repositories as follows:

* The repository PVC is mounted at ``/kopia``
* The repository is initialized at ``/kopia/repository`` (fixed location)
* VolSync automatically sets ``KOPIA_REPOSITORY=filesystem:///kopia/repository``
* The same parsing logic handles filesystem URLs as other backend types (s3://, azure://, etc.)

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

   Error: repository not initialized at /kopia/repository

**Solution**: The repository will be automatically initialized on first backup. If initialization fails, check:

* PVC has sufficient space
* PVC is writable
* Password secret is properly configured

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

Inspect the repository structure:

.. code-block:: bash

   # Check if repository is properly mounted
   kubectl exec -it <kopia-pod> -n <namespace> -- ls -la /kopia
   
   # Verify repository initialization
   kubectl exec -it <kopia-pod> -n <namespace> -- ls -la /kopia/repository
   
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
* **Single Repository**: Cannot use both ``repositoryPVC`` and remote repository configuration simultaneously
* **Fixed Path**: Repository location is fixed at ``/kopia/repository`` for security
* **Single PVC**: Only one repository PVC can be specified per ReplicationSource

Compatibility Notes
--------------------

* The ``repositoryPVC`` field is available in VolSync v0.10.0 and later
* Repositories created with ``repositoryPVC`` use standard Kopia filesystem format
* Compatible with all Kopia retention and compression settings
* Works with existing Kopia tooling for repository maintenance

Related Documentation
=====================

* :doc:`index` - Main Kopia documentation
* :doc:`backup-configuration` - Detailed backup configuration options including ``repositoryPVC``
* :doc:`restore-configuration` - Restore operations from filesystem repositories
* :doc:`troubleshooting` - Comprehensive troubleshooting guide
* :doc:`backends` - Alternative remote storage backend configuration