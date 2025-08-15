======================
Backup Configuration
======================

.. contents:: Backup Setup and Configuration
   :local:

This section covers how to configure ReplicationSource resources for Kopia backups, including various backup options, scheduling, and advanced features.

Basic Backup Configuration
---------------------------

A basic backup configuration requires a source PVC and a repository configuration. Here's a minimal example:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     sourcePVC: mydata
     trigger:
       schedule: "0 0 * * *"  # Daily at midnight
     kopia:
       repository: kopia-config

Backup options
--------------

There are a number of configuration options available for customizing backup behavior:

.. include:: ../inc_src_opts.rst

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

customCA
   This option allows a custom certificate authority to be used when making TLS
   (https) connections to the remote repository. See :doc:`custom-ca` for
   detailed configuration instructions.

hostname
   This specifies a custom hostname for the Kopia client. When not provided,
   VolSync automatically generates a hostname that is ALWAYS just the namespace name.
   This is intentional design - all ReplicationSources in a namespace share the same
   hostname, representing a single tenant. Combined with unique usernames (from object names),
   this ensures unique identities without collision risk. The namespace-only hostname design
   simplifies multi-tenancy and makes behavior predictable.
   See :doc:`multi-tenancy` for details on hostname generation.

repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository. See :doc:`backends` for
   supported remote storage backends and configuration examples. For filesystem-based
   backups using PVCs, also configure the ``repositoryPVC`` field.

repositoryPVC
   **ReplicationSource Only**: This option specifies a PVC to use as a filesystem-based backup repository.
   When set, Kopia will write backups directly to this PVC instead of a remote repository.
   The PVC must exist in the same namespace as the ReplicationSource.
   The repository will be created at the fixed path ``/kopia/repository`` within the mounted PVC.
   
   .. important::
      This field is only available for ReplicationSource (backup operations).
      ReplicationDestination does not support ``repositoryPVC`` - you must use a
      repository secret with appropriate backend configuration for restore operations.
   
   See :doc:`filesystem-destination` for detailed configuration and examples.

sourcePath
   This specifies the path within the source PVC to backup. If not specified,
   the entire PVC will be backed up. See the Source Path Override section below
   for detailed usage.

username
   This specifies a custom username for the Kopia client. When not provided,
   VolSync automatically generates a username from the ReplicationSource name.
   Combined with the namespace-based hostname, this creates a unique identity
   for each backup source. Since Kubernetes prevents duplicate object names
   in a namespace, there's no risk of collision. See :doc:`multi-tenancy` for
   details on username generation.

Source Path Override
--------------------

VolSync provides two complementary path override features for Kopia backups:

1. **sourcePath**: Selects which directory within the PVC to backup (input selection)
2. **sourcePathOverride**: Controls how the path appears in snapshots (identity preservation)

sourcePath Parameter
~~~~~~~~~~~~~~~~~~~~

The ``sourcePath`` parameter allows you to backup a specific directory within your PVC rather than the entire volume. This feature provides several benefits:

**Purpose and Benefits**

- **Selective Backup**: Only backup relevant data, reducing storage costs and backup time
- **Application Integration**: Backup specific application data directories
- **Compliance**: Meet regulatory requirements by excluding certain data types
- **Performance**: Reduce I/O overhead by backing up only necessary files
- **Organization**: Create logical backup boundaries within shared volumes

sourcePathOverride Parameter
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The ``sourcePathOverride`` parameter allows you to preserve the original application path identity in snapshots, even when backing up from different locations like temporary mounts or volume snapshots.

**Purpose and Benefits**

- **Path Identity Preservation**: Maintain consistent snapshot paths regardless of backup mount location
- **Flexible Backup Sources**: Backup from snapshots or temporary mounts while preserving original paths  
- **Restore Compatibility**: Ensure snapshots can be restored to the expected application paths
- **Cross-Environment Consistency**: Maintain path consistency across development, staging, and production

**Key Difference**

- ``sourcePath``: "What to backup" - selects data from ``/app/data`` instead of entire volume
- ``sourcePathOverride``: "How to identify it" - stores snapshots as ``/var/lib/myapp/data`` even if backing up from ``/mnt/snapshot/data``

**Common Use Cases**

Database Backups
~~~~~~~~~~~~~~~~~

For database applications that store data in specific subdirectories:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: postgres-data-backup
   spec:
     sourcePVC: postgres-storage
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM
     kopia:
       repository: kopia-config
       # Only backup the actual database data directory
       sourcePath: "/var/lib/postgresql/data"

Application Configuration Backups
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

For applications with separate data and configuration directories:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-config-backup
   spec:
     sourcePVC: app-storage
     trigger:
       schedule: "0 6 * * *"
     kopia:
       repository: kopia-config
       # Backup only configuration files
       sourcePath: "/app/config"

**Common Use Cases for sourcePathOverride**

Snapshot-Based Backups
^^^^^^^^^^^^^^^^^^^^^^^

When backing up from volume snapshots, preserve the original application path:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-snapshot-backup
   spec:
     sourcePVC: app-data-snapshot  # Volume snapshot clone
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       # Backup from snapshot mount at /data, but preserve original application path
       sourcePathOverride: "/var/lib/myapp/data"

Temporary Mount Backups
^^^^^^^^^^^^^^^^^^^^^^^

When applications mount data at non-standard locations during backup:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: temp-mount-backup
   spec:
     sourcePVC: app-temp-mount  # Temporary mount PVC
     trigger:
       schedule: "0 3 * * *"
     kopia:
       repository: kopia-config
       sourcePath: "/backup-temp"  # Actual path in temp mount
       # But identify snapshots with production path for restore compatibility
       sourcePathOverride: "/opt/application/data"

Cross-Environment Consistency
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

Maintain consistent snapshot paths across environments with different mount structures:

.. code-block:: yaml

   ---
   # Development environment - data at /dev/app-data
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: dev-app-backup
     namespace: development
   spec:
     sourcePVC: dev-app-storage
     kopia:
       repository: kopia-config
       sourcePath: "/dev/app-data"  # Development mount path
       # Use consistent path identity for all environments
       sourcePathOverride: "/app/data"
       
   ---
   # Production environment - data at /opt/myapp/data  
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: prod-app-backup
     namespace: production
   spec:
     sourcePVC: prod-app-storage
     kopia:
       repository: kopia-config
       sourcePath: "/opt/myapp/data"  # Production mount path
       # Same path identity as development for easy cross-environment restores
       sourcePathOverride: "/app/data"

**Configuration Examples**

Basic sourcePath Override:

.. code-block:: yaml

   spec:
     kopia:
       sourcePath: "/data/important"

Basic sourcePathOverride:

.. code-block:: yaml

   spec:
     kopia:
       # Back up everything, but store snapshots with specific path identity
       sourcePathOverride: "/var/lib/database/data"

Combined sourcePath and sourcePathOverride:

.. code-block:: yaml

   spec:
     kopia:
       sourcePath: "/backup-staging/app"  # Only backup this subdirectory
       sourcePathOverride: "/app/data"    # But identify snapshots with production path

Multiple Path Scenarios:

.. code-block:: yaml

   # Backup 1: Application data
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-data-backup
   spec:
     sourcePVC: shared-volume
     kopia:
       repository: kopia-config
       sourcePath: "/app/data"
       username: "app-data-user"
   
   ---
   # Backup 2: Application logs (separate backup policy)
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-logs-backup
   spec:
     sourcePVC: shared-volume
     kopia:
       repository: kopia-config
       sourcePath: "/app/logs"
       username: "app-logs-user"

Scheduling and Triggers
-----------------------

Kopia backups can be triggered in several ways:

**Scheduled Backups**

Use cron expressions to schedule regular backups:

.. code-block:: yaml

   spec:
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM

**Manual Triggers**

For one-time or on-demand backups:

.. code-block:: yaml

   spec:
     trigger:
       manual: backup-now

For more information on triggers, see :doc:`../triggers`.

Advanced Configuration
-----------------------

The following sections cover advanced policy configuration, repository settings,
and performance tuning options available for Kopia backups.

Repository Policy Configuration
--------------------------------

VolSync provides comprehensive policy configuration for Kopia backups, allowing you to define retention policies, compression settings, snapshot actions, and advanced repository configurations. These can be configured through inline settings or external policy files via ConfigMaps and Secrets.

.. note::
   **Policy Configuration Features**:
   
   - ✅ External policy files via ConfigMap/Secret
   - ✅ Structured repository configuration via inline JSON
   - ✅ Global policy files for retention, compression, and file patterns
   - ✅ Repository config files for actions and speed limits
   - ✅ JSON validation and safe parsing with 1MB size limits

Policy Configuration Methods
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

There are three ways to configure Kopia policies:

1. **Inline Configuration** - Simple policies directly in the ReplicationSource spec
2. **External Policy Files** - Complex policies via ConfigMap/Secret
3. **Structured Repository Config** - Advanced JSON configuration for Kopia's native features

Inline Retention Policy
~~~~~~~~~~~~~~~~~~~~~~~~

The simplest way to configure retention is through the ``retain`` field:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     sourcePVC: mydata
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       retain:
         hourly: 24       # Keep 24 hourly snapshots
         daily: 7         # Keep 7 daily snapshots
         weekly: 4        # Keep 4 weekly snapshots
         monthly: 12      # Keep 12 monthly snapshots
         yearly: 5        # Keep 5 yearly snapshots

**Retention Policy Fields:**

- ``hourly``: Number of hourly snapshots to retain (default: not set)
- ``daily``: Number of daily snapshots to retain (default: not set)
- ``weekly``: Number of weekly snapshots to retain (default: not set)
- ``monthly``: Number of monthly snapshots to retain (default: not set)
- ``yearly``: Number of yearly snapshots to retain (default: not set)

When not specified, Kopia uses its default retention policy.

Inline Compression Configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Configure compression algorithm for better storage efficiency. The compression field is now fully functional and properly applies compression policies via ``kopia policy set`` commands:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     sourcePVC: mydata
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       compression: "zstd"  # Now works as expected!

**Supported Compression Algorithms:**

Note: Validation isn't performed on compression algorithm input - Kopia handles validation. The list may change as Kopia adds new algorithms. Users can use ``kopia benchmark compression`` to test which algorithm works best for their data.

Based on the official Kopia documentation:

**S2 variants** (Snappy/S2 compression):

- ``s2-default``: Default S2 compression
- ``s2-better``: Better compression ratio, slightly slower
- ``s2-parallel-4``: Parallel compression with 4 threads
- ``s2-parallel-8``: Parallel compression with 8 threads
- Note: ``s2-parallel-n`` supports various concurrency levels

**ZSTD variants** (Zstandard compression - recommended):

- ``zstd``: Standard zstd compression (good balance)
- ``zstd-fastest``: Fastest zstd mode, lower compression
- ``zstd-better-compression``: Better compression ratio
- ``zstd-best-compression``: Maximum compression ratio

**GZIP variants** (Traditional gzip):

- ``gzip``: Standard gzip compression
- ``gzip-best-speed``: Fastest gzip mode
- ``gzip-best-compression``: Maximum gzip compression

**PGZIP variants** (Parallel gzip):

- ``pgzip``: Parallel gzip compression
- ``pgzip-best-speed``: Fastest parallel gzip mode
- ``pgzip-best-compression``: Maximum parallel gzip compression

**DEFLATE variants**:

- ``deflate-best-speed``: Fastest deflate mode
- ``deflate-default``: Standard deflate compression
- ``deflate-best-compression``: Maximum deflate compression

**Other algorithms**:

- ``lz4``: Very fast compression with reasonable ratio
- ``none``: No compression (fastest but uses more storage)

**How Compression Works:**

- Compression is applied **per-path**, not globally
- The direct ``compression`` field takes precedence over manual JSON configuration
- Different ReplicationSources can use different compression settings
- Compression policies are set via ``kopia policy set`` commands during backup operations

.. note::
   Compression can be changed at any time using Kopia's policy system. Each source path can have its own compression policy, allowing different compression algorithms for different ReplicationSources within the same repository. Changes take effect on the next snapshot.

Snapshot Actions (Hooks)
~~~~~~~~~~~~~~~~~~~~~~~~

Define commands to run before and after snapshots for application consistency:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-backup
   spec:
     sourcePVC: database-storage
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       actions:
         beforeSnapshot: "mysqldump -u root --all-databases > /data/backup.sql"
         afterSnapshot: "rm -f /data/backup.sql"

**Common Use Cases for Actions:**

1. **Database Consistency**: Flush tables and create dumps before backup
2. **Application Quiesce**: Stop or pause services during backup
3. **Cache Clearing**: Remove temporary files before snapshot
4. **Notification**: Send alerts when backups complete

External Policy Configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

External policy file configuration via ConfigMaps and Secrets allows you to apply complex Kopia
policies that go beyond what's possible with inline configuration.

**Supported Features:**

- External policy files via ConfigMap/Secret
- Global policy files (JSON) - Applied via ``kopia policy set --global``
- Repository config files (JSON) - Sets environment variables for actions
- Structured repository config - Used with ``kopia repository connect from-config``

**Using ConfigMap for Policy Files:**

.. code-block:: yaml

   # ✅ WORKING FEATURE - Available Now
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-policies
   data:
     # Global policy file - Applied via 'kopia policy set --global'
     global-policy.json: |
       {
         "retention": {
           "keepDaily": 7,
           "keepWeekly": 4,
           "keepMonthly": 12,
           "keepYearly": 3
         },
         "compression": {
           "compressor": "zstd",
           "compressionLevel": 3
         },
         "files": {
           "ignore": [".git", "*.tmp", "node_modules"],
           "dotIgnoreFiles": [".kopiaignore"],
           "maxFileSize": 10737418240
         },
         "errorHandling": {
           "ignoreFileErrors": true,
           "ignoreDirectoryErrors": false
         },
         "scheduling": {
           "uploadParallelism": 4
         }
       }
     # Repository config file - Sets environment variables for actions
     repository.config: |
       {
         "enableActions": true,
         "actionCommandTimeout": "5m",
         "uploadSpeed": 10485760,
         "downloadSpeed": 20971520
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     sourcePVC: mydata
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       # External policy files via ConfigMap
       policyConfig:
         configMapName: kopia-policies
         # Optional: Specify custom filenames (defaults shown)
         globalPolicyFilename: "global-policy.json"
         repositoryConfigFilename: "repository.config"

**Using Secret for Policy Files:**

For sensitive policy configurations, you can use a Secret instead of a ConfigMap:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-policies-secret
   type: Opaque
   stringData:
     global-policy.json: |
       {
         "retention": {
           "keepHourly": 24,
           "keepDaily": 30,
           "keepWeekly": 8,
           "keepMonthly": 24,
           "keepYearly": 10
         },
         "compression": {
           "compressor": "gzip",
           "compressionLevel": 9
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "encryptionAlgorithm": "AES256-GCM-HMAC-SHA256"
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: secure-backup
   spec:
     sourcePVC: sensitive-data
     trigger:
       schedule: "0 */6 * * *"
     kopia:
       repository: kopia-config
       policyConfig:
         secretName: kopia-policies-secret  # Use Secret instead of ConfigMap

**Structured Repository Configuration (Inline JSON):**

For advanced Kopia configurations, you can provide a complete repository configuration
inline as JSON. This is particularly useful for complex setups that need Kopia's
native ``repository connect from-config`` functionality:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: advanced-backup
   spec:
     sourcePVC: application-data
     trigger:
       schedule: "0 */4 * * *"
     kopia:
       repository: kopia-config
       policyConfig:
         # Inline JSON configuration for advanced repository settings
         repositoryConfig: |
           {
             "storage": {
               "type": "s3",
               "config": {
                 "bucket": "my-backup-bucket",
                 "prefix": "kopia-backups/",
                 "endpoint": "s3.amazonaws.com",
                 "region": "us-west-2"
               }
             },
             "policies": {
               "compression": {
                 "compressor": "zstd",
                 "compressionLevel": 3
               },
               "retention": {
                 "keepLatest": 10,
                 "keepDaily": 30,
                 "keepWeekly": 8,
                 "keepMonthly": 12,
                 "keepYearly": 5
               },
               "scheduling": {
                 "uploadParallelism": 8,
                 "downloadParallelism": 4
               },
               "splitter": {
                 "algorithm": "DYNAMIC-4M-BUZHASH",
                 "minSize": 1048576,
                 "maxSize": 8388608
               }
             },
             "caching": {
               "cacheDirectory": "/tmp/kopia-cache",
               "maxCacheSize": 5368709120,
               "metadataCacheSize": 1073741824
             },
             "maintenance": {
               "owner": "@local",
               "quickCycle": {
                 "interval": "1h"
               },
               "fullCycle": {
                 "interval": "24h"
               }
             }
           }

.. warning::
   The ``repositoryConfig`` field has a 1MB size limit for security reasons.
   Very large configurations should be split into separate policy files.

Policy File Structure and Contents
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

**Global Policy File (global-policy.json)**

The global policy file configures repository-wide settings that apply to all snapshots.
It's applied using ``kopia policy set --global`` command:

.. code-block:: json

   {
     "retention": {
       "keepLatest": 10,
       "keepHourly": 48,
       "keepDaily": 30,
       "keepWeekly": 8,
       "keepMonthly": 24,
       "keepYearly": 10
     },
     "compression": {
       "compressor": "zstd",
       "compressionLevel": 3
     },
     "files": {
       "ignore": [
         "*.tmp",
         "*.swp",
         "~*",
         ".DS_Store",
         "Thumbs.db"
       ],
       "dotIgnoreFiles": [".kopiaignore", ".gitignore"],
       "oneFileSystem": true,
       "maxFileSize": 10737418240
     },
     "errorHandling": {
       "ignoreFileErrors": true,
       "ignoreDirectoryErrors": false,
       "ignoreUnknownTypes": true
     },
     "scheduling": {
       "uploadParallelism": 4,
       "downloadParallelism": 2
     },
     "splitter": {
       "algorithm": "DYNAMIC-4M-BUZHASH"
     },
     "actions": {
       "beforeSnapshotRoot": null,
       "afterSnapshotRoot": null
     }
   }

**Repository Config File (repository.config)**

The repository config file sets environment variables and repository-specific settings:

.. code-block:: json

   {
     "enableActions": true,
     "actionCommandTimeout": "10m",
     "uploadSpeed": 52428800,
     "downloadSpeed": 104857600,
     "encryptionAlgorithm": "AES256-GCM-HMAC-SHA256",
     "hashingAlgorithm": "BLAKE2B-256-128",
     "ecc": "REED-SOLOMON",
     "eccOverheadPercent": 1
   }

Complete Repository Policy Example
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Here's a comprehensive example combining all policy configuration methods:

.. code-block:: yaml

   # First, create a comprehensive policy configuration
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: production-policies
   data:
     global-policy.json: |
       {
         "retention": {
           "keepHourly": 48,
           "keepDaily": 30,
           "keepWeekly": 12,
           "keepMonthly": 24,
           "keepYearly": 7
         },
         "compression": {
           "compressor": "zstd",
           "compressionLevel": 3
         },
         "files": {
           "ignore": ["*.tmp", "*.log", ".git"],
           "oneFileSystem": true
         },
         "scheduling": {
           "uploadParallelism": 8
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "uploadSpeed": 104857600,
         "actionCommandTimeout": "5m"
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: production-backup
   spec:
     sourcePVC: production-data
     trigger:
       schedule: "0 */6 * * *"  # Every 6 hours
     kopia:
       repository: kopia-config
       
       # Use external policy files for complex configuration
       policyConfig:
         configMapName: production-policies
       
       # Inline configuration still works alongside external policies
       # These override external policy settings if conflicts exist
       retain:
         hourly: 48      # Override hourly retention
       
       # Snapshot Actions (can be inline or in policy files)
       actions:
         beforeSnapshot: |
           echo "Starting backup at $(date)" >> /data/backup.log
           sync  # Flush filesystem buffers
         afterSnapshot: |
           echo "Backup completed at $(date)" >> /data/backup.log
       
       # Performance
       parallelism: 4
       
       # Cache
       cacheCapacity: 2Gi

Policy Application in VolSync
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

**Policy Precedence and Application Order:**

1. **Repository Creation**: Compression settings from first connection
2. **External Policy Files**: Applied after repository connection
   - Global policy file via ``kopia policy set --global``
   - Repository config file sets environment variables
3. **Inline Configuration**: Overrides external policies where applicable
4. **Structured Repository Config**: Used with ``kopia repository connect from-config``

**How Policies Are Applied:**

1. **Connection Phase**:
   - Repository connection established
   - Structured config used if provided via ``repositoryConfig``
   - JSON validation ensures safe parsing

2. **Policy Application Phase**:
   - Global policy file applied if present
   - Repository config file processed for environment variables
   - Inline settings override where conflicts exist

3. **Runtime Phase**:
   - Actions execute before/after snapshots
   - Retention enforced during maintenance
   - Speed limits and parallelism applied during transfers

**Security Features:**

- JSON validation before applying any policies
- 1MB size limit on policy files to prevent abuse
- Read-only mounts for policy ConfigMaps/Secrets
- Safe parsing to prevent command injection
- Clear error reporting with exit codes

Policy Configuration Best Practices
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

1. **Choose the Right Method**:
   - Use **inline configuration** for simple retention and basic settings
   - Use **external policy files** for complex multi-faceted policies
   - Use **structured repository config** for advanced Kopia features

2. **Policy File Organization**:
   - Keep global policies in ``global-policy.json``
   - Keep repository settings in ``repository.config``
   - Use descriptive ConfigMap/Secret names

3. **Security Considerations**:
   - Use Secrets for sensitive configurations
   - Validate JSON syntax before deploying
   - Keep policy files under 1MB

4. **Testing Policies**:
   - Test policies in non-production first
   - Monitor policy application in logs
   - Verify retention works as expected

5. **Error Handling**:
   - Policies that fail don't stop backups
   - Check logs for policy application errors
   - Use ``kubectl describe`` to see status

Common Policy Scenarios
~~~~~~~~~~~~~~~~~~~~~~~~

**Scenario 1: Database Backup with Short Retention**

For frequently changing databases where recent backups are most valuable:

.. code-block:: yaml

   spec:
     kopia:
       retain:
         hourly: 24      # Keep all hourly backups for 1 day
         daily: 7        # Keep daily backups for 1 week
         weekly: 2       # Keep only 2 weekly backups
       compression: "zstd"  # Good compression for database dumps
       actions:
         beforeSnapshot: "pg_dump -U postgres mydb > /data/dump.sql"

**Scenario 2: Archive Storage with Long Retention**

For archival data that changes infrequently:

.. code-block:: yaml

   spec:
     kopia:
       retain:
         daily: 1        # Keep only 1 daily snapshot
         monthly: 120    # Keep 10 years of monthly snapshots
         yearly: 999     # Keep all yearly snapshots
       compression: "zstd"  # Maximum compression for archives

**Scenario 3: Development Environment with Minimal Retention**

For development environments where storage efficiency is key:

.. code-block:: yaml

   spec:
     kopia:
       retain:
         hourly: 6       # Keep 6 hours of snapshots
         daily: 3        # Keep 3 days
       compression: "s2"    # Fast compression

**Scenario 4: Compliance-Driven Retention**

For environments with regulatory requirements:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: compliance-policy
   stringData:
     global-policy.json: |
       {
         "retention": {
           "keepDaily": 90,
           "keepMonthly": 84,
           "keepYearly": 999
         },
         "files": {
           "ignore": [],
           "oneFileSystem": true
         },
         "errorHandling": {
           "ignoreFileErrors": false,
           "ignoreDirectoryErrors": false
         }
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: compliance-backup
   spec:
     sourcePVC: regulated-data
     kopia:
       repository: kopia-config
       policyConfig:
         secretName: compliance-policy

**Scenario 5: High-Performance Backup with Speed Limits**

For high-throughput environments with bandwidth management:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: performance-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressor": "s2",
           "compressionLevel": 1
         },
         "scheduling": {
           "uploadParallelism": 16,
           "downloadParallelism": 8
         },
         "splitter": {
           "algorithm": "DYNAMIC-8M-BUZHASH"
         }
       }
     repository.config: |
       {
         "uploadSpeed": 209715200,
         "downloadSpeed": 419430400,
         "enableActions": false
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: high-perf-backup
   spec:
     sourcePVC: performance-data
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: performance-policies
       parallelism: 16
       cacheCapacity: 5Gi

Advanced Policy Examples
~~~~~~~~~~~~~~~~~~~~~~~~

**Multi-Environment Policy Management**

Manage different policies for dev, staging, and production:

.. code-block:: yaml

   # Development environment - minimal retention
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: dev-policies
     namespace: development
   data:
     global-policy.json: |
       {
         "retention": {
           "keepDaily": 3,
           "keepWeekly": 1
         },
         "compression": {
           "compressor": "s2"
         }
       }
   
   # Production environment - comprehensive retention
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: prod-policies
     namespace: production
   data:
     global-policy.json: |
       {
         "retention": {
           "keepHourly": 72,
           "keepDaily": 90,
           "keepWeekly": 52,
           "keepMonthly": 60,
           "keepYearly": 10
         },
         "compression": {
           "compressor": "zstd",
           "compressionLevel": 5
         },
         "files": {
           "oneFileSystem": true,
           "ignoreDeleted": false
         }
       }

**Custom Filename Configuration**

Use non-default filenames for policy files:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: custom-policies
   data:
     my-retention.json: |
       {"retention": {"keepDaily": 30}}
     my-settings.conf: |
       {"enableActions": true}
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: custom-backup
   spec:
     sourcePVC: app-data
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: custom-policies
         globalPolicyFilename: "my-retention.json"
         repositoryConfigFilename: "my-settings.conf"

Troubleshooting Policy Issues
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

**Policy Not Being Applied**

1. Check if policy ConfigMap/Secret exists and is accessible:

.. code-block:: bash

   # Check ConfigMap
   kubectl get configmap kopia-policies -o yaml
   
   # Check Secret
   kubectl get secret kopia-policies-secret -o yaml
   
   # Verify ReplicationSource configuration
   kubectl get replicationsource mydata-backup -o yaml | grep -A10 policyConfig

2. Check mover pod logs for policy application:

.. code-block:: bash

   # Get the latest mover pod
   kubectl get pods -l volsync.backube/owner-name=mydata-backup
   
   # Check logs for policy application
   kubectl logs <pod-name> | grep -i "policy"

**JSON Validation Errors**

Validate your JSON syntax:

.. code-block:: bash

   # Extract and validate JSON from ConfigMap
   kubectl get configmap kopia-policies -o jsonpath='{.data.global-policy\.json}' | jq .
   
   # If jq reports errors, fix the JSON syntax

**Policy File Size Limit Exceeded**

If you see errors about file size:

- Keep policy files under 1MB
- Split large configurations into multiple files
- Use structured repository config for complex setups

**Compression Not Working**

1. Compression is set at repository creation time
2. Check if using the workaround with KOPIA_MANUAL_CONFIG:

.. code-block:: yaml

   # In your repository secret
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   stringData:
     KOPIA_MANUAL_CONFIG: |
       {"compression": {"compressor": "zstd"}}

**Snapshots Not Being Pruned**

1. Check maintenance is running:

.. code-block:: bash

   kubectl get replicationsource mydata-backup -o jsonpath='{.status.kopia.lastMaintenance}'

2. Verify retention policy is set correctly:

.. code-block:: bash

   # Check the policy in the mover pod
   kubectl exec -it <mover-pod> -- kopia policy show --global

**Actions Not Executing**

1. Ensure ``enableActions`` is set to true in repository.config
2. Check action command syntax and availability in container
3. Review logs for action execution errors

Policy Configuration Limitations and Considerations
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

- **Compression algorithm must be set at repository creation** - cannot be changed afterward
- **Policy file size limit**: 1MB maximum for security reasons
- **JSON validation**: Invalid JSON will be rejected with clear error messages
- **Retention policies** are applied during maintenance runs (ensure maintenance is enabled)
- **Actions** run within the mover container context with its available commands
- **ConfigMap vs Secret**: Both work identically, choose based on sensitivity of content
- **Policy conflicts**: Inline configuration takes precedence over external files
- Some policy changes require a maintenance run to take effect

Error Handling and Recovery
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

VolSync's policy configuration includes robust error handling:

1. **Non-Fatal Errors**: Policy application failures don't stop backups
2. **Clear Reporting**: Exit codes and error messages identify issues
3. **Validation**: JSON is validated before application
4. **Safe Defaults**: System continues with default policies if custom ones fail
5. **Summary Reporting**: Logs show which policies succeeded vs failed

Example error handling in logs:

.. code-block:: text

   === Applying policy configuration ===
   Found global policy file: /policies/global-policy.json
   Global policy applied successfully
   Found repository config file: /policies/repository.config
   ERROR: Invalid JSON in repository config file
   Continuing with default settings
   Policy configuration summary: 1 applied, 1 failed

Kopia Feature Implementation Status
------------------------------------

This table provides a clear overview of which Kopia features are currently implemented,
partially working, or planned for future releases:

.. list-table:: Kopia Feature Implementation Status
   :header-rows: 1
   :widths: 30 20 50

   * - Feature
     - Status
     - Notes
   * - Basic Backup/Restore
     - Supported
     - Core functionality works as expected
   * - Repository Backends (S3, GCS, Azure)
     - Supported
     - All major cloud providers supported
   * - Filesystem Repository (repositoryPVC)
     - Supported
     - ReplicationSource only, not for ReplicationDestination
   * - Retention Policies (inline)
     - Supported
     - Use ``retain`` field with hourly/daily/weekly/monthly/yearly
   * - Snapshot Actions (hooks)
     - Supported
     - beforeSnapshot/afterSnapshot commands work
   * - Source Path Selection
     - Supported
     - sourcePath and sourcePathOverride work correctly
   * - Identity Management
     - Supported
     - username/hostname and sourceIdentity work
   * - enableFileDeletion
     - Supported
     - Clean destination before restore
   * - Custom CA Certificates
     - Supported
     - Support for custom TLS certificates
   * - Compression Field
     - Supported
     - Full support for all compression algorithms with per-path policy application
   * - External Policy Files
     - Supported
     - ConfigMap/Secret policy files with JSON validation and 1MB size limit
   * - policyConfig Field
     - Supported
     - Supports ConfigMap, Secret, and inline repository configuration
   * - Global Policy Files
     - Supported
     - Applied via ``kopia policy set --global`` with JSON validation
   * - Repository Config Files
     - Supported
     - Sets environment variables for actions, speed limits, etc.
   * - Structured Repository Config
     - Supported
     - Inline JSON for ``kopia repository connect from-config``
   * - Custom Policy Filenames
     - Supported
     - globalPolicyFilename and repositoryConfigFilename fields
   * - Repository Auto-Discovery
     - Supported
     - Automatic discovery from ReplicationSource
   * - Environment Variable Variants
     - Supported
     - Both AWS_* and KOPIA_* variables supported
   * - Maintenance Operations
     - Supported
     - Automatic maintenance with configurable intervals
   * - Multi-Tenancy
     - Supported
     - Multiple sources can share repositories
   * - Point-in-Time Restore
     - Supported
     - restoreAsOf, previous, shallow parameters work
   * - Policy Error Handling
     - Supported
     - Non-fatal errors with clear reporting and safe defaults

**Legend:**

- ✅ **Supported**: Feature works as documented
- ❌ **Not Implemented**: Planned feature not yet available

.. note::
   This status reflects the current implementation. Check the latest documentation
   or release notes for updates on feature availability.

Complete Working Examples
-------------------------

**Example 1: Enterprise Backup with External Policies**

A production-ready configuration using external policy files for comprehensive backup management:

.. code-block:: yaml

   # Create comprehensive policy configuration
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: enterprise-kopia-policies
     namespace: production
   data:
     # Global policy for all snapshots in this repository
     global-policy.json: |
       {
         "retention": {
           "keepLatest": 10,
           "keepHourly": 72,
           "keepDaily": 90,
           "keepWeekly": 52,
           "keepMonthly": 60,
           "keepYearly": 10
         },
         "compression": {
           "compressor": "zstd",
           "compressionLevel": 3
         },
         "files": {
           "ignore": [
             "*.tmp",
             "*.swp",
             "*.log",
             ".git/**",
             "node_modules/**",
             "__pycache__/**"
           ],
           "dotIgnoreFiles": [".kopiaignore", ".gitignore"],
           "oneFileSystem": true,
           "maxFileSize": 21474836480
         },
         "errorHandling": {
           "ignoreFileErrors": true,
           "ignoreDirectoryErrors": false,
           "ignoreUnknownTypes": true
         },
         "scheduling": {
           "uploadParallelism": 8,
           "downloadParallelism": 4
         },
         "splitter": {
           "algorithm": "DYNAMIC-4M-BUZHASH",
           "minSize": 1048576,
           "maxSize": 8388608
         }
       }
     # Repository configuration for environment variables
     repository.config: |
       {
         "enableActions": true,
         "actionCommandTimeout": "10m",
         "uploadSpeed": 104857600,
         "downloadSpeed": 209715200,
         "encryptionAlgorithm": "AES256-GCM-HMAC-SHA256",
         "hashingAlgorithm": "BLAKE2B-256-128",
         "ecc": "REED-SOLOMON",
         "eccOverheadPercent": 1
       }
   
   # Repository credentials and connection info
   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-repository
     namespace: production
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: s3://backup-bucket/production/kopia
     KOPIA_PASSWORD: "$(openssl rand -base64 32)"
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     AWS_REGION: us-west-2
   
   # ReplicationSource with external policies
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: production-database
     namespace: production
   spec:
     sourcePVC: postgres-data
     trigger:
       schedule: "0 */4 * * *"  # Every 4 hours
     kopia:
       repository: kopia-repository
       
       # Use external policy files
       policyConfig:
         configMapName: enterprise-kopia-policies
       
       # Database-specific actions
       actions:
         beforeSnapshot: |
           #!/bin/bash
           echo "Starting database backup at $(date)"
           pg_dump -U postgres -d production_db > /data/backup.sql
           sync
         afterSnapshot: |
           #!/bin/bash
           rm -f /data/backup.sql
           echo "Backup completed at $(date)"
       
       # Performance tuning
       parallelism: 8
       cacheCapacity: 5Gi
       
       # Maintenance schedule
       maintenanceIntervalDays: 1

**Example 2: Multi-Tier Application with Structured Config**

Using inline structured repository configuration for complex setups:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: multi-tier-app
     namespace: applications
   spec:
     sourcePVC: app-storage
     trigger:
       schedule: "0 2,14 * * *"  # Twice daily
     kopia:
       repository: kopia-config
       
       # Inline structured repository configuration
       policyConfig:
         repositoryConfig: |
           {
             "storage": {
               "type": "azure",
               "config": {
                 "container": "backups",
                 "storageAccount": "mybackupaccount",
                 "prefix": "kopia/multi-tier/"
               }
             },
             "policies": {
               "retention": {
                 "keepHourly": 24,
                 "keepDaily": 30,
                 "keepWeekly": 8,
                 "keepMonthly": 12
               },
               "compression": {
                 "compressor": "zstd",
                 "compressionLevel": 5
               },
               "scheduling": {
                 "uploadParallelism": 4
               }
             },
             "caching": {
               "cacheDirectory": "/tmp/kopia-cache",
               "maxCacheSize": 10737418240
             }
           }
       
       # Additional inline configuration
       sourcePath: "/app/data"
       parallelism: 4

**Example 3: Secure Financial Data with Secret-Based Policies**

Using Secrets for sensitive policy configurations:

.. code-block:: yaml

   # Sensitive policy configuration in Secret
   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: financial-policies
     namespace: finance
   type: Opaque
   stringData:
     global-policy.json: |
       {
         "retention": {
           "keepDaily": 365,
           "keepMonthly": 84,
           "keepYearly": 999
         },
         "compression": {
           "compressor": "zstd",
           "compressionLevel": 9
         },
         "encryption": {
           "algorithm": "AES256-GCM-HMAC-SHA256",
           "deriveKey": true
         },
         "files": {
           "oneFileSystem": true,
           "ignoreDeleted": false
         },
         "errorHandling": {
           "ignoreFileErrors": false,
           "ignoreDirectoryErrors": false
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "auditLog": true,
         "requireTwoFactorAuth": false,
         "uploadSpeed": 52428800
       }
   
   # Financial data backup
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: financial-backup
     namespace: finance
   spec:
     sourcePVC: financial-data
     trigger:
       schedule: "0 */2 * * *"  # Every 2 hours
     kopia:
       repository: secure-kopia-repo
       
       # Use Secret for sensitive policies
       policyConfig:
         secretName: financial-policies
       
       # Compliance actions
       actions:
         beforeSnapshot: |
           echo "Compliance check: $(date)" >> /data/audit.log
           sha256sum /data/*.db >> /data/audit.log
         afterSnapshot: |
           echo "Backup verified: $(date)" >> /data/audit.log
       
       # High availability settings
       parallelism: 2
       cacheCapacity: 10Gi

**Example 4: Development Environment with Custom Filenames**

Using custom filenames for policy files:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: dev-policies
     namespace: development
   data:
     dev-retention.json: |
       {
         "retention": {
           "keepDaily": 7,
           "keepWeekly": 2
         }
       }
     dev-config.json: |
       {
         "enableActions": false,
         "uploadSpeed": 5242880
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: dev-backup
     namespace: development
   spec:
     sourcePVC: dev-data
     trigger:
       manual: backup-now
     kopia:
       repository: dev-kopia-repo
       policyConfig:
         configMapName: dev-policies
         globalPolicyFilename: "dev-retention.json"
         repositoryConfigFilename: "dev-config.json"

**Example 5: Disaster Recovery with Comprehensive Policies**

Complete disaster recovery setup with all policy features:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: dr-policies
     namespace: disaster-recovery
   data:
     global-policy.json: |
       {
         "retention": {
           "keepLatest": 20,
           "keepHourly": 168,
           "keepDaily": 90,
           "keepWeekly": 52,
           "keepMonthly": 120,
           "keepYearly": 999
         },
         "compression": {
           "compressor": "zstd",
           "compressionLevel": 5
         },
         "files": {
           "ignore": ["*.tmp", "cache/**"],
           "oneFileSystem": true,
           "maxFileSize": 107374182400
         },
         "scheduling": {
           "uploadParallelism": 16,
           "downloadParallelism": 8
         },
         "splitter": {
           "algorithm": "DYNAMIC-8M-BUZHASH"
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "actionCommandTimeout": "30m",
         "uploadSpeed": 524288000,
         "downloadSpeed": 1048576000
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: dr-critical-systems
     namespace: disaster-recovery
   spec:
     sourcePVC: critical-data
     trigger:
       schedule: "0 * * * *"  # Hourly
     kopia:
       repository: dr-repository
       
       # Comprehensive policy configuration
       policyConfig:
         configMapName: dr-policies
       
       # Critical system actions
       actions:
         beforeSnapshot: |
           #!/bin/bash
           # Stop services for consistency
           kubectl scale deployment critical-app --replicas=0
           sleep 10
           # Create application dump
           mysqldump -u root --all-databases > /data/dr-dump.sql
           # Create metadata
           kubectl get all -A -o yaml > /data/cluster-state.yaml
         afterSnapshot: |
           #!/bin/bash
           # Cleanup and restart
           rm -f /data/dr-dump.sql /data/cluster-state.yaml
           kubectl scale deployment critical-app --replicas=3
           # Send notification
           echo "DR backup completed at $(date)" | mail -s "DR Backup" ops@company.com
       
       # DR-specific settings
       sourcePath: "/data"
       parallelism: 16
       cacheCapacity: 20Gi
       maintenanceIntervalDays: 1

Validation and Testing
~~~~~~~~~~~~~~~~~~~~~~

**Validating Policy Configuration:**

1. Check policy files are valid JSON:

.. code-block:: bash

   # Validate ConfigMap JSON
   kubectl get configmap enterprise-kopia-policies -o json | \
     jq '.data["global-policy.json"]' -r | jq .
   
   # Validate Secret JSON
   kubectl get secret financial-policies -o json | \
     jq '.data["global-policy.json"]' -r | base64 -d | jq .

2. Verify policy application in mover logs:

.. code-block:: bash

   # Get mover pod
   POD=$(kubectl get pods -l volsync.backube/owner-name=production-database -o name | head -1)
   
   # Check policy application
   kubectl logs $POD | grep -A5 "Applying policy configuration"

3. Confirm policies in Kopia:

.. code-block:: bash

   # Exec into mover pod
   kubectl exec -it $POD -- bash
   
   # Show applied global policy
   kopia policy show --global
   
   # List all policies
   kopia policy list

**Testing Error Handling:**

1. Test with invalid JSON:

.. code-block:: yaml

   data:
     global-policy.json: |
       {invalid json here}

2. Check logs show graceful handling:

.. code-block:: text

   ERROR: Invalid JSON in global policy file
   Continuing with default policies

3. Verify backup continues despite policy errors

**Performance Monitoring:**

Monitor the impact of policies:

.. code-block:: bash

   # Check upload speed with limits
   kubectl logs $POD | grep -i "upload.*speed"
   
   # Monitor parallelism
   kubectl logs $POD | grep -i "parallel"
   
   # Check compression ratio
   kubectl exec -it $POD -- kopia content stats