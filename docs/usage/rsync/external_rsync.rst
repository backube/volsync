==============
External Rsync
==============
A situation may occur where data needs to be imported into a Kubernetes environment.
In the Scribe repository, the script `bin/external-rsync-source` can be executed
which will serve as the `replicationsource` allowing data to be replicated to a
Kubernetes cluster.


Usage
=====
This binary works by using Rsync and SSH to copy a directory into an endpoint
within a Kubernetes cluster created by the `replicationdestination` object. Because
of the simplicity the underlying storage does not matter. This means the directory
could exist on a NFS share, within GlusterFS, or on the servers filesystem.

The binary requires specific flags to be provided.

- -d Destination address to rsync the data
- -i Path to the SSH Key
- -s Source Directory

An example usage of the script would be to copy the `/var/www/html` directory to the
LoadBalancer service created by the `replicationdestination`.

.. code:: bash

   $ external-rsync-source -s /var/www/html -d a48a38bf6f69c4070831391e8b22e8d5-08027986c9de8c10.elb.us-east-2.amazonaws.com -i /home/user/source-key

Example
======

In this example, a database is running on a RHEL 8 server. It has been decided
that this database should move from a server into a Kubernetes environment.

Starting at the Kubernetes cluster create a file with the YAML below. The
`replicationdestination` will use the `gp2 storageclass` for the PVC.

Create the namespace, PVC, and `replicationdestination` object.

.. code:: bash

   $ kubectl create ns database
   $ kubectl create -f examples/external-database/replicationdestination.yaml
   $ kubectl create -f examples/external-database/mysql-pvc.yaml


This will geneate a LoadBalancer service which will be used by our binary as our
destination address.

.. code:: bash

   $ kubectl get replicationdestination database-destination -n database --template={{.status.rsync.address}}

The `replicationdestination` created an SSH key to be used on the server.

Acquire the private key by running the following.

.. code:: bash

   $ kubectl get secret -n database scribe-rsync-dest-src-database-destination --template {{.data.source}} | base64 -d > ~/replication-key
   $ chmod 0600 ~/replication-key

From the server, run the `external-rsync-source` binary specifying the Loadbalancer, SSH private key, and MySQL directory.

.. code:: bash

   $ ./external-rsync-source -i ~/replication-key -s /var/lib/mysql/ -d a48a38bf6f69c4070831391e8b22e8d5-08027986c9de8c10.elb.us-east-2.amazonaws.com

At the Kubernetes cluster we can now create our database deployment.

.. code:: bash

   $ kubectl create -f examples/external-database/mysql-deployment.yaml


Now that the MySQL deployment is running, verify the expected databases exist within the Kubernetes cluster. When logging
into the database the password and authentication values were copied over from the database running on the RHEL server.

.. code:: bash

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
