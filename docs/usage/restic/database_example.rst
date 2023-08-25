=======================
Restic Database Example
=======================

Restic backup
=============

`Restic <https://restic.readthedocs.io/>`_ is a fast and secure backup program.
The following example will use Restic to create a backup of a source volume.

A MySQL database will be used as the example application.

Creating source PVC to be backed up
-----------------------------------

Create a namespace called ``source``

.. code-block:: console

   $ kubectl create ns source
   $ kubectl annotate namespace source volsync.backube/privileged-movers="true"

.. note::
    The second command to annotate the namespace is used to enable the restic data mover to run in privileged mode.
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

   $ kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash

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

Restic Repository Setup
-----------------------

For the purpose of this tutorial we are using minio as the object storage target
for the backup.

Start ``minio``:

.. code-block:: console

   $ hack/run-minio.sh

The ``restic-config`` Secret configures the Restic repository parameters:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: Secret
   metadata:
      name: restic-config
   type: Opaque
   stringData:
      # The repository url
      RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo
      # The repository encryption key
      RESTIC_PASSWORD: my-secure-restic-password
      # ENV vars specific to the back end
      # https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html
      AWS_ACCESS_KEY_ID: access
      AWS_SECRET_ACCESS_KEY: password

The above will backup to a bucket called ``restic-repo``. If the same bucket will
be used for different PVCs or different uses, then make sure to specify a unique
path under ``restic-repo``. See `Shared S3 Bucket Notes`_ for more detail.

ReplicationSource
------------------

Start by configuring the source; a minimal example is shown below

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
      restic:
        pruneIntervalDays: 15
        repository: restic-config
        retain:
          hourly: 1
          daily: 1
          weekly: 1
          monthly: 1
          yearly: 1
        copyMethod: Clone

In the above ``ReplicationSource`` object,

- The PiT copy of the source data ``mysql-pv-claim`` will be created by cloning
  the source volume.
- The synchronization schedule, ``.spec.trigger.schedule``, is defined by a
  `cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule
  very flexible. In this case, it will take a backup every 30 minutes.
- The restic repository configuration is provided via the ``restic-config``
  Secret.
- ``pruneIntervalDays`` defines the interval between Restic prune operations.
- The ``retain`` settings determine how many backups should be saved in the
  repository. Read more about `restic forget
  <https://restic.readthedocs.io/en/stable/060_forget.html?highlight=forget#removing-snapshots-according-to-a-policy>`_.

Now, deploy the ``restic-config`` followed by ``ReplicationSource`` configuration.


.. code-block:: console

   $ kubectl create -f examples/restic/source-restic/source-restic.yaml -n source
   $ kubectl create -f examples/restic/volsync_v1alpha1_replicationsource.yaml -n source

To verify the replication has completed, view the the ReplicationSource
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
     - lastTransitionTime: "2021-05-17T18:16:35Z"
       message: Reconcile complete
       reason: ReconcileComplete
       status: "True"
       type: Reconciled
     lastSyncDuration: 3m10.261673933s
     lastSyncTime: "2021-05-17T18:19:45Z"
     nextSyncTime: "2021-05-17T18:30:00Z"
     restic: {}

In the above output, the ``lastSyncTime`` shows the time when the last backup
completed.

-----------------------------------------

The backup created by VolSync can be seen by directly accessing the Restic
repository:

.. code-block:: console

   # In one window, create a port forward to access the minio server
   $ kubectl port-forward --namespace minio svc/minio 9000:9000

   # An another, access the repository w/ restic via the above forward
   $ AWS_ACCESS_KEY_ID=access AWS_SECRET_ACCESS_KEY=password restic -r s3:http://127.0.0.1:9000/restic-repo snapshots
   enter password for repository:
   repository 03fd0c91 opened successfully, password is correct
   created new cache in /home/jstrunk/.cache/restic
   ID        Time                 Host        Tags        Paths
   ------------------------------------------------------------
   caebaa8e  2021-05-17 14:19:42  volsync                  /data
   ------------------------------------------------------------
   1 snapshots

There is a snapshot in the restic repository created by the restic data mover.

Restoring the backup
====================

To restore from the backup, create a destination, deploy ``restic-config`` and
``ReplicationDestination`` on the destination.

.. code-block:: console

   $ kubectl create ns dest
   $ kubectl annotate namespace dest volsync.backube/privileged-movers="true"
   $ kubectl -n dest create -f examples/restic/source-restic/

To start the restore, create a empty PVC for the data:

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
   spec:
     trigger:
       manual: restore
     restic:
       destinationPVC: mysql-pv-claim
       repository: restic-config
       copyMethod: Direct


.. code-block:: console

   $ kubectl -n dest create -f examples/restic/volsync_v1alpha1_replicationdestination.yaml

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

   $ kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` -- /bin/bash
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

.. _Shared S3 Bucket Notes:

===============================================
Backing up multiple PVCs to the same S3 bucket
===============================================

If using the same S3 bucket for multiple backups, then be aware of the following:

- Each PVC to be backed up will need its own separate ``restic-config`` secret.
- Each ``restic-config`` secret may use the same s3 bucket name in the RESTIC_REPOSITORY, but
  they must each have a unique path underneath.

Example of backing up 2 PVCs, ``pvc-a`` and ``pvc-b``:
=========================================================

A ``restic-config`` and ``replicationsource`` needs to be created for each pvc and each replicationsource
must refer to the correct ``restic-config``.

For ``pvc-a``:

.. code-block:: yaml

   ---
   # Restic-config Secret for pvc-a
   apiVersion: v1
   kind: Secret
   metadata:
      name: restic-config-a
   type: Opaque
   stringData:
      # The repository url with pvc-a-backup as the subpath under the restic-repo bucket
      RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo/pvc-a-backup
      # The repository encryption key
      RESTIC_PASSWORD: my-secure-restic-password-pvc-a
      # ENV vars specific to the back end
      # https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html
      AWS_ACCESS_KEY_ID: access
      AWS_SECRET_ACCESS_KEY: password

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
      restic:
        pruneIntervalDays: 15
        repository: restic-config-a
        retain:
          hourly: 1
          daily: 1
          weekly: 1
          monthly: 1
          yearly: 1
        copyMethod: Clone

For ``pvc-b``:

.. code-block:: yaml

   ---
   # Restic-config Secret for pvc-b
   apiVersion: v1
   kind: Secret
   metadata:
      name: restic-config-b
   type: Opaque
   stringData:
      # The repository url with pvc-b-backup as the subpath under the restic-repo bucket
      RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo/pvc-b-backup
      # The repository encryption key - using a different key from pvc-a.  This will not prevent overwrites
      # or deletes of the data for others who have access to the bucket, but will prevent reads/writes
      # to the restic data in the pvc-b-backup folder for those without this encryption key.
      RESTIC_PASSWORD: my-secure-restic-password-pvc-b
      # ENV vars specific to the back end
      # https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html
      AWS_ACCESS_KEY_ID: access
      AWS_SECRET_ACCESS_KEY: password

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
      restic:
        pruneIntervalDays: 15
        repository: restic-config-b
        retain:
          hourly: 1
          daily: 1
          weekly: 1
          monthly: 1
          yearly: 1
        copyMethod: Clone
