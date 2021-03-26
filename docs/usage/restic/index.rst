=======================
Restic Database Example
=======================

Restic
------
`Restic <https://restic.readthedocs.io/>`_ is a fast and secure backup program. 
The following example will use the Restic backup and take a Snapshot at the source.

A MySQL database will be used as the example application.

Creating source pvc to be backed up
-----------------------------------

First, create a namespace called ``source``. Next deploy the source MySQL database.  


.. code:: bash

   kubectl create ns source
   kubectl create -f examples/source-database/ -n source

Verify the database is running.

.. code:: bash

   watch oc get pods,pvc,volumesnapshots
   
   NAME                        READY     STATUS    RESTARTS   AGE
   pod/mysql-87f849f8c-n9j7j   1/1       Running   1          58m

   NAME                                   STATUS    VOLUME                                     CAPACITY   ACCESS MODES
   STORAGECLASS	  AGE
   persistentvolumeclaim/mysql-pv-claim   Bound     pvc-adbf57f1-6399-4738-87c9-4c660d982a0f   2Gi        RWO
   csi-hostpath-sc   60m



Add a new database.

.. code:: bash

   kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash
   mysql -u root -p$MYSQL_ROOT_PASSWORD
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
   exit
   
Restic Repository Setup
-----------------------

For the purpose of this tutorial we are using minio as S3 storage.
Start ``minio`` 

.. code:: bash

   hack/run-minio.sh 

   kubectl port-forward --namespace minio svc/minio 9000:9000



Once ``minio`` is up and running, create a restic repository for storing backup snapshots.
The contents of a directory at a specific point in time is called a "snapshot" in restic.

.. code:: bash

   export AWS_ACCESS_KEY_ID=access
   export AWS_SECRET_ACCESS_KEY=password

   restic -r s3:http://127.0.0.1:9000/restic-repo init

On password prompt use the same password as used in setting up the ``restic-config``.
It should match the field ``RESTIC_PASSWORD`` in the secret. 

Now, deploy the ``restic-config`` followed by ``ReplicationSource`` configuration.

.. code:: bash

   kubectl create -f example/source-restic/source-restic.yaml -n source
   kubectl create -f examples/scribe_v1alpha1_replicationsource_restic.yaml -n source

To verify the replication has completed describe the Replication source.

.. code:: bash

   kubectl describe ReplicationSource -n source database-source

From the output, the success of the replication can be seen by the following
lines:

.. code:: bash

 Status:
  Conditions:
    Last Transition Time:  2021-01-18T21:50:59Z
    Message:               Reconcile complete
    Reason:                ReconcileComplete
    Status:                True
    Type:                  Reconciled
  Next Sync Time:          2021-01-18T22:00:00Z

At ``Next Sync Time`` Scribe will create the next Restic data mover job. 

-----------------------------------------

Follow the steps below to verify the backup 

.. code:: bash
   
   restic -r s3:http://127.0.0.1:9000/restic-repo snapshots

   enter password for repository: 
   repository e6f9ccf6 opened successfully, password is correct
   ID        Time                 Host                   Tags        Paths
   ---------------------------------------------------------------------------------------------
   42ec9adb  2021-03-26 11:40:24  scribe                             /data
   ---------------------------------------------------------------------------------------------

There is a snapshot in the restic repository created by the restic data mover.
