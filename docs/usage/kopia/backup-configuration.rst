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
   VolSync automatically generates a hostname based on the namespace and PVC name.
   See :doc:`multi-tenancy` for details on hostname generation.

repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository. See :doc:`backends` for
   supported storage backends and configuration examples.

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

The ``sourcePath`` parameter allows you to backup a specific directory within your PVC rather than the entire volume. This feature provides several benefits:

**Purpose and Benefits**

- **Selective Backup**: Only backup relevant data, reducing storage costs and backup time
- **Application Integration**: Backup specific application data directories
- **Compliance**: Meet regulatory requirements by excluding certain data types
- **Performance**: Reduce I/O overhead by backing up only necessary files
- **Organization**: Create logical backup boundaries within shared volumes

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

**Configuration Examples**

Basic Path Override:

.. code-block:: yaml

   spec:
     kopia:
       sourcePath: "/data/important"

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