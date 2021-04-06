=============================
Rsync Database Plugin Example
=============================

This example will sync data from mysql database persistent volumes
For this example, sync will happen within a single cluster and 2 namespaces.
Also, the copy method in this example is "None" for both source and destination.
This means data will be synced directly from source PV to dest PV. Because of this,
before the sync, the source deployment must be scaled to 0 to allow scribe to bind
to the PV during replication.

TODO: Add example using SnapShot CopyMethod

.. note::
    * :doc:`Cluster must have the scribe operator installed </installation/index>`.
    * :doc:`Cluster storage may need configuring </installation/index>`.

Build Scribe CLI
----------------

.. code:: bash

    $ make scribe
    $ mv bin/kubectl-scribe /usr/local/bin (or add to $PATH)

Create a scribe-config
----------------------

Create a config file to designate your source and destination options.
You can also pass these individually to each command, but they add up so the
config file is usually a good option. You can add any, some, or all flags
to the config file.

Create the config file at :code:`./config.yaml` *or* :code:`~/.scribeconfig/config.yaml`,
scribe will look for that file in the current directory or in :code:`~/.scribeconfig`.
For complete list of options for a command, run the following or consult the API:

.. code:: bash

   $ kubectl scribe <command> -h

.. code:: bash

    $ cat config.yaml

    dest-access-mode: ReadWriteOnce
    dest-copy-method: None
    dest-namespace: dest
    source-namespace: source
    source-pvc: mysql-pv-claim
    source-copy-method: None

Refer to the :doc:`example config </usage/rsync/plugin_opts>` that lists plugin options with default values.

Create source application
--------------------------

.. code:: bash

    $ kubectl create ns source
    $ kubectl -n source apply -f examples/source-database/

Modify the mysql database
^^^^^^^^^^^^^^^^^^^^^^^^^

.. code:: bash

    $ kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > create database my_new_database;
    > show databases;
    > exit
    $ exit

Create a scribe destination
----------------------------

.. code:: bash

    $ kubectl scribe start-replication

The above command:
* Creates destination PVC (if dest PVC not provided)
* Syncs SSH secret from destination to source
* Creates replication destination
* Creates replication source

Necessary flags are configured in :code:`./config.yaml` shown above.

Create a replication database
-----------------------------

Create the destination application from the scribe example:

.. code:: bash

    $ kubectl apply -n dest -f examples/destination-database/mysql-deployment.yaml
    $ kubectl apply -n dest -f examples/destination-database/mysql-service.yaml
    $ kubectl apply -n dest -f examples/destination-database/mysql-secret.yaml

Delete the replication destination 
-----------------------------------

TODO: Add delete command

Deleting the replication destination after the data sync is required to allow the
destination PVC to bind with the destination deployment pod. Also, delete the
synced dest-src ssh key secret in the source namespace to avoid errors with the
next data sync and stale ssh keys.

.. code:: bash

   $ kubectl delete -n dest < name of replication destination default: <destns-destination> >
   $ kubectl delete -n source < ssh-key-secret default scribe-rsync-dest-src-<destns>-destination >

Verify the synced database
^^^^^^^^^^^^^^^^^^^^^^^^^^

.. code:: bash

    $ kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > show databases;
    > exit
    $ exit
