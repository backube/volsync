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
   VolSync automatically generates a hostname using just the namespace name.
   All PVCs in a namespace share the same hostname unless customized.
   See :doc:`multi-tenancy` for details on hostname generation.

repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository. See :doc:`backends` for
   supported remote storage backends and configuration examples. For filesystem-based
   backups using PVCs, also configure the ``repositoryPVC`` field.

repositoryPVC
   This option specifies a PVC to use as a filesystem-based backup repository.
   When set, Kopia will write backups directly to this PVC instead of a remote repository.
   The PVC must exist in the same namespace as the ReplicationSource.
   The repository will be created at the fixed path ``/kopia/repository`` within the mounted PVC.
   See :doc:`filesystem-destination` for detailed configuration and examples.

sourcePath
   This specifies the path within the source PVC to backup. If not specified,
   the entire PVC will be backed up. See the Source Path Override section below
   for detailed usage.

username
   This specifies a custom username for the Kopia client. When not provided,
   VolSync automatically generates a username. See :doc:`multi-tenancy` for
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
   apiVersion: volsyncv1alpha1/v1alpha1
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

For advanced policy configuration, repository settings, and performance tuning,
see :doc:`advanced-configuration` and :doc:`advanced-features`.