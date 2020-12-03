===============
Getting Started
===============
The following directions will walk through the process of deploying Scribe and using the rsync functionality.
In the future, this document will also contain steps for the rclone functionality.

Deploying
=========
Once the repository has been cloned run the following within the scribe directory to deploy the required
components.

Volume SnapShot capabilities are required for Scribe. If you are using Kind to develop functionality in
Scribe or to test replication perform the following.

.. code-block bash

   kubectl create -f hack/crds

Run the make command to deploy the objects into the cluster. Before starting ensure that your KUBECONFIG
is set to the cluster you want to install Scribe.

.. code-block:: bash

   make deploy

Verify Scribe is running.

.. code-block:: bash

   kubectl get pods -n scribe-system

Using Rsync
===========
The following example will use the rsync replication method and take a Snapshot at the destination.
A database will be used.

First, create the destination and deploy the ReplicationDestination configuration.
.. code-block:: bash

   kubectl create ns dest
   kubectl create -n dest -f examples/scribe_v1alpha1_replicationdestination.yaml

A service object is created which will be used by the ReplicationSource to rsync the data. Record
the service IP as it will be used for the ReplicationSource.

.. code-block:: bash

   kubectl get svc -n dest
   NAME                                TYPE        CLUSTER-IP      EXTERNAL-IP   PORT(S)   AGE
   scribe-rsync-dest-database-sample   ClusterIP   10.107.249.72   <none>        22/TCP    18s

Now it is time to deploy our database.
.. code-block:: bash

   kubectl create ns source
   kubectl create -n source -f examples/source-database

Verify the database is running.
.. code-block:: bash

   kubectl get pods -n source
   NAME                    READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-24w6g   1/1     Running   0          17s

Now it is time to create the ReplicationSource items. First, we need the ssh secret from the
dest namespace.

.. code-block:: bash

   kubectl get secret -n dest scribe-rsync-dest-src-database-destination -o yaml | sed 's/namespace: dest/namespace: source/g' > /tmp/secret.yaml
   kubectl create -f /tmp/secret.yaml

Using the service object that relates to the ReplicationDestination that was recorded earlier. Modify
*scribe_v1alpha1_replicationsource.yaml* replacing the value of the address and create the ReplicationSource object.

.. code-block:: bash

   sed -i 's/my.host.com/10.107.249.72/g' examples/scribe_v1alpha1_replicationsource.yaml
   oc create -n source -f examples/scribe_v1alpha1_replicationsource.yaml

To verify the replication has completed describe the Replication source.

.. code-block:: bash

   kubectl describe ReplicationSource -n source database-source

From the output, the success of the replication can be seen by the following lines.

.. code-block:: bash

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

The database will be deployed to the dest namespace which will use the data that has been rsync'd.
List the snapshots, the snapshot will be used to create the pvc.

.. code-block:: bash

   kubectl get volumesnapshots -n dest
   sed -i 's/snapshotToReplace/scribe-dest-database-destination-20201203174504/g' examples/destination-database/mysql-pvc.yaml
   kubectl create -n dest -f examples/destination-database/

Create a databases in the source namespace and verify it is copied over to the mysql pod running in
the dest namespace.

.. code-block:: bash

   kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` /bin/bash
   mysql -u root -p$MYSQL_ROOT_PASSWORD
   create database synced;
   exit
   exit

Currently, there is a bug which does not clean up the destination job. This will be resolved shortly. Until then
perform the following.

.. code-block:: bash

   kubectl delete job scribe-rsync-dest-database-destination -n dest