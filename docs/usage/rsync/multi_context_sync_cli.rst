===========================================
Rsync Database Cross-Cluster Plugin Example
===========================================

This example will sync data from mysql database persistent volumes
For this example, sync will happen between 2 clusters. Data will be synced
from cluster-name :code:`api-source-com:6443` to cluster-name :code:`destination123`
Snapshot copy method will be used.

.. note::
    * :doc:`Clusters must have the volsync operator installed </installation/index>`.
    * :doc:`Cluster storage may need configuring </installation/index>`.

Build VolSync
-------------

.. code:: bash

    $ make volsync
    $ mv bin/kubectl-volsync /usr/local/bin (or add to $PATH)

Merge Kubeconfigs
------------------

If clusters not already in a single kubeconfig, then merge like so:

~/kubeconfig1 with context :code:`destuser` and cluster-name :code:`destination123`

~/kubeconfig2 with context :code:`sourceuser` and cluster-name :code:`api-source-com:6443`

.. code:: bash

    $ export KUBECONFIG=~/kubeconfig1:~/kubeconfig2

You can view config with the following commands:

.. code:: bash

    $ kubectl config view
    $ kubectl config get-clusters
    $ kubectl config get-contexts

You can rename contexts with the following:

.. code:: bash

    $ kubectl config rename-context <oldname> <newname>

Create Source Application
--------------------------

.. code:: bash

    $ kubectl --context sourceuser create ns source
    $ kubectl --context sourceuser -n source apply -f examples/source-database/

Modify the Mysql Database
-------------------------

.. code:: bash

    $ kubectl --context sourceuser exec --stdin --tty -n source `kubectl --context sourceuser get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > create database my_new_database;
    > show databases;
    > exit
    $ exit

Create a VolSync-Config
-----------------------

Create a config file to designate your source and destination options.
You can also pass these individually to each command, but they add up so the
config file is usually a good option. You can add any, some, or all flags
to the config file. For multiple clusters, you must pass the source and destination
contexts and cluster names.

Create the config file at :code:`./config.yaml` *or* :code:`~/.volsync/config.yaml`,
volsync will look for that file in the current directory or in :code:`~/.volsync`.
For complete list of options for a command, run the following or consult the API:

.. code:: bash

   $ kubectl volsync <command> -h

.. code:: bash

    $ cat config.yaml

    dest-kube-context: destuser
    dest-kube-clustername: destination123
    dest-service-type: LoadBalancer
    dest-access-mode: ReadWriteOnce
    dest-copy-method: Snapshot
    dest-namespace: dest
    source-kube-context: sourceuser
    source-kube-clustername: api-source-com:6443
    source-namespace: source
    source-service-type: LoadBalancer
    source-copy-method: Snapshot
    source-pvc: mysql-pv-claim

Refer to the :doc:`example config </usage/rsync/plugin_opts>` that lists plugin options with default values.

Start a VolSync Replication
---------------------------

.. code:: bash

    $ kubectl volsync start-replication

The above command:
* Creates destination PVC (if dest PVC not provided & if dest CopyMethod=None)
* Creates replication destination
* Syncs SSH secret from destination to source
* Creates replication source

Necessary flags are configured in :code:`./config.yaml` shown above.

Set and Pause a VolSync Replication
-----------------------------------

Usually the source deployment will be scaled down before
pinning a point-in-time image.

.. code:: bash

    $ kubectl scale deployment/mysql --replicas=0 -n source --context sourceuser

.. code:: bash

    $ kubectl volsync set-replication

The above command:
* Sets a manual trigger on the replication source
* Waits for final data sync to complete
* Creates destination PVC with latest snapshot (if dest PVC not provided & if dest CopyMethod=Snapshot)

Necessary flags are configured in :code:`./config.yaml` shown above.


For the rest of the example, you'll be working from the :code:`destuser context`.
So we don't have to pass that to every kubectl command, run this:

.. code:: bash

    $ kubectl config use-context destuser

Create a Destination Application if not already running
--------------------------------------------------------

Create the destination application from the volsync example:

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

Resume Existing VolSync Replication
-----------------------------------

It may be desireable to periodically sync data from source to destination. In this case, the
`continue-replication` command is available. If scaled down, then scale back up and
resume replications.

.. code:: bash

    $ kubectl scale deployment/mysql --replicas=1 -n source --context sourceuser

.. code:: bash

    $ kubectl volsync continue-replication

The above command:
* Removes a manual trigger on the replication source

It is now possible to set the replication again with the following.

.. code:: bash

    $ kubectl scale deployment/mysql --replicas=0 -n source --context sourceuser
    $ kubectl volsync set-replication

After setting a replication, the destination application may be updated to reference the latest destination PVC. The stale destination PVC
will remain in the destination namespace.

Remove VolSync Replication
--------------------------

After verifying the destination application is up-to-date and the destination PVC is
bound, the volsync replication can be removed. **VolSync does not delete source or destination PVCs**.
Each new destination PVC is tagged with a date and time. It is up to the user to prune stale
destination PVCs.

.. code:: bash

    $ kubectl volsync remove-replication

The above command:
* Removes the replication source
* Removed the synced SSH Secret from the source namespace
* Removes the replication destination
