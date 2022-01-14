====================================
Rsync Database Cross-Cluster Example
====================================

The following example will use the Rsync replication method and take a Snapshot
at the destination. A MySQL database will be used as the example application.

The examples will assume 2 clusters. Data will be synced from cluster-name :code:`source-cluster`
in namespace :code:`source` to cluster-name :code:`dest-cluster` in namespace :code:`dest`.

.. note::
    * :doc:`Clusters must have the volsync operator installed </installation/index>`.
    * :doc:`Cluster storage may need configuring </installation/index>`.

First, create the destination and deploy the ReplicationDestination
configuration.

On the destination cluster :code:`dest-cluster`:

.. code:: bash

   $ kubectl create ns dest
   $ kubectl create -n dest -f examples/rsync/volsync_v1alpha1_replicationdestination_remotecluster.yaml

A Load Balancer Service is created which will be used by the ReplicationSource to Rsync the
data. Record the Load Balancer address as it will be used for the
ReplicationSource.  Also record the name of the secret as it will need to be copied over to the source cluster.

.. code:: bash

   $ ADDRESS=`kubectl get replicationdestination database-destination -n dest --template={{.status.rsync.address}}`
   $ echo $ADDRESS
   a83126a5a50e64f81b3a46f9e4a02eb2-5592c0b3d94dd376.elb.us-east-1.amazonaws.com

   $ SSHKEYS=`kubectl get replicationdestination database-destination -n dest  --template={{.status.rsync.sshKeys}}`
   $ echo $SSHKEYS
   volsync-rsync-dst-src-database-destination


Now it is time to deploy our database.

On the source cluster :code:`source-cluster`:

.. code:: bash

   $ kubectl create ns source
   $ kubectl create -n source -f examples/source-database

Verify the database is running.

.. code:: bash

   $ kubectl get pods -n source
   NAME                    READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-24w6g   1/1     Running   0          17s

Now it is time to create the ReplicationSource items. First, we need the ssh
secret from the destination cluster.

On the destination cluster :code:`dest-cluster`:

.. code:: bash

   $ kubectl get secret -n dest $SSHKEYS -o yaml > /tmp/secret.yaml
   $ vi /tmp/secret.yaml
   # ^^^ change the namespace to "source"
   # ^^^ remove the owner reference (.metadata.ownerReferences)


On the source cluster :code:`source-cluster`:

.. code:: bash

   $ kubectl create -f /tmp/secret.yaml

Using the Load Balancer address and secret name that relates to the ReplicationDestination that was
recorded earlier, modify ``volsync_v1alpha1_replicationsource_remotecluster.yaml`` replacing
the value of the address and sshKeys and create the ReplicationSource object.

.. code:: bash

   $ sed -i "s/my.host.com/$ADDRESS/g" examples/rsync/volsync_v1alpha1_replicationsource_remotecluster.yaml
   $ sed -i "s/mysshkeys/$SSHKEYS/g" examples/rsync/volsync_v1alpha1_replicationsource_remotecluster.yaml
   $ kubectl create -n source -f examples/rsync/volsync_v1alpha1_replicationsource_remotecluster.yaml

To verify the replication has completed describe the Replication source.

.. code:: bash

   $ kubectl describe ReplicationSource -n source database-source

From the output, the success of the replication can be seen by the following
lines:

.. code:: bash

  Status:
    Conditions:
      Last Transition Time:  2021-10-14T20:48:00Z
      Message:               Synchronization in-progress
      Reason:                SyncInProgress
      Status:                True
      Type:                  Synchronizing
      Last Transition Time:  2021-10-14T20:41:41Z
      Message:               Reconcile complete
      Reason:                ReconcileComplete
      Status:                True
      Type:                  Reconciled
    Last Sync Duration:      5m20.764642395s
    Last Sync Time:          2021-10-14T20:47:01Z
    Next Sync Time:          2021-10-14T20:48:00Z

The Last Sync Time should be filled out, indicating that the last sync completed.

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

On the destination cluster :code:`dest-cluster`:

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
