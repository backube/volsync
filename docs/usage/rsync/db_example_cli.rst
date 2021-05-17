=============================
Rsync Database Plugin Example
=============================

This example will sync data from mysql database persistent volumes
For this example, sync will happen within a single cluster and 2 namespaces.

.. note::
    * :doc:`Cluster must have the scribe operator installed </installation/index>`.
    * :doc:`Cluster storage may need configuring </installation/index>`.

Build Scribe CLI
----------------

.. code:: bash

    $ make scribe
    $ mv bin/kubectl-scribe /usr/local/bin (or add to $PATH)

Create a Scribe-Config
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
    dest-copy-method: Snapshot
    dest-namespace: dest
    source-namespace: source
    source-pvc: mysql-pv-claim
    source-copy-method: Snapshot

Refer to the :doc:`example config </usage/rsync/plugin_opts>` that lists plugin options with default values.

Create Source Application
--------------------------

.. code:: bash

    $ kubectl create ns source
    $ kubectl -n source apply -f examples/source-database/

Modify the Mysql Database
^^^^^^^^^^^^^^^^^^^^^^^^^

.. code:: bash

    $ kubectl exec --stdin --tty -n source `kubectl get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > create database my_new_database;
    > show databases;
    > exit
    $ exit

Start a Scribe Replication
----------------------------

.. code:: bash

    $ kubectl scribe start-replication

The above command:
* Creates destination PVC (if dest PVC not provided & if dest CopyMethod=None)
* Creates replication destination
* Syncs SSH secret from destination to source
* Creates replication source

Necessary flags are configured in :code:`./config.yaml` shown above.

Set and Pause a Scribe Replication
-----------------------------------

Usually the source deployment will be scaled down before
pinning a point-in-time image.

.. code:: bash

    $ kubectl scale deployment/mysql --replicas=0 -n source

.. code:: bash

    $ kubectl scribe set-replication

The above command:
* Sets a manual trigger on the replication source
* Waits for final data sync to complete
* Creates destination PVC with latest snapshot (if dest PVC not provided & if dest CopyMethod=Snapshot)

Necessary flags are configured in :code:`./config.yaml` shown above.

Create a Destination Application if not already running
--------------------------------------------------------

Create the destination application from the scribe example:

.. code:: bash

    $ kubectl apply -n dest -f examples/destination-database/mysql-deployment.yaml
    $ kubectl apply -n dest -f examples/destination-database/mysql-service.yaml
    $ kubectl apply -n dest -f examples/destination-database/mysql-secret.yaml

Edit the Destination Application with Destination PVC
------------------------------------------------------

.. code:: bash

   $ kubectl edit deployment/mysql -n dest

Replace the value of Spec.Volumes.PersistentVolumeClaim.ClaimName with name of destination PVC created from
the source PVC. By default, this will be named `sourcePVCName-date-time-stamp` in destination namespace.

Verify the Synced Database
^^^^^^^^^^^^^^^^^^^^^^^^^^

.. code:: bash

    $ kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > show databases;
    > exit
    $ exit

Resume Existing Scribe Replication
-----------------------------------

It may be desireable to periodically sync data from source to destination. In this case, the
`continue-replication` command is available.

.. code:: bash

    $ kubectl scribe continue-replication

The above command:
* Removes a manual trigger on the replication source

It is now possible to set the replication again with the following.

.. code:: bash

    $ kubectl scribe set-replication

After setting a replication, the destination application may be updated to reference the latest destination PVC. The stale destination PVC
will remain in the destination namespace.

Remove Scribe Replication
--------------------------

After verifying the destination application is up-to-date and the destination PVC is
bound, the scribe replication can be removed. **Scribe does not delete source or destination PVCs**.
Each new destination PVC is tagged with a date and time. It is up to the user to prune stale
destination PVCs.

.. code:: bash

    $ kubectl scribe remove-replication

The above command:
* Removes the replication source
* Removed the synced SSH Secret from the source namespace
* Removes the replication destination
