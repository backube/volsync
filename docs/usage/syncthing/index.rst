===========================
Syncthing-based replication
===========================

.. toctree::
   :hidden:

.. sidebar:: Contents

   .. contents:: Syncthing-based replication
      :local:

VolSync supports synchronization of ``PersistentVolume`` data across a vast number of volumes using a Syncthing-based data mover.

``ReplicationSource`` objects are configured to connect to other Syncthing devices in order to sync data on a provided ``PersistentVolumeClaim``.
Any changes made to the shared data will be propagated to the rest of the peers sharing the volume.

------

What Is Syncthing?
==================


In a nutshell, Syncthing is a distributed, peer-to-peer, file synchronization system.
It syncs the contents of a directory with a list of specified peers selected by the user, 
effectively replicating the data across various devices in real-time.

Unlike the other data movers supported by VolSync, Syncthing is an "always-on" system, which means that 
it is always listening for changes on the local filesystem, as well as updates from other peers. 

Taken from `Syncthing's website <https://syncthing.net/>`_:
  
  Syncthing is a continuous file synchronization program. It synchronizes files between two or more computers in real time, safely protected from prying eyes.



How Syncthing Works In VolSync
==============================

When a ``ReplicationSource`` is created and configured to sync a PVC with other peers, all of the connected peers will maintain their
own Volume containing the synced data. To detect file changes, Syncthing employs two methods: a filesystem watcher, which notifies 
Syncthing of any changes to the local filesystem, and a full filesystem scan which occurs routinely at a specified interval (default is an hour).

In order to launch Syncthing, VolSync creates the following resources in the same namespace as the ``ReplicationSource``:

- Source PVC to use as the Syncthing data source (provided by user)
- Config PVC to persist Syncthing's identity and configuration data across restarts
- A Service exposing Syncthing's API which VolSync will use for communication
- A Service exposing Syncthing's data port that other devices can connect to
- An Opaque Secret containing the Syncthing API keys and SSL certificates
- A Deployment that launches Syncthing and connects it with the necessary components

VolSync uses a custom-built Syncthing mover which disables the use of relay servers and global announce, and relying instead on 
being provided with the addresses of other Syncthing peers directly.

.. note::
    Syncthing is peer-to-peer technology which connects to other peers directly rather than going through intermediary servers.
    Because Syncthing lacks centralization, file conflicts are resolved by favoring the `most recent version <https://docs.syncthing.net/users/syncing.html#conflicting-changes>`_.


Configuring a Synced Volume
===========================

To configure a ``PersistentVolume`` for syncing, create a ``ReplicationSource``
with the ``PersistentVolume``'s name specified in ``.spec.sourcePVC``, and provide a basic 
Syncthing configuration by specifying ``.spec.syncthing.serviceType: ClusterIP``.

.. code-block:: yaml
  :caption: Minimal specification of a ReplicationSource using the Syncthing data mover

  ---
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: sync-todo-database
  spec:
    sourcePVC: todo-database
    syncthing:
      serviceType: ClusterIP

This will launch a bare-bones Syncthing instance that uses ``ClusterIP`` to expose the data service.
No other peers will be configured at this point, so our volume will not be shared with anyone else.


.. note::
  Syncthing unions the data in the provided ``PersistentVolume`` with the data from the other peers.
  When two pieces of data have the same name, the most recent data will be favored.


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

In order to connect two Syncthing-based ReplicationSources, each must list the other's address and ID in ``.spec.syncthing.peers``.

Additionally, VolSync requires that you declare whether or not a given ReplicationSource should be automatically connected with the other's peers by setting ``introducer: true/false``.
This allows new ReplicationSources to be automatically connected without needing to explicitly update your ReplicationSources.

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