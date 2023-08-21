=======================
Rclone Database Example
=======================

The following example will use the Rclone replication method to replicate a sample MySQL database.

First, create the source namespace and deploy the source MySQL database.

.. code:: console

   $ kubectl create ns source
   $ kubectl annotate namespace source volsync.backube/privileged-movers="true"

.. note::
    The second command to annotate the namespace is used to enable the rclone data mover to run in privileged mode.
    This is because this simple example runs MySQL as root. For your own applications, you can run unprivileged by
    setting the ``moverSecurityContext`` in your ReplicationSource/ReplicationDestination to match that of your
    application in which case the namespace annotation will not be required. See the
    :doc:`permission model documentation </usage/permissionmodel>` for more details.

Deploy the source MySQL database.

.. code:: console

   $ kubectl create -f examples/source-database/ -n source

Verify the database is running.

.. code:: console

   $ kubectl get pods -n source
   NAME                    READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-24w6g   1/1     Running   0          17s

Add a new database.

.. code:: console

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


Now edit ``examples/rclone/rclone.conf`` with your rclone configuration, or you can deploy minio as object-storage to use
with the examples.

To start minio in your cluster, run:

.. code:: console

   $ hack/run-minio.sh

If using minio then you can edit ``examples/rclone/rclone.conf`` to match the following:

.. code-block:: none
    :caption: rclone.conf for use with local minio

    [rclone-bucket]
    type = s3
    provider = Minio
    env_auth = false
    access_key_id = access
    secret_access_key = password
    region = us-east-1
    endpoint = http://minio.minio.svc.cluster.local:9000

Now, deploy the ``rclone-secret`` followed by ``ReplicationSource`` configuration.

.. code:: console

   $ kubectl create secret generic rclone-secret --from-file=rclone.conf=./examples/rclone/rclone.conf -n source
   $ kubectl create -f examples/rclone/volsync_v1alpha1_replicationsource.yaml -n source

To verify the replication has completed describe the Replication source.

.. code:: console

   $ kubectl describe ReplicationSource -n source database-source

From the output, the success of the replication can be seen by the following
lines:

.. code:: console

 Status:
  Conditions:
    Last Transition Time:  2021-01-18T21:50:59Z
    Message:               Reconcile complete
    Reason:                ReconcileComplete
    Status:                True
    Type:                  Reconciled
  Next Sync Time:          2021-01-18T22:00:00Z

At ``Next Sync Time`` VolSync will create the next Rclone data mover job. 

-----------------------------------------

To complete the replication, create a destination, deploy ``rclone-secret`` and ``ReplicationDestination``
on the destination.

.. code:: console

   $ kubectl create ns dest
   $ kubectl annotate namespace dest volsync.backube/privileged-movers="true"
   $ kubectl create secret generic rclone-secret --from-file=rclone.conf=./examples/rclone/rclone.conf -n dest
   $ kubectl create -f examples/rclone/volsync_v1alpha1_replicationdestination.yaml -n dest



Once the ``ReplicationDestination`` is deployed, VolSync will create a Rclone data mover job on the
destination side. At the end of the each successful iteration, the ``ReplicationDestination`` is
updated with the latest snapshot image.

Now deploy the MySQL database to the ``dest`` namespace which will use the data that has been replicated.

The PVC uses the VolSync volume populator feature and sets the ReplicationDestination
as its dataSourceRef. This will populate the PVC with the latest snapshot contents from the ReplicationDestination.

Create the Deployment, Service, PVC, and Secret.

.. code:: console

   # Start the database
   $ kubectl create -n dest -f examples/destination-database/

Validate that the mysql pod is running within the environment.

.. code:: console

   $ kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the ``synced`` database
exists.

.. code:: console

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
