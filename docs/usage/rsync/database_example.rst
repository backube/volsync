======================
Rsync Database Example
======================

The following example will use the Rsync replication method and take a Snapshot
at the destination. A MySQL database will be used as the example application.

First, create the destination and deploy the ReplicationDestination
configuration.

.. code:: bash

   $ kubectl create ns dest
   $ kubectl create -n dest -f examples/rsync/volsync_v1alpha1_replicationdestination.yaml

A Service is created which will be used by the ReplicationSource to Rsync the
data. Record the service IP address as it will be used for the
ReplicationSource.

.. code:: bash

   $ kubectl get replicationdestination database-destination -n dest --template={{.status.rsync.address}}
   10.107.249.72

Now it is time to deploy our database.

.. code:: bash

   $ kubectl create ns source
   $ kubectl create -n source -f examples/source-database

Verify the database is running.

.. code:: bash

   $ kubectl get pods -n source
   NAME                    READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-24w6g   1/1     Running   0          17s

Now it is time to create the ReplicationSource items. First, we need the ssh
secret from the dest namespace.

.. code:: bash

   $ kubectl get secret -n dest volsync-rsync-dest-src-database-destination -o yaml > /tmp/secret.yaml
   $ vi /tmp/secret.yaml
   # ^^^ change the namespace to "source"
   # ^^^ remove the owner reference (.metadata.ownerReferences)
   $ kubectl create -f /tmp/secret.yaml

Using the IP address that relates to the ReplicationDestination that was
recorded earlier. Modify ``volsync_v1alpha1_replicationsource.yaml`` replacing
the value of the address and create the ReplicationSource object.

.. code:: bash

   $ sed -i 's/my.host.com/10.107.249.72/g' examples/rsync/volsync_v1alpha1_replicationsource.yaml
   $ kubectl create -n source -f examples/rsync/volsync_v1alpha1_replicationsource.yaml

To verify the replication has completed describe the Replication source.

.. code:: bash

   $ kubectl describe ReplicationSource -n source database-source

From the output, the success of the replication can be seen by the following
lines:

.. code:: bash

   Status:
     Conditions:
       Last Transition Time:  2020-12-03T16:07:35Z
       Message:               Reconcile complete
       Reason:                ReconcileComplete
       Status:                True
       Type:                  Reconciled
     Last Sync Duration:      4.511334577s
     Last Sync Time:          2020-12-03T16:09:04Z
     Next Sync Time:          2020-12-03T16:12:00Z

Create a database in the mysql pod running in the source namespace.

.. code:: bash

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

Now the mysql database will be deployed to the dest namespace which will use the
data that has been replicated. First we need to identify the latest snapshot
from the ReplicationDestination object. Record the values of the latest snapshot
as it will be used to create a pvc. Then create the Deployment, Service, PVC,
and Secret. Ensure the Snapshots Age is not greater than 3 minutes as it will be
replaced by VolSync before it can be used.

.. code:: bash

   $ kubectl get replicationdestination database-destination -n dest --template={{.status.latestImage.name}}
   $ sed -i 's/snapshotToReplace/volsync-dest-database-destination-20201203174504/g' examples/destination-database/mysql-pvc.yaml
   $ kubectl create -n dest -f examples/destination-database/

Validate that the mysql pod is running within the environment.

.. code:: bash

   $ kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the synced database
exists.

.. code:: bash

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
