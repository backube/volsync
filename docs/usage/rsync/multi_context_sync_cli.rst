=============================================
Rsync Multiple Cluster Kubectl Plugin Example
=============================================

This example will sync data from mysql database persistent volumes
For this example, sync will happen between 2 clusters. Data will be synced
from cluster-name `api-source-com:6443` to cluster-name `destination123`

.. note::
    * :doc:`Clusters must have the scribe operator installed </installation/index>`.
    * :doc:`Cluster storage may need configuring </installation/index>`.

Build Scribe
------------

.. code:: bash

    $ make scribe
    $ mv bin/kubectl-scribe /usr/local/bin (or add to $PATH)

Merge Kubeconfigs
------------------

If clusters not already in a single kubeconfig, then merge like so:

~/kubeconfig1 with context `destuser` and cluster-name `destination123`

~/kubeconfig2 with context `sourceuser` and cluster-name `api-source-com:6443`

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

Create source application
-------------------------

.. code:: bash

    $ kubectl --context sourceuser create ns source
    $ kubectl --context sourceuser -n source apply -f ../scribe/examples/source-database/

Modify the mysql database
-------------------------

.. code:: bash

    $ kubectl --context sourceuser exec --stdin --tty -n source `kubectl --context sourceuser get pods -n source | grep mysql | awk '{print $1}'` -- /bin/bash
    # mysql -u root -p$MYSQL_ROOT_PASSWORD
    > create database my_new_database;
    > show databases;
    > exit
    $ exit

Create a scribe-config with necessary flags
-------------------------------------------

Create a config file to designate your source and destination options.
You can also pass these individually to each command, but they add up so the
config file is usually a good option. You can add any, some, or all flags
to the config file.

Create the config file at **./config.yaml** *or* **~/.scribeconfig/config.yaml**,
scribe will look for that file in the current directory or in **~/.scribeconfig**.
For complete list of options for a command, run the following or consult the API:

.. code:: bash

   $ kubectl scribe <command> -h

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

Create a replication destination
---------------------------------

Necessary flags are configured in `./config.yaml` shown above.

.. code:: bash

    $ kubectl --context destuser create ns dest
    $ kubectl scribe new-destination

Save the rsync address from the destination to pass to the new-source:

.. code:: bash

    $ address=$(kubectl --context destuser get replicationdestination/dest-destination  -n dest --template={{.status.rsync.address}})
    $ echo ${address}
    # be sure it's not empty, may take a minute to populate

Sync SSH secret from destination to source

This assumes the default secret name that is created by the scribe controller.
You can also pass `--ssh-keys-secret` that is a valid ssh-key-secret
in the DestinationReplication namespace and cluster.

Necessary flags are configured in `./config.yaml` shown above.
Save the output from the command below, as you will need the
name of the ssh-keys-secret to pass to `new-source`

.. code:: bash

    $ kubectl scribe sync-ssh-secret

## Create replication source

Necessary flags are configured in `./config.yaml` shown above.
The ssh-keys-secret name listed below is copied from output of `scribe sync-ssh-secret`.

.. code:: bash

    $ kubectl scribe new-source --address ${address} --ssh-keys-secret <ssh-keys-secret>

For the rest of the example, you'll be working from the `destuser context`.
So we don't have to pass that to every kubectl command, run this:

.. code:: bash

    $ kubectl config use-context destuser

Create the destination application
----------------------------------

.. code:: bash

    $ cd examples/destination-database
    $ cp mysql-pvc.yaml /tmp/pvc.yaml // will use that later
    # edit the /tmp/pvc.yaml with metadata.namespace
    # otherwise you may forget to add the `-n dest` when you apply (like I did).

    $ kubectl apply -n dest -f mysql-deployment.yaml
    $ kubectl apply -n dest -f mysql-service.yaml
    $ kubectl apply -n dest -f mysql-secret.yaml

**TODO:** add this to scribe CLI

To sync the data, you have to replace the PVC each time.
This is because PersistenVolumeClaims are immutable.
That is the reason for creating the PVC, extracting the yaml to a local file,
then updating the snapshot image. For each sync, find the latest image from the
ReplicationDestination, then use this image to create the PVC

Data Sync
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
