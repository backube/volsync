=======================
Rclone Database Example
=======================

The following example will use the Rclone replication method and take a Snapshot
at the source. A MySQL database will be used as the example application.

First, create the source. Next deploy the source MySQL database.  


.. code:: bash

   $ kubectl create ns source
   $ kubectl create -f example/source-databases/ -n source

Verify the database is running.

.. code:: bash

   $ kubectl get pods -n source
   NAME                    READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-24w6g   1/1     Running   0          17s

Add a new database.

.. code:: bash

   $ kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` /bin/bash
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
   

Now, deploy the ``rclone-secret`` followed by ``ReplicationSource`` configuration.

.. code:: bash

   $ kubectl create secret generic rclone-secret --from-file=rclone.conf=./examples/rclone.conf -n source
   $ kubectl create -f examples/scribe_v1alpha1_replicationsource_rclone.yaml -n source

To verify the replication has completed describe the Replication source.

.. code:: bash

   $ kubectl describe ReplicationSource -n source database-source

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

At ``Next Sync Time`` Scribe will create the next Rclone data mover job. 

-----------------------------------------

To complete the replication, create a destination, deploy ``rclone-secret`` and ``ReplicationDestination``
on the destination.

.. code:: bash

   $ kubectl create ns dest
   $ kubectl create secret generic rclone-secret --from-file=rclone.conf=./examples/rclone.conf -n dest
   $ create -f examples/scribe_v1alpha1_replicationdestination_rclone.yaml -n dest



Once the ``ReplicationDestination`` is deployed, Scribe will create a Rclone data mover job on the
destination side. At the end of the each successful reconcilation iteration, the ``ReplicationDestination`` is 
updated with the lastest snapshot image.

Now deploy the MySQL database to the ``dest`` namespace which will use the data that has been replicated. 
First we need to identify the latest snapshot from the ``ReplicationDestination`` object. Record the values of 
the latest snapshot as it will be used to create a pvc. Then create the Deployment, Service, PVC,
and Secret. 

Ensure the Snapshots Age is not greater than 3 minutes as it will be replaced by Scribe before it can be used.

.. code:: bash

   $ kubectl get replicationdestination database-destination -n dest --template={{.status.latestImage.name}}
   $ sed -i 's/snapshotToReplace/scribe-dest-database-destination-20201203174504/g' examples/destination-database/mysql-pvc.yaml
   $ kubectl create -n dest -f examples/destination-database/

Validate that the mysql pod is running within the environment.

.. code:: bash

   $ kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the synced database
exists.

.. code:: bash

   $ kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` /bin/bash
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



