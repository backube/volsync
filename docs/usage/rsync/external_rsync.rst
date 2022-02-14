====================================
Moving data into Kubernetes w/ Rsync
====================================
While VolSync is typically used to replicate data between Kubernetes clusters,
it is sometimes necessary to replicate data into a cluster from outside. For
example, when containerizing a previously standalone workload, that
application's data needs to be moved into the cluster and onto a PVC.
This section will walk through the process of pairing a VolSync
ReplicationDestination and an external script to send data into a cluster.

In this configuration, VolSync manages the destination (via a
ReplicationDestination object), but instead of having a VolSync
ReplicationSource as the sender, it will be an external script that plays that
role. It will transmit the data to the destination by initiating the Rsync over
SSH connection directly.

Usage
=====
The helper script, ``./bin/external-rsync-source``, requires the following
parameters:

- ``-d`` Destination address to rsync the data
- ``-i`` Path to the SSH Key
- ``-s`` Source Directory

Migration Example
=================

In this example, a database is running on a RHEL 8 server. It has been decided
that this database should move from a server into a Kubernetes environment.

Starting at the Kubernetes cluster, we create the Namespace,
ReplicationDestination, and the PVC:

.. code:: console

   $ kubectl create ns database
   $ kubectl create -f examples/external-database/replicationdestination.yaml
   $ kubectl create -f examples/external-database/mysql-pvc.yaml

The ReplicationDestination is as follows:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: database-destination
     namespace: database
   spec:
     rsync:
       serviceType: LoadBalancer
       destinationPVC: mysql-pvc
       copyMethod: Direct

Since the have specified a ``serviceType: LoadBalancer``, VolSync will allocate
a LoadBalancer Service to expose the SSH endpoint. We have also specified the
destination PVC directly, via ``destinationPVC``. This tells VolSync not to
allocate a volume, but instead, to use one that has been pre-created. Finally,
we have also specified ``copyMethod: Direct`` which tells VolSync not to
Snapshot the volume at the conclusion of each transfer. We have chosen this
since we are directly syncing the data into its final PVC.

Once the PVC and ReplicationDestination have been created, VolSync will provide
the SSH keys and address of the LoadBalancer in the ``.status.rsync`` fields.

.. code:: console

   # Retrieve the connection address
   $ kubectl get replicationdestination database-destination -n database --template={{.status.rsync.address}}
   a48a38bf6f69c4070831391e8b22e8d5-08027986c9de8c10.elb.us-east-2.amazonaws.com

   # Get the SSH key Secret name
   $ kubectl get replicationdestination database-destination -n database --template={{.status.rsync.sshKeys}}
   volsync-rsync-dest-src-database-destination

   # Retrieve the source private key from the Secret
   $ kubectl get secret -n database volsync-rsync-dest-src-database-destination --template {{.data.source}} | base64 -d > ~/replication-key
   $ chmod 0600 ~/replication-key

Now that we have the address and the key, we can sync data into the cluster by
specifying the directory to sync:

.. code:: console

   $ ./bin/external-rsync-source -i ~/replication-key -s /var/lib/mysql/ -d a48a38bf6f69c4070831391e8b22e8d5-08027986c9de8c10.elb.us-east-2.amazonaws.com

When this completes, all the data underneath the ``/var/lib/mysql`` directory
will be present in the PVC. On the Kubernetes cluster we can now create our
database deployment.

.. code:: console

   $ kubectl create -f examples/external-database/mysql-deployment.yaml

Now that the MySQL deployment is running, verify the expected databases exist within the Kubernetes cluster. When logging
into the database the password and authentication values were copied over from the database running on the RHEL server.

.. code:: console

   $ kubectl exec --stdin --tty -n database `kubectl get pods -n database | grep mysql | awk '{print $1}'` -- /bin/bash
   $ root@mysql-87c47498d-7rc9m:/# mysql -u root -p
   Enter password:
   Welcome to the MySQL monitor.  Commands end with ; or \g.
   Your MySQL connection id is 15
   Server version: 8.0.23 MySQL Community Server - GPL

   Copyright (c) 2000, 2021, Oracle and/or its affiliates.

   Oracle is a registered trademark of Oracle Corporation and/or its
   affiliates. Other names may be trademarks of their respective
   owners.

   Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

   mysql> show databases;
   +--------------------+
   | Database           |
   +--------------------+
   | employees          |
   | information_schema |
   | mysql              |
   | performance_schema |
   | sys                |
   +--------------------+
   5 rows in set (0.01 sec)

   mysql> exit
   Bye
