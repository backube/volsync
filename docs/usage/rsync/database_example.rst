======================
Rsync Database Example
======================

The following example will use the Rsync replication method to periodically
replicate a MySQL database.

First, create the destination Namespace and deploy the ReplicationDestination
object.

.. code:: console

   $ kubectl create ns dest
   $ kubectl create -n dest -f dest.yaml

The ReplicationDestination has the following configuration:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: database-destination
   spec:
     rsync:
       serviceType: LoadBalancer
       copyMethod: Snapshot
       capacity: 2Gi
       accessModes: [ReadWriteOnce]
       storageClassName: standard-csi
       volumeSnapshotClassName: csi-gce-pd-vsc

A LoadBalancer Service is created by VolSync which will be used by the
ReplicationSource to connect to the destination. Record the service IP address
as it will be used in the ReplicationSource configuration.

.. code:: console

   $ kubectl get replicationdestination database-destination -n dest --template={{.status.rsync.address}}
   34.133.219.189

Now it is time to deploy our database.

.. code:: console

   $ kubectl create ns source
   $ kubectl create -n source -f examples/source-database

Verify the database is running.

.. code:: console

   $ kubectl get pods -n source
   NAME                    READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-24w6g   1/1     Running   0          17s

Now create the ReplicationSource items. First, we need the ssh
secret from the destination namespace.

.. code:: console

   # Retrieve the Secret from the destination cluster
   $ kubectl get secret -n dest volsync-rsync-dest-src-database-destination -o yaml > /tmp/secret.yaml

   # Remove unnecessary fields
   $ vi /tmp/secret.yaml
   # ^^^ change the namespace to "source"
   # ^^^ remove the owner reference (.metadata.ownerReferences)

   # Insert the Secret into the source cluster
   $ kubectl create -f /tmp/secret.yaml

Using the IP address that relates to the ReplicationDestination that was
recorded earlier. Create a ReplicationSource object:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-source
     namespace: source
   spec:
     sourcePVC: mysql-pv-claim
     trigger:
       # Replicate every 5 minutes
       schedule: "*/5 * * * *"
     rsync:
       # The name of the Secret we just created
       sshKeys: volsync-rsync-dest-src-database-destination
       # The LoadBalancer address from the ReplicationDestination
       address: 34.133.219.189
       copyMethod: Clone

Note: You may need to change the ``copyMethod`` to ``Snapshot`` and specify both
a ``storageClassName`` and ``volumeSnapshotClassName``, depending on your CSI
driver's capabilities.

Once the ReplicationSource is created, the initial synchronization should begin.
To verify the replication has completed describe the Replication source.

.. code:: console

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
     Next Sync Time:          2020-12-03T16:10:00Z

We will modify the source database by creating an additional database in the
mysql pod running in the source namespace.

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

During the next synchronization iteration, these updates will be replicated to
the destination.

Now the mysql database will be deployed to the destination namespace which will use the
data that has been replicated.

First we need to identify the latest snapshot from the ReplicationDestination
object. Record the values of the latest snapshot as it will be used to create a
pvc. Then create the Deployment, Service, PVC, and Secret. Ensure that the above
steps are completed before a new replication cycle starts or the latest snapshot
may be replaced before it can be used.

.. code:: console

   $ kubectl get replicationdestination database-destination -n dest --template={{.status.latestImage.name}}
   volsync-dest-database-destination-20201203174504

   $ sed -i 's/snapshotToReplace/volsync-dest-database-destination-20201203174504/g' examples/destination-database/mysql-pvc.yaml
   $ kubectl create -n dest -f examples/destination-database/

Validate that the mysql pod is running within the environment.

.. code:: console

   $ kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the synced database
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
