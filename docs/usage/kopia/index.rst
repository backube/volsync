===================
Kopia-based backup
===================

.. toctree::
   :hidden:

   database_example

.. sidebar:: Contents

   .. contents:: Backing up using Kopia
      :local:

VolSync supports taking backups of PersistentVolume data using the Kopia-based
data mover. A ReplicationSource defines the backup policy (target, frequency,
and retention), while a ReplicationDestination is used for restores.

The Kopia mover is different than most of VolSync's other movers because it is
not meant for synchronizing data between clusters. This mover is specifically
designed for data backup with advanced features like compression, deduplication,
and concurrent access.

Kopia vs. Restic
=================

While both Kopia and Restic are backup tools supported by VolSync, Kopia offers
several advantages:

**Performance**: Kopia typically provides faster backup and restore operations
due to its efficient chunking algorithm and support for parallel uploads.

**Compression**: Kopia supports multiple compression algorithms (zstd, gzip, s2)
with zstd providing better compression ratios and speed compared to Restic's options.

**Concurrent Access**: Kopia safely supports multiple clients writing to the same
repository simultaneously, while Restic requires careful coordination to avoid
conflicts.

**Modern Architecture**: Kopia uses a more modern content-addressable storage
design that enables features like shallow clones and efficient incremental backups.

**Actions/Hooks**: Kopia provides built-in support for pre and post snapshot
actions, making it easier to ensure data consistency for applications like databases.

**Maintenance**: Kopia's maintenance operations (equivalent to Restic's prune)
are more efficient and can run concurrently with backups.

Specifying a repository
=======================

For both backup and restore operations, it is necessary to specify a backup
repository for Kopia. The repository and connection information are defined in
a ``kopia-config`` Secret.

Below is an example showing how to use a repository stored on an S3-compatible backend (Minio).

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     # The repository url
     KOPIA_REPOSITORY: s3://backup-bucket/my-backups
     # The repository encryption password
     KOPIA_PASSWORD: my-secure-kopia-password
     # S3 credentials
     AWS_ACCESS_KEY_ID: access
     AWS_SECRET_ACCESS_KEY: password
     # S3 endpoint (required for non-AWS S3)
     AWS_S3_ENDPOINT: http://minio.minio.svc.cluster.local:9000

This Secret will be referenced for both backup (ReplicationSource) and for
restore (ReplicationDestination). The key names in this configuration Secret
directly correspond to the environment variable names supported by Kopia.

.. note::
   When providing credentials for Google Cloud Storage, the
   ``GOOGLE_APPLICATION_CREDENTIALS`` key should contain the actual contents of
   the json credential file, not just the path to the file.

The path used in the ``KOPIA_REPOSITORY`` is the s3 bucket but can optionally
contain a folder name within the bucket as well. This can be useful
if multiple PVCs are to be backed up to the same S3 bucket.

As an example one kopia-config secret could use:

.. code-block:: yaml

  KOPIA_REPOSITORY: s3://backup-bucket/pvc-1-backup

While another (saved in a separate kopia-config secret) could use:

.. code-block:: yaml

  KOPIA_REPOSITORY: s3://backup-bucket/pvc-2-backup

.. note::
   Unlike some other backup tools, Kopia supports multiple clients writing to
   the same repository path safely. However, for organizational purposes and
   test isolation, it's still recommended to use separate paths for different PVCs.

.. note::
   If necessary, the repository will be automatically initialized (i.e.,
   ``kopia repository create``) during the first backup.

Supported backends
------------------

Kopia supports various storage backends with their respective configuration formats:

S3-compatible storage (AWS S3, MinIO, etc.)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: s3://my-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     # For non-AWS S3 (MinIO, etc.)
     AWS_S3_ENDPOINT: http://minio.example.com:9000
     # Optional: specify region
     AWS_REGION: us-west-2

Filesystem storage
~~~~~~~~~~~~~~~~~~

For local or network-attached storage:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: filesystem:///mnt/backups
     KOPIA_PASSWORD: my-secure-password

Google Cloud Storage
~~~~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: gcs://my-gcs-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     # Service account credentials (JSON content, not file path)
     GOOGLE_APPLICATION_CREDENTIALS: |
       {
         "type": "service_account",
         "project_id": "my-project",
         "private_key_id": "key-id",
         "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
         "client_email": "backup-service@my-project.iam.gserviceaccount.com",
         "client_id": "123456789",
         "auth_uri": "https://accounts.google.com/o/oauth2/auth",
         "token_uri": "https://oauth2.googleapis.com/token"
       }

Azure Blob Storage
~~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: azure://container/backups
     KOPIA_PASSWORD: my-secure-password
     AZURE_STORAGE_ACCOUNT: mystorageaccount
     AZURE_STORAGE_KEY: storage-key-here
     # Alternative: using SAS token
     # AZURE_STORAGE_SAS_TOKEN: sv=2020-08-04&ss=bfqt&srt=sco&sp=rwdlacupx&se=2021-01-01T00:00:00Z&st=2020-01-01T00:00:00Z&spr=https,http&sig=signature

Configuring backup
==================

A backup policy is defined by a ReplicationSource object that uses the Kopia
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
     kopia:
       # Run maintenance (garbage collection, etc.) every 7 days
       maintenanceIntervalDays: 7
       # Name of the Secret with the connection information
       repository: kopia-config
       # Retention policy for backups
       retain:
         hourly: 6
         daily: 7
         weekly: 4
         monthly: 6
         yearly: 2
       # Compression algorithm (zstd, gzip, s2, none)
       compression: zstd
       # Number of parallel upload streams
       parallelism: 2
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
example. VolSync's Kopia mover options closely follow those of Kopia itself.

.. include:: ../inc_src_opts.rst

actions
   This allows you to define pre and post snapshot actions (hooks) that will
   be executed before and after taking snapshots. This can be used to ensure
   data consistency by running database flush commands, application quiesce
   operations, etc.

   beforeSnapshot
      Command to run before taking a snapshot
   afterSnapshot
      Command to run after taking a snapshot

cacheCapacity
   This determines the size of the Kopia metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the repository metadata. The default is ``1 Gi``.
cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.
cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.
compression
   This specifies the compression algorithm to use. Options are:
   
   * ``zstd`` - Zstandard compression (recommended, default)
   * ``gzip`` - Gzip compression
   * ``s2`` - S2 compression (fast)
   * ``none`` - No compression

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

maintenanceIntervalDays
   This determines the number of days between running maintenance operations
   on the repository. Maintenance includes garbage collection, compaction,
   and other housekeeping tasks. Setting this option allows a trade-off
   between storage efficiency and access costs.
parallelism
   This specifies the number of parallel upload streams to use when backing up
   data. Higher values can improve performance on fast networks but may increase
   memory usage. The default is ``1``.
repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository.
retain
   This has sub-fields for ``hourly``, ``daily``, ``weekly``, ``monthly``, and
   ``yearly`` that allow setting the number of each type of backup to retain.

   When more than the specified number of backups are present in the repository,
   they will be removed during the next maintenance operation.

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

The example, shown above, will restore the data from the most recent backup. To
restore an older version of the data, the ``shallow`` and ``restoreAsOf``
fields can be used. See below for more information on their meaning.

Restore options
---------------

There are a number of additional configuration options not shown in the above
example.

.. include:: ../inc_dst_opts.rst

cacheCapacity
   This determines the size of the Kopia metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the repository metadata. The default is ``1 Gi``.
cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.
cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.
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

Using a custom certificate authority
====================================

Normally, Kopia will use a default set of certificates to verify the validity
of remote repositories when making https connections. However, users that deploy
with a self-signed certificate will need to provide their CA's certificate via
the ``customCA`` option.

The custom CA certificate needs to be provided in a Secret or ConfigMap to
VolSync. For example, if the CA certificate is a file in the current directory
named ``ca.crt``, it can be loaded as a Secret or a ConfigMap.

Example using a customCA loaded as a secret:

.. code-block:: console

   $ kubectl create secret generic tls-secret --from-file=ca.crt=./ca.crt
   secret/tls-secret created

   $ kubectl describe secret/tls-secret
   Name:         tls-secret
   Namespace:    default
   Labels:       <none>
   Annotations:  <none>

   Type:  Opaque

   Data
   ====
   ca.crt:  1127 bytes

This Secret would then be used in the ReplicationSource and/or
ReplicationDestination objects:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup-with-customca
   spec:
     # ... fields omitted ...
     kopia:
       # ... other fields omitted ...
       customCA:
         secretName: tls-secret
         key: ca.crt

To use a customCA in a ConfigMap, specify ``configMapName`` in the spec instead
of ``secretName``, for example:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup-with-customca
   spec:
     # ... fields omitted ...
     kopia:
       # ... other fields omitted ...
       customCA:
         configMapName: tls-configmap-name
         key: ca.crt

Troubleshooting
===============

Common issues and solutions when using the Kopia mover:

Repository connection issues
----------------------------

**Problem**: Kopia fails to connect to the repository with authentication errors.

**Solution**: Verify that the credentials in your ``kopia-config`` Secret are correct:

.. code-block:: console

   $ kubectl get secret kopia-config -o yaml
   $ kubectl describe secret kopia-config

For S3-compatible storage, ensure the endpoint URL is correct and accessible from the cluster.

**Problem**: Repository initialization fails.

**Solution**: Ensure the storage backend is accessible and the bucket/container exists:

.. code-block:: console

   # Check if the storage backend is reachable
   $ kubectl run test-pod --image=curlimages/curl --rm -it -- curl -v http://minio.minio.svc.cluster.local:9000

Cache volume issues
-------------------

**Problem**: Kopia mover fails with "no space left on device" errors.

**Solution**: Increase the cache capacity in your ReplicationSource/ReplicationDestination:

.. code-block:: yaml

   kopia:
     cacheCapacity: 5Gi  # Increase from default 1Gi

**Problem**: Cache PVC remains after ReplicationDestination is deleted.

**Solution**: Set ``cleanupCachePVC: true`` in your ReplicationDestination to automatically clean up cache volumes.

Performance issues
------------------

**Problem**: Backups are slow or time out.

**Solutions**:

1. Increase parallelism for faster uploads:

   .. code-block:: yaml

      kopia:
        parallelism: 4  # Default is 1

2. Use faster compression or disable compression:

   .. code-block:: yaml

      kopia:
        compression: s2   # Faster than zstd
        # or
        compression: none # No compression

3. Increase mover resource limits:

   .. code-block:: yaml

      kopia:
        moverResources:
          limits:
            cpu: "2"
            memory: 4Gi
          requests:
            cpu: "1"
            memory: 2Gi

Snapshot consistency issues
---------------------------

**Problem**: Database backups are inconsistent or corrupted.

**Solution**: Use ``beforeSnapshot`` actions to ensure consistency:

.. code-block:: yaml

   kopia:
     actions:
       beforeSnapshot: "sync && echo 3 > /proc/sys/vm/drop_caches"
       # For databases, use appropriate flush/lock commands
       # beforeSnapshot: "mysqldump --single-transaction --all-databases > /data/backup.sql"

**Problem**: Actions fail or time out.

**Solution**: Ensure commands are compatible with the container environment and have appropriate timeouts. Actions run in a basic shell environment within the data container.

Repository maintenance issues
-----------------------------

**Problem**: Repository grows too large despite retention policies.

**Solution**: Ensure maintenance is running regularly:

.. code-block:: yaml

   kopia:
     maintenanceIntervalDays: 3  # Run maintenance more frequently

Check the ``lastMaintenance`` field in the ReplicationSource status to verify maintenance is occurring.

**Problem**: Multiple backup sources conflict.

**Solution**: While Kopia supports concurrent access, for better isolation use separate repository paths:

.. code-block:: yaml

   # Source 1
   KOPIA_REPOSITORY: s3://bucket/app1-backups
   
   # Source 2
   KOPIA_REPOSITORY: s3://bucket/app2-backups

Restore issues
--------------

**Problem**: Restore fails with "snapshot not found" errors.

**Solution**: Verify the snapshot exists and check timestamp format:

.. code-block:: yaml

   kopia:
     restoreAsOf: "2024-01-15T18:30:00Z"  # Must use RFC-3339 format

**Problem**: Partial restore or missing files.

**Solution**: Ensure the destination PVC has sufficient space and appropriate permissions. Check that the ``copyMethod`` is set correctly for your use case.

Advanced policy configuration
===============================

VolSync supports Kopia's advanced policy-based configuration system, allowing users to define comprehensive backup policies using ConfigMaps or Secrets. This enables fine-grained control over Kopia's behavior including compression, retention, ignore patterns, error handling, and more.

Overview of Kopia policies
---------------------------

Kopia uses a hierarchical policy system with four levels:

1. **Global Policy** - Applies to all snapshots in the repository
2. **Per-Host Policy (@host)** - Applies to all snapshots from a specific machine  
3. **Per-User Policy (user@host)** - Applies to all snapshots from a specific user
4. **Per-Directory Policy (user@host:path)** - Applies to specific directories

More specific policies override less specific ones (Directory → User → Host → Global).

VolSync currently supports global policy configuration, which provides comprehensive control over backup behavior across the entire repository.

Policy configuration options
-----------------------------

The ``policyConfig`` field allows you to specify ConfigMaps or Secrets containing Kopia policy JSON files:

.. code-block:: yaml

   kopia:
     repository: kopia-config
     policyConfig:
       # Use either configMapName OR secretName, not both
       configMapName: kopia-policies
       # secretName: kopia-policy-secret
       
       # Optional: customize filenames (defaults shown)
       globalPolicyFilename: global-policy.json
       repositoryConfigFilename: repository.config

.. note::
   The ``policyConfig`` field is available for both ReplicationSource and ReplicationDestination objects, allowing policy-driven configuration for both backup and restore operations.

``configMapName``
   The name of a ConfigMap containing policy configuration files. Use this for non-sensitive policy data.

``secretName``
   The name of a Secret containing policy configuration files. Use this for policies containing sensitive information like scripts or credentials.

``globalPolicyFilename``
   The filename for the global policy configuration within the ConfigMap/Secret. Defaults to ``global-policy.json``.

``repositoryConfigFilename``
   The filename for repository-specific settings within the ConfigMap/Secret. Defaults to ``repository.config``.

Creating policy files
----------------------

Global policy file format
~~~~~~~~~~~~~~~~~~~~~~~~~~

The global policy file should be in JSON format and can include comprehensive backup settings:

.. code-block:: json

   {
     "compression": {
       "compressorName": "zstd",
       "minSize": 1024,
       "maxSize": 1048576
     },
     "retention": {
       "keepLatest": 10,
       "keepHourly": 24,
       "keepDaily": 30,
       "keepWeekly": 4,
       "keepMonthly": 12,
       "keepAnnual": 3
     },
     "files": {
       "ignore": [
         ".DS_Store",
         "Thumbs.db",
         "*.tmp",
         "*.log",
         "node_modules/",
         ".git/",
         "__pycache__/"
       ],
       "ignoreCacheDirectories": true,
       "noParentIgnoreRules": false
     },
     "errorHandling": {
       "ignoreFileErrors": false,
       "ignoreDirectoryErrors": false
     },
     "upload": {
       "maxParallelFileReads": 16,
       "maxParallelSnapshots": 4,
       "parallelUploads": 8
     },
     "actions": {
       "beforeSnapshotRoot": {
         "script": "sync && echo 3 > /proc/sys/vm/drop_caches",
         "timeout": "5m",
         "mode": "essential"
       },
       "afterSnapshotRoot": {
         "script": "echo 'Backup completed'",
         "timeout": "2m",
         "mode": "optional"
       }
     }
   }

Repository configuration file format
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The repository configuration file controls repository-wide settings:

.. code-block:: json

   {
     "storage": {
       "type": "s3"
     },
     "caching": {
       "maxCacheSize": 1073741824,
       "maxListCacheDuration": 600
     },
     "enableActions": true,
     "compression": {
       "onlyCompress": ["*.txt", "*.log"],
       "neverCompress": ["*.jpg", "*.png", "*.mp4"],
       "minSize": 1024,
       "maxSize": 1073741824
     }
   }

.. note::
   The ``enableActions`` setting in the repository configuration is required for pre/post snapshot actions defined in policies to execute. Without this setting, action scripts will be ignored even if defined in the global policy.

Policy configuration examples
-----------------------------

Basic policy configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~

Create a ConfigMap with comprehensive backup policies:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd",
           "minSize": 1024
         },
         "retention": {
           "keepLatest": 5,
           "keepDaily": 14,
           "keepWeekly": 8,
           "keepMonthly": 6
         },
         "files": {
           "ignore": [
             "*.log",
             "*.tmp",
             ".cache/",
             "node_modules/"
           ],
           "ignoreCacheDirectories": true
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "caching": {
           "maxCacheSize": 2147483648
         }
       }

Use the policy configuration in a ReplicationSource:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup-with-policies
   spec:
     sourcePVC: app-data
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: kopia-policies
       # Standard fields still work as fallbacks
       cacheCapacity: 5Gi
       copyMethod: Snapshot

Migration from basic configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

You can gradually migrate from basic VolSync configuration to policy-based configuration while maintaining backward compatibility:

**Before (Basic Configuration)**:

.. code-block:: yaml

   kopia:
     repository: kopia-config
     retain:
       daily: 7
       weekly: 4
     compression: zstd
     parallelism: 2

**After (Policy-Based Configuration)**:

.. code-block:: yaml

   kopia:
     repository: kopia-config
     # Add policy configuration
     policyConfig:
       configMapName: kopia-policies
     # Keep existing fields as fallbacks
     retain:
       daily: 7
       weekly: 4  
     compression: zstd
     parallelism: 2

This approach allows incremental adoption of policy-based configuration while ensuring existing backups continue to work.

Advanced policy with actions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

For applications requiring specific pre/post backup actions:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-database-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd"
         },
         "retention": {
           "keepLatest": 3,
           "keepDaily": 7,
           "keepWeekly": 4
         },
         "actions": {
           "beforeSnapshotRoot": {
             "script": "mysqldump --single-transaction --all-databases > /data/backup.sql",
             "timeout": "10m",
             "mode": "essential"
           },
           "afterSnapshotRoot": {
             "script": "rm -f /data/backup.sql",
             "timeout": "1m",
             "mode": "optional"
           }
         },
         "files": {
           "ignore": [
             "*.log",
             "mysql-bin.*"
           ]
         }
       }

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-backup
   spec:
     sourcePVC: mysql-data
     trigger:
       schedule: "0 1 * * *"
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: kopia-database-policies
       copyMethod: Clone

Environment-specific policies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Different policies for development and production environments:

.. code-block:: yaml

   # Development policies (faster, less retention)
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-dev-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "s2"  # Faster compression
         },
         "retention": {
           "keepLatest": 3,
           "keepDaily": 7
         },
         "upload": {
           "parallelUploads": 2
         }
       }

   ---
   # Production policies (better compression, longer retention)
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-prod-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd"  # Better compression
         },
         "retention": {
           "keepLatest": 10,
           "keepDaily": 30,
           "keepWeekly": 12,
           "keepMonthly": 12,
           "keepAnnual": 5
         },
         "upload": {
           "parallelUploads": 8
         }
       }

Using policies with ReplicationDestination
------------------------------------------

Policy configuration can also be used with ReplicationDestination for restore operations:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-with-policies
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: kopia-policies
       destinationPVC: restored-data
       copyMethod: Direct

Policy precedence and interaction
---------------------------------

When both policy files and VolSync spec fields are provided:

1. **Policy files take precedence** for settings they define
2. **VolSync spec fields act as fallbacks** for undefined policy settings  
3. **Repository-level settings** override both for repository-wide configurations

For example, if both ``policyConfig`` and spec-level ``retain`` are specified:

.. code-block:: yaml

   kopia:
     policyConfig:
       configMapName: kopia-policies  # Contains retention: {"keepDaily": 14}
     retain:
       daily: 7   # This becomes fallback since policy defines keepDaily
       weekly: 4  # This is used since policy doesn't define keepWeekly

Troubleshooting policy configuration
------------------------------------

Verifying policy application
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Check if policies are being applied correctly:

.. code-block:: console

   # Check the ConfigMap contents
   $ kubectl get configmap kopia-policies -o yaml
   
   # View job logs to see policy import messages
   $ kubectl logs <replicationsource-job-name>
   
   # Look for policy import success/failure messages
   $ kubectl logs <replicationsource-job-name> | grep -i policy

Common policy issues
~~~~~~~~~~~~~~~~~~~~

**Invalid JSON format**
   Policy files must be valid JSON. Use a JSON validator to check syntax before creating ConfigMaps/Secrets.

**Missing policy files**
   Ensure the specified filenames exist in the ConfigMap/Secret with the correct names. Default filenames are ``global-policy.json`` and ``repository.config``.

**Policy import failures**
   Check job logs for specific error messages about policy import failures. Common issues include invalid policy syntax or conflicting policy settings.

**ConfigMap/Secret not found**
   Verify the ConfigMap or Secret exists in the same namespace as the ReplicationSource/ReplicationDestination. Policy resources must be in the same namespace as the VolSync resources.

**Actions not executing**
   Ensure ``enableActions`` is set to ``true`` in the repository configuration file. Actions defined in policies will be silently ignored if repository-level actions are disabled.

**Policy precedence confusion**
   Remember that policy file settings override VolSync spec fields. If unexpected behavior occurs, check both policy files and spec fields to understand which settings are taking precedence.

Best practices for policy management
------------------------------------

1. **Use ConfigMaps** for non-sensitive policy data
2. **Use Secrets** for policies containing sensitive scripts or configurations  
3. **Test policies** in development environments before production use
4. **Version control** your policy configurations
5. **Document policy changes** and their expected impact
6. **Monitor backup success** after implementing new policies
7. **Use meaningful names** for ConfigMaps/Secrets to identify their purpose
8. **Validate JSON** before creating ConfigMaps/Secrets

Security considerations
-----------------------

Policy files can contain executable scripts in the ``actions`` section. Consider these security aspects:

* **Validate script content** before deploying policies
* **Use Secrets** for policies containing sensitive information
* **Apply appropriate RBAC** to ConfigMaps/Secrets containing policies
* **Monitor policy changes** through change management processes
* **Limit script complexity** to reduce potential security risks

Policy configuration quick reference
====================================

Field reference
---------------

.. code-block:: yaml

   kopia:
     repository: kopia-config
     policyConfig:
       # Required: specify either configMapName OR secretName
       configMapName: my-policies     # ConfigMap containing policy files
       secretName: my-policy-secret   # Secret containing policy files
       
       # Optional: custom filenames (defaults shown)
       globalPolicyFilename: global-policy.json      # Global policy file
       repositoryConfigFilename: repository.config   # Repository config file

Global policy structure
-----------------------

.. code-block:: json

   {
     "compression": {
       "compressorName": "zstd|gzip|s2|none",
       "minSize": 1024,
       "maxSize": 1048576
     },
     "retention": {
       "keepLatest": 10,
       "keepHourly": 24,
       "keepDaily": 30,
       "keepWeekly": 4,
       "keepMonthly": 12,
       "keepAnnual": 3
     },
     "files": {
       "ignore": ["*.tmp", "*.log", ".cache/"],
       "ignoreCacheDirectories": true,
       "noParentIgnoreRules": false
     },
     "errorHandling": {
       "ignoreFileErrors": false,
       "ignoreDirectoryErrors": false
     },
     "upload": {
       "maxParallelFileReads": 16,
       "maxParallelSnapshots": 4,
       "parallelUploads": 8
     },
     "actions": {
       "beforeSnapshotRoot": {
         "script": "sync && echo 3 > /proc/sys/vm/drop_caches",
         "timeout": "5m",
         "mode": "essential|optional"
       }
     }
   }

Repository configuration structure
----------------------------------

.. code-block:: json

   {
     "enableActions": true,
     "caching": {
       "maxCacheSize": 1073741824,
       "maxListCacheDuration": 600
     },
     "compression": {
       "onlyCompress": ["*.txt", "*.log"],
       "neverCompress": ["*.jpg", "*.png", "*.mp4"],
       "minSize": 1024,
       "maxSize": 1073741824
     }
   }

Common use cases
----------------

**Basic policy setup**:
  Use ``configMapName`` with comprehensive retention and compression settings

**Database backups**:
  Use policy actions for consistent snapshots with ``beforeSnapshot`` commands

**Multi-environment**:
  Create separate ConfigMaps for dev, staging, and production policies

**Sensitive configurations**:
  Use ``secretName`` for policies containing scripts or credentials

**Migration**:
  Add ``policyConfig`` while keeping existing spec fields as fallbacks

Kopia-specific features
=======================

Compression
-----------

Kopia offers several compression algorithms that can significantly reduce backup
size and transfer time:

* **zstd** (default): Excellent compression ratio with good speed
* **gzip**: Standard compression, widely compatible
* **s2**: Fast compression with lower CPU usage
* **none**: No compression for already compressed data

.. code-block:: yaml

   kopia:
     compression: zstd

Parallelism
-----------

Kopia can upload data using multiple parallel streams, which can significantly
improve backup performance on high-bandwidth connections:

.. code-block:: yaml

   kopia:
     parallelism: 4  # Use 4 parallel upload streams

Actions (Hooks)
---------------

Kopia supports pre and post snapshot actions that can be used to ensure data
consistency before taking backups:

.. code-block:: yaml

   kopia:
     actions:
       beforeSnapshot: "mysqldump --single-transaction --routines --triggers --all-databases > /data/mysql-dump.sql"
       afterSnapshot: "rm -f /data/mysql-dump.sql"

These actions run inside the source PVC container and can be used to:

* Flush database buffers
* Create consistent application snapshots  
* Pause application writes
* Clean up temporary files after backup

.. note::
   For more advanced action configuration, consider using the ``policyConfig`` option which allows defining actions with timeouts, error handling modes, and more sophisticated scripting capabilities.

Concurrent Access
-----------------

Unlike some other backup tools, Kopia supports multiple clients safely accessing
the same repository. This means multiple VolSync instances can backup to the
same repository path without corruption, making it easier to manage centralized
backup repositories.