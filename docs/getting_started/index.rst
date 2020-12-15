===============
Getting Started
===============
The following directions will walk through the process of deploying Scribe and using the rsync functionality.
In the future, this document will also contain steps for the rclone functionality.

Deploying
=========
Once the repository has been cloned run the following within the scribe directory to deploy the required
components.

Volume SnapShot capabilities are required for Scribe. If you are using Kind to develop functionality for
Scribe or another Kubernetes provider ensure that you are using a CSI capable storageclass.

Running Scribe on OpenShift requires some additional steps. Follow the steps to deploy Scribe
on `OpenShift  <https://scribe-replication.readthedocs.io/en/latest/openshift/index.html>`_


If you are wanting try scribe run the make command to deploy the objects into
the cluster. Before starting ensure that your KUBECONFIG is set to the cluster you want to install Scribe.

.. code-block:: bash

   make deploy

Verify Scribe is running by checking the output of kubectl describe.

.. code-block:: bash

   kubectl describe pods -n scribe-system


If you are developing and adding functionality to scribe run the following as it will output the logs of
the controller to your terminal.

.. code-block:: bash

   make install
   make run

Understanding Rsync Data Flow
=============================
The source and destination CustomResources support a *CopyMethod* field which will indicate how the data is to
be delivered and received. Scribe supports the underlying CSI methods for Snapshots and Clones.

Destination copyMethod:
-----------------------

Destination receives incoming source data as a PVC and the copyMethod will determine what happens once the source data
is received.

.. code-block:: yaml

  spec:
    rsync:
      serviceType: ClusterIP
      copyMethod: Snapshot
      capacity: 2Gi
      accessModes: [ReadWriteOnce]


The copyMethod supports two values on Destination side:

- **Snapshot** - Data is received from source as a PVC and if copyMethod is Snapshot then Scribe will take a
  VolumeSnapshot of the source PVC and this now becomes the latestImage in the CustomResource.

- **None** - data is received from the source as a PVC and if copyMethod is None, then Scribe will take no action on
  the PVC and it will become the latestImage in the CustomResource.


Source copyMethod:
------------------
Source will create a copy of the source volume and transfer the data to the destination as a PVC.

.. code-block:: yaml

  spec:
    sourcePVC: mysql-pv-claim
    trigger:
      schedule: "*/3 * * * *"
    rsync:
      sshKeys: scribe-rsync-dest-src-database-destination
      address: my.host.com
      copyMethod: Clone


The copyMethod supports three values on the Source side:

* **Snapshot** - will use the VolumeSnapshot capabilities of CSI and create a point in time copy of the PVC.
* **Clone** - will use the VolumeClone capabilities of CSI and create a copy of the PVC
* **None** - will not use any CSI capabilities (no VolumeSnapshot or VolumeClone) and simply copy the volume as is.

.. _using rsync:

Using Rsync
===========
The following example will use the rsync replication method and take a Snapshot at the destination.
A MySQL database will be used as the example application.

First, create the destination and deploy the ReplicationDestination configuration.

.. code-block:: bash

   kubectl create ns dest
   kubectl create -n dest -f examples/scribe_v1alpha1_replicationdestination.yaml

A service is created which will be used by the ReplicationSource to rsync the data. Record
the service IP address as it will be used for the ReplicationSource.

.. code-block:: bash

   kubectl get replicationdestination database-destination -n dest --template={{.status.rsync.address}}
   10.107.249.72

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

Using the IP address that relates to the ReplicationDestination that was recorded earlier. Modify
*scribe_v1alpha1_replicationsource.yaml* replacing the value of the address and create the ReplicationSource object.

.. code-block:: bash

   sed -i 's/my.host.com/10.107.249.72/g' examples/scribe_v1alpha1_replicationsource.yaml
   kubectl create -n source -f examples/scribe_v1alpha1_replicationsource.yaml

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

Create a databases in the mysql pod running in the source namespace.

.. code-block:: bash

   kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` /bin/bash
   mysql -u root -p$MYSQL_ROOT_PASSWORD
   show databases;
   +--------------------+
   | Database           |
   +--------------------+
   | information_schema |
   | mysql              |
   | performance_schema |
   | sys                |
   +--------------------+
   4 rows in set (0.00 sec)


   create database synced;
   exit
   exit

Now the mysql database will be deployed to the dest namespace which will use the data that has been replicated.
First we need to identify the latest snapshot from the replicationdestination object.
Record the values of the latest snapshot as it will be used to create a pvc. Then create
the deployment, service, pvc, and secret. Ensure the Snapshots Age is not greater than 3 minutes as it will be replaced
by scribe before it can be used.

.. code-block:: bash

   kubectl get replicationdestination database-destination -n dest --template={{.status.latestImage.name}}
   sed -i 's/snapshotToReplace/scribe-dest-database-destination-20201203174504/g' examples/destination-database/mysql-pvc.yaml
   kubectl create -n dest -f examples/destination-database/

Validate that the mysql pod is running within the environment.

.. code-block:: bash

   kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the synced database exists.

.. code-block:: bash

   kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` /bin/bash
   mysql -u root -p$MYSQL_ROOT_PASSWORD
   show databases;
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

.. _using rclone:

Using Rclone
============

WIP - Place Holder for