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
    dest-copy-method: Snapshot
    dest-namespace: dest
    source-namespace: source
    source-pvc: mysql-pv-claim
    source-copy-method: Snapshot

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

Create a replication destination
---------------------------------

Necessary flags are configured in :code:`./config.yaml` shown above.

.. code:: bash

    $ kubectl create ns dest
    $ kubectl scribe new-destination

Save the rsync address from the destination to pass to the new-source:

.. code:: bash

    $ address=$(kubectl get replicationdestination/dest-destination  -n dest --template={{.status.rsync.address}})
    $ echo ${address} 
    # be sure it's not empty, may take a minute to populate

Sync SSH secret from destination to source
------------------------------------------

This assumes the default secret name that is created by the scribe controller.
You can also pass :code:`--ssh-keys-secret` that is a valid ssh-key-secret in the
DestinationReplication namespace and cluster.

Necessary flags are configured in :code:`./config.yaml` shown above.
Save the output from the command below, you will need the name of the
ssh-keys-secret to pass to :code:`scribe new-source`.

.. code:: bash

    $ kubectl scribe sync-ssh-secret

Create a replication source
----------------------------

Necessary flags are configured in :code:`./config.yaml` shown above.

.. code:: bash

    $ kubectl scribe new-source --address ${address} --ssh-keys-secret <name-of-ssh-secret-from-output-of-sync>

Create a replication database
-----------------------------

Create the destination application from the scribe example:

.. code:: bash

    $ cd examples/destination-database
    $ cp mysql-pvc.yaml /tmp/pvc.yaml
    # edit the /tmp/pvc.yaml with metadata.namespace
    # otherwise you may forget to add the `-n dest` (like I did).

    $ kubectl apply -n dest -f mysql-deployment.yaml
    $ kubectl apply -n dest -f mysql-service.yaml
    $ kubectl apply -n dest -f mysql-secret.yaml

**TODO:** add this to scribe CLI

To sync the data, you have to replace the PVC with every sync.
This is because PersistenVolumeClaims are immutable.
That is the reason for extracting the yaml to a local file,
then updating it with the snapshot image. For each sync, find the latest image
from the ReplicationDestination, then use this image to create the PVC

Data sync
---------

.. code:: bash

    $ SNAPSHOT=$(kubectl get replicationdestination dest-destination -n dest --template={{.status.latestImage.name}})
    $ echo ${SNAPSHOT} // make sure this is not empty, may take a minute
    $ sed -i "s/snapshotToReplace/${SNAPSHOT}/g" /tmp/pvc.yaml
    $ kubectl apply -f /tmp/pvc.yaml

Verify the synced database
^^^^^^^^^^^^^^^^^^^^^^^^^^

.. code:: bash

    $ kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > show databases;
    > exit
    $ exit
