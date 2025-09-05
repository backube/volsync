=======================
Kopia Database Example
=======================

Kopia backup
============

`Kopia <https://kopia.io/>`_ is a fast, secure, and efficient backup program that
supports encryption, compression, deduplication, and incremental backups. The
following example will use Kopia to create a backup of a source volume.

A MySQL database will be used as the example application.

Creating source PVC to be backed up
-----------------------------------

Create a namespace called ``source``

.. code-block:: console

   $ kubectl create ns source
   $ kubectl annotate namespace source volsync.backube/privileged-movers="true"

.. note::
    The second command to annotate the namespace is used to enable the kopia data mover to run in privileged mode.
    This is because this simple example runs MySQL as root. For your own applications, you can run unprivileged by
    setting the ``moverSecurityContext`` in your ReplicationSource/ReplicationDestination to match that of your
    application in which case the namespace annotation will not be required. See the
    :doc:`permission model documentation </usage/permissionmodel>` for more details.

Deploy the source MySQL database.

.. code:: console

   $ kubectl -n source create -f examples/source-database/

Verify the database is running:

.. code-block:: console

   $ kubectl -n source get pods,pvc,volumesnapshots

   NAME                        READY     STATUS    RESTARTS   AGE
   pod/mysql-87f849f8c-n9j7j   1/1       Running   1          58m

   NAME                                   STATUS    VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS      AGE
   persistentvolumeclaim/mysql-pv-claim   Bound     pvc-adbf57f1-6399-4738-87c9-4c660d982a0f   2Gi        RWO            csi-hostpath-sc   60m


Add a new database:

.. code-block:: console

   $ kubectl exec --stdin --tty -n source $(kubectl get pods -n source | grep mysql | awk '{print $1}') -- /bin/bash

   $ mysql -u root -p$MYSQL_ROOT_PASSWORD

   > show databases;
   +--------------------+
   | Database           |
   +--------------------+
   | information_schema |
   | mysql              |
   | performance_schema |
   | sys                |
   +--------------------+
   4 rows in set (0.00 sec)


   > create database synced;
   > exit

   $ exit

Kopia Repository Setup
----------------------

For the purpose of this tutorial we are using minio as the object storage target
for the backup.

Start ``minio``:

.. code-block:: console

   $ hack/run-minio.sh

The ``kopia-config`` Secret configures the Kopia repository parameters:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: Secret
   metadata:
      name: kopia-config
   type: Opaque
   stringData:
      # The repository url
      KOPIA_REPOSITORY: s3://kopia-repo
      # The repository encryption password
      KOPIA_PASSWORD: my-secure-kopia-password
      # S3 credentials
      AWS_ACCESS_KEY_ID: access
      AWS_SECRET_ACCESS_KEY: password
      # S3 endpoint (required for non-AWS S3)
      AWS_S3_ENDPOINT: http://minio.minio.svc.cluster.local:9000

The above will backup to a bucket called ``kopia-repo``. If the same bucket will
be used for different PVCs or different uses, then make sure to specify a unique
path under ``kopia-repo``. See `Shared S3 Bucket Notes`_ for more detail.

ReplicationSource with Database Consistency and Repository Policies
--------------------------------------------------------------------

Start by configuring the source with database-specific consistency hooks and comprehensive
repository policies. This example demonstrates using Kopia's advanced features including
retention policies, compression, and actions to ensure consistent MySQL backups:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
      name: database-source
      namespace: source
   spec:
      sourcePVC: mysql-pv-claim
      trigger:
         schedule: "*/30 * * * *"
      kopia:
        maintenanceIntervalDays: 7
        repository: kopia-config
        
        # Repository Retention Policy
        # Define how many snapshots to keep at different intervals
        retain:
          hourly: 24      # Keep 24 hourly snapshots (1 day)
          daily: 7        # Keep 7 daily snapshots (1 week)
          weekly: 4       # Keep 4 weekly snapshots (1 month)
          monthly: 6      # Keep 6 monthly snapshots
          yearly: 1       # Keep 1 yearly snapshot
        
        # Compression Configuration
        # Use zstd for optimal balance of speed and compression ratio
        compression: zstd
        
        # Performance Tuning
        # Use multiple parallel streams for faster uploads
        parallelism: 2
        
        # Database Consistency Actions
        # These hooks ensure database consistency during backup
        actions:
          # Before snapshot: Create consistent database dump
          beforeSnapshot: |
            echo "Starting database backup at $(date)" >> /data/backup.log
            mysqldump --single-transaction --routines --triggers --all-databases > /data/mysql-backup.sql
            echo "Database dump completed" >> /data/backup.log
          
          # After snapshot: Clean up temporary files
          afterSnapshot: |
            rm -f /data/mysql-backup.sql
            echo "Cleanup completed at $(date)" >> /data/backup.log
        
        # Use clone for point-in-time consistency
        copyMethod: Clone

In the above ``ReplicationSource`` object:

- The PiT copy of the source data ``mysql-pv-claim`` will be created by cloning
  the source volume.
- The synchronization schedule, ``.spec.trigger.schedule``, is defined by a
  `cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule
  very flexible. In this case, it will take a backup every 30 minutes.
- The kopia repository configuration is provided via the ``kopia-config`` Secret.

**Repository Policy Features:**

- **Retention Policy**: The ``retain`` field defines a comprehensive retention policy:
  
  - ``hourly: 24``: Keeps all hourly snapshots for the last 24 hours
  - ``daily: 7``: Keeps one snapshot per day for the last 7 days
  - ``weekly: 4``: Keeps one snapshot per week for the last 4 weeks
  - ``monthly: 6``: Keeps one snapshot per month for the last 6 months
  - ``yearly: 1``: Keeps one snapshot per year
  
  This policy ensures recent changes are captured frequently while older data
  is retained with decreasing granularity to optimize storage usage.

- **Compression**: ``zstd`` compression is enabled for optimal balance between
  compression ratio and speed. This typically reduces backup size by 50-70%
  for database dumps.

- **Performance**: ``parallelism: 2`` enables parallel upload streams for
  faster backup operations, especially beneficial for large databases.

- **Maintenance**: ``maintenanceIntervalDays: 7`` ensures weekly maintenance
  runs to enforce retention policies and optimize repository storage.

- **Consistency Actions**: The ``actions`` section defines hooks that run
  before and after snapshots:
  
  - ``beforeSnapshot``: Creates a consistent SQL dump using ``mysqldump --single-transaction``
  - ``afterSnapshot``: Cleans up temporary files to avoid backing up unnecessary data
  
  These actions ensure the backup captures a consistent database state even
  during active transactions.

.. note::
   **Database Consistency Best Practices:**
   
   - The ``beforeSnapshot`` action uses ``mysqldump --single-transaction`` to
     create a consistent backup without locking tables
   - The ``--routines`` and ``--triggers`` flags ensure stored procedures and
     triggers are included in the backup
   - Logging timestamps helps track backup duration and troubleshoot issues
   - The SQL dump is cleaned up after snapshot to avoid storing redundant data

.. tip::
   **Policy Inheritance:**
   
   Repository policies are automatically inherited by all snapshots created
   from this ReplicationSource. The retention policy is evaluated during
   maintenance runs, automatically removing snapshots that exceed the defined
   retention limits. This ensures storage efficiency without manual intervention.

Now, deploy the ``kopia-config`` followed by ``ReplicationSource`` configuration.

.. code-block:: console

   $ kubectl create -f examples/kopia/source-kopia/source-kopia.yaml -n source
   $ kubectl create -f examples/kopia/volsync_v1alpha1_replicationsource.yaml -n source

To verify the replication has completed, view the ReplicationSource
``.status`` field.

.. code-block:: console

   $ kubectl -n source get ReplicationSource/database-source -oyaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-source
     namespace: source
   spec:
     # ... lines omitted ...
   status:
     conditions:
     - lastTransitionTime: "2024-01-15T18:16:35Z"
       message: Reconcile complete
       reason: ReconcileComplete
       status: "True"
       type: Reconciled
     lastSyncDuration: 2m45.123456789s
     lastSyncTime: "2024-01-15T18:19:45Z"
     nextSyncTime: "2024-01-15T18:30:00Z"
     kopia:
       lastMaintenance: "2024-01-15T12:00:00Z"

In the above output, the ``lastSyncTime`` shows the time when the last backup
completed, and ``lastMaintenance`` shows when maintenance was last run. The
maintenance operation enforces retention policies, removing old snapshots
according to the defined retention rules.

-----------------------------------------

The backup created by VolSync can be seen by directly accessing the Kopia
repository:

.. code-block:: console

   # In one window, create a port forward to access the minio server
   $ kubectl port-forward --namespace minio svc/minio 9000:9000

   # In another, access the repository with kopia via the above forward
   $ export AWS_ACCESS_KEY_ID=access
   $ export AWS_SECRET_ACCESS_KEY=password
   $ export KOPIA_PASSWORD=my-secure-kopia-password
   $ kopia repository connect s3 --bucket=kopia-repo --endpoint=http://127.0.0.1:9000
   $ kopia snapshot list
   
   Snapshots:
   
   2024-01-15 18:19:45 UTC k8s-volsync@cluster 01234567890abcdef Path: /data Size: 1.2 GB

There is a snapshot in the kopia repository created by the kopia data mover.

Advanced Policy Configuration (Future Enhancement)
===================================================

.. warning::
   External policy file configuration requires mounting policy files via ConfigMap or Secret.
   The following example shows the planned functionality. Currently, use inline
   configuration options (retain, compression, actions) in the ReplicationSource spec.

For future complex policy requirements (not yet available):

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: database-kopia-policies
     namespace: source
   data:
     global-policy.json: |
       {
         "retention": {
           "keepLatest": 10,
           "keepHourly": 48,
           "keepDaily": 30,
           "keepWeekly": 8,
           "keepMonthly": 24,
           "keepAnnual": 5
         },
         "compression": {
           "compressor": "zstd",
           "minSize": 1024,
           "maxSize": 20971520
         },
         "actions": {
           "beforeSnapshotRoot": [
             {
               "mode": "essential",
               "script": "/scripts/pre-backup.sh",
               "timeout": 300
             }
           ],
           "afterSnapshotRoot": [
             {
               "mode": "async",
               "script": "/scripts/post-backup.sh"
             }
           ]
         },
         "scheduling": {
           "intervalSeconds": 3600,
           "timesOfDay": ["02:00", "14:00", "22:00"]
         },
         "errorHandling": {
           "ignoreFileErrors": true,
           "ignoreDirectoryErrors": false
         },
         "files": {
           "ignore": [
             "*.tmp",
             "*.swp",
             "lost+found/",
             ".Trash*/"
           ],
           "dotFiles": "include",
           "oneFileSystem": true
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "permittedActions": [
           "beforeSnapshotRoot",
           "afterSnapshotRoot"
         ]
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-source-advanced
     namespace: source
   spec:
     sourcePVC: mysql-pv-claim
     trigger:
       schedule: "0 */2 * * *"  # Every 2 hours
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: database-kopia-policies
         globalPolicyFilename: "global-policy.json"
         repositoryConfigFilename: "repository.config"
       copyMethod: Clone

**External Policy Benefits:**

- **Fine-grained Control**: Access to all Kopia policy settings
- **Complex Scheduling**: Define multiple backup times per day
- **Advanced Filtering**: Exclude specific file patterns from backups
- **Error Handling**: Configure how to handle backup errors
- **Action Modes**: Control action execution (essential, async, optional)
- **Size-based Compression**: Only compress files within specific size ranges

.. note::
   **Current Status**: External policy files via ConfigMap/Secret are not yet implemented.
   Use inline configuration options in the ReplicationSource spec for retention policies,
   compression settings (at repository creation), and snapshot actions.

Restoring the backup
====================

To restore from the backup, create a destination, deploy ``kopia-config`` and
``ReplicationDestination`` on the destination.

.. code-block:: console

   $ kubectl create ns dest
   $ kubectl annotate namespace dest volsync.backube/privileged-movers="true"
   $ kubectl -n dest create -f examples/kopia/source-kopia/

To start the restore, create an empty PVC for the data:

.. code-block:: console

   $ kubectl -n dest create -f examples/source-database/mysql-pvc.yaml
   persistentvolumeclaim/mysql-pv-claim created

Create the ReplicationDestination in the ``dest`` namespace to restore the data:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: database-destination
     namespace: dest
   spec:
     trigger:
       manual: restore
     kopia:
       destinationPVC: mysql-pv-claim
       repository: kopia-config
       copyMethod: Direct
       # ⚠️ sourceIdentity REQUIRED because this is a cross-namespace restore
       # (dest namespace ≠ source namespace)  
       # For same-namespace restores with matching names, sourceIdentity is optional
       sourceIdentity:
         sourceName: database-source  # Source ReplicationSource name
         sourceNamespace: source      # Source namespace (different from dest)
         # sourcePVCName is auto-discovered from the ReplicationSource

.. code-block:: console

   $ kubectl -n dest create -f examples/kopia/volsync_v1alpha1_replicationdestination.yaml

Once the restore is complete, the ``.status.lastManualSync`` field will match
``.spec.trigger.manual``.

To verify restore, deploy the MySQL database to the ``dest`` namespace which will use the data that has
been restored from sourcePVC backup.

Create the Deployment, Service, and Secret.

.. code-block:: console

   $ kubectl create -n dest -f examples/destination-database/mysql-secret.yaml
   $ kubectl create -n dest -f examples/destination-database/mysql-deployment.yaml
   $ kubectl create -n dest -f examples/destination-database/mysql-service.yaml

Validate that the mysql pod is running within the environment.

.. code-block:: console

   $ kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the synced database
exists.

.. code-block:: console

   $ kubectl exec --stdin --tty -n dest $(kubectl get pods -n dest | grep mysql | awk '{print $1}') -- /bin/bash
   $ mysql -u root -p$MYSQL_ROOT_PASSWORD
   > show databases;
   +--------------------+
   | Database           |
   +--------------------+
   | information_schema |
   | mysql              |
   | performance_schema |
   | synced             |
   | sys                |
   +--------------------+
   5 rows in set (0.00 sec)

   > exit
   $ exit

.. note::
   If the ``beforeSnapshot`` action created a SQL dump file, you may also find
   ``mysql-backup.sql`` in the restored data. This dump can be used as an
   additional recovery option or imported into a fresh database instance.

.. _Shared S3 Bucket Notes:

===============================================
Backing up multiple PVCs to the same S3 bucket
===============================================

If using the same S3 bucket for multiple backups, then be aware of the following:

- Each PVC to be backed up will need its own separate ``kopia-config`` secret.
- Each ``kopia-config`` secret may use the same s3 bucket name in the KOPIA_REPOSITORY, but
  they must each have a unique path underneath.

Example of backing up 2 PVCs, ``pvc-a`` and ``pvc-b``:
=========================================================

A ``kopia-config`` and ``replicationsource`` needs to be created for each pvc and each replicationsource
must refer to the correct ``kopia-config``.

For ``pvc-a``:

.. code-block:: yaml

   ---
   # Kopia-config Secret for pvc-a
   apiVersion: v1
   kind: Secret
   metadata:
      name: kopia-config-a
   type: Opaque
   stringData:
      # The repository url with pvc-a-backup as the subpath under the kopia-repo bucket
      KOPIA_REPOSITORY: s3://kopia-repo/pvc-a-backup
      # The repository encryption password
      KOPIA_PASSWORD: my-secure-kopia-password-pvc-a
      # S3 credentials
      AWS_ACCESS_KEY_ID: access
      AWS_SECRET_ACCESS_KEY: password
      # S3 endpoint (required for non-AWS S3)
      AWS_S3_ENDPOINT: http://minio.minio.svc.cluster.local:9000

   ---
   # ReplicationSource for pvc-a
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
      name: replication-source-pvc-a
      namespace: source
   spec:
      sourcePVC: pvc-a
      trigger:
         schedule: "*/30 * * * *"
      kopia:
        maintenanceIntervalDays: 7
        repository: kopia-config-a
        retain:
          daily: 7
          weekly: 4
          monthly: 6
          yearly: 1
        compression: zstd
        parallelism: 2
        copyMethod: Clone

For ``pvc-b``:

.. code-block:: yaml

   ---
   # Kopia-config Secret for pvc-b
   apiVersion: v1
   kind: Secret
   metadata:
      name: kopia-config-b
   type: Opaque
   stringData:
      # The repository url with pvc-b-backup as the subpath under the kopia-repo bucket
      KOPIA_REPOSITORY: s3://kopia-repo/pvc-b-backup
      # The repository encryption password - using a different key from pvc-a for additional security
      KOPIA_PASSWORD: my-secure-kopia-password-pvc-b
      # S3 credentials
      AWS_ACCESS_KEY_ID: access
      AWS_SECRET_ACCESS_KEY: password
      # S3 endpoint (required for non-AWS S3)
      AWS_S3_ENDPOINT: http://minio.minio.svc.cluster.local:9000

   ---
   # ReplicationSource for pvc-b
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
      name: replication-source-pvc-b
      namespace: source
   spec:
      sourcePVC: pvc-b
      trigger:
         schedule: "*/30 * * * *"
      kopia:
        maintenanceIntervalDays: 7
        repository: kopia-config-b
        retain:
          daily: 7
          weekly: 4
          monthly: 6
          yearly: 1
        compression: zstd
        parallelism: 2
        copyMethod: Clone

.. note::
   Unlike some other backup tools, Kopia supports multiple clients safely writing
   to the same repository path. However, for organizational purposes and better
   isolation, it's still recommended to use separate paths for different PVCs.

Kopia Advantages for Database Backups
======================================

Kopia provides several advantages for database backups:

**Consistency Actions**: The ``beforeSnapshot`` and ``afterSnapshot`` actions ensure
database consistency without requiring application downtime.

**Efficient Compression**: Kopia's zstd compression typically achieves better compression
ratios than traditional backup tools, reducing storage costs.

**Incremental Backups**: Kopia's content-defined chunking provides efficient incremental
backups that only transfer changed data blocks.

**Concurrent Access**: Multiple backup sources can safely write to the same repository,
making it easier to manage centralized backup infrastructure.

**Fast Restores**: Kopia's architecture enables fast partial and full restores without
needing to download entire backup archives.