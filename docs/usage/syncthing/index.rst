===========================
Syncthing-based replication
===========================

.. toctree::
   :hidden:

.. sidebar:: Contents

   .. contents:: Syncthing-based replication
      :local:

VolSync supports synchronization of PersistentVolume data across a vast number of volumes using a Syncthing-based data mover.
With the Syncthing-based approach, any number of PersistentVolumes can be configured to share the same 
synchronized set of data, and any mutation on one of the nodes will be replicated to all other nodes.

A ReplicationSource object specifies which volume to sync, what peers to sync with, and Syncthing handles the rest.
Syncthing is a peer-to-peer system which keeps several directories in sync across 

VolSync uses a custom-built Syncthing mover which disables the use of relay servers and global announce, and instead relies on 
being provided with the addresses of other Syncthing peers directly.

.. note::
	  Syncthing is peer-to-peer technology which connects to other peers directly rather than going through intermediary servers.
	  Because Syncthing lacks centralization, it resolves file conflicts by favoring the `most recent version <https://docs.syncthing.net/users/syncing.html#conflicting-changes>`_.



Syncing Volumes
===============

To sync a PersistentVolume, simply specify the volume's name in the ReplicationSource's ``.spec.SourcePVC`` field.

Suppose we have a PVC named ``todo-database`` and wanted to configure VolSync to keep it synced. 
We  create the following ReplicationSource and specify the PVC in ``.spec.SourcePVC``: 

.. code-block:: yaml
	
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: sync-todo-database
   spec:
     sourcePVC: todo-database
     syncthing:
       serviceType: ClusterIP

This will cause VolSync to spin up a Syncthing instance that mounts the specified PVC and syncs it with the other specified peers.
VolSync will also create a ``Service`` to expose Syncthing's data port. Other ReplicationSource objects must specify 
this service's address in order to connect to our Syncthing instance.

.. note::
   By default, the type of Service created is ``ClusterIP``, although ``LoadBalancer`` is also supported. To specify the type of Service you'd like VolSync to create,
   specify ``.spec.syncthing.serviceType`` in your ``ReplicationSource``.


Obtaining Your Syncthing Information
====================================

VolSync simplifies this process: once your Syncthing instance is online and ready, simply check the values of the ``.status.syncthing.address`` and ``.status.syncthing.ID`` fields in your ReplicationSource:

.. code-block:: yaml
    :caption: ReplicationSource object with the Syncthing ID and address highlighted.
    :emphasize-lines: 23,24

    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationSource
    metadata:
      creationTimestamp: "2022-04-27T20:25:32Z"
      generation: 1
      name: sync-todo-database
      namespace: legendary-node-app
      resourceVersion: "443737"
      uid: 7284700f-ecf0-4185-914e-3067cfc0efd0
    spec:
      sourcePVC: todo-database
      syncthing:
        serviceType: ClusterIP
    status:
      conditions:
      - lastTransitionTime: "2022-04-27T20:26:23Z"
        message: Reconcile complete
        reason: ReconcileComplete
        status: "True"
        type: Reconciled
      lastSyncStartTime: "2022-04-27T20:25:32Z"
      syncthing:
        ID: GVONGZX-6FVQPEY-4QWTVLK-TXNJUHA-5UGA625-UBC7HZQ-P5BG2XJ-EHJ4XQ3
        address: tcp://10.96.55.168:22000


Connecting To Peers
===================

In order for two Syncthing instances to connect with one another, each must know the address and ID of the other.
To accomplish this in VolSync, each ReplicationSource object must specify at least one other Syncthing peer in its ``.spec.syncthing.peers`` field.
If you would like Syncthing to be automatically connected with other peers as they join the cluster, you must set ``introducer: true`` for those peers you'd like to adopt connections from.

For instance, if we have another ReplicationSource object in a separate namespace, it would specify the address of our previous Syncthing instance in its ``.spec.syncthing.peers`` field: 

.. code-block:: yaml
    :caption: ReplicationSource object 'sync-todo-database-2' specifying 'sync-todo-database' as a peer.

    # Syncthing instance in namespace 'incredible-python-analytics'
    # (details omitted for brevity)
    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationSource
    metadata:
      name: sync-todo-database-2
    spec:
      sourcePVC: other-todo-database
      syncthing:
        serviceType: ClusterIP
        peers:
        - ID: GVONGZX-6FVQPEY-4QWTVLK-TXNJUHA-5UGA625-UBC7HZQ-P5BG2XJ-EHJ4XQ3
          address: tcp://10.96.55.168:22000
          introducer: true

The first ReplicationSource object, ``sync-todo-database``, would then need to include this ReplicationSource's address and ID in its ``.spec.syncthing.peers`` field:

.. code-block:: yaml
    :caption: 'sync-todo-database' in 'legendary-node-app' namespace specifying 'sync-todo-database-2' as a peer.

    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationSource
    metadata:
      name: sync-todo-database
    spec:
      sourcePVC: todo-database
      syncthing:
        serviceType: ClusterIP
        peers:
        - ID: KKPV6U6-LXMJROE-PIFX63H-NIHIC4Z-5CPIHLR-KLSH35T-RITFSPR-ND7KWQ6
          address: tcp://10.96.193.124:22000
          introducer: true
    
We can also set ``introducer`` to be true, meaning the other device introduces us to the rest of the nodes that are Syncthing the same PVC.