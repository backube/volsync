===========================
Syncthing-based replication
===========================

.. toctree::
   :hidden:

   syncthing_example

.. sidebar:: Contents

   .. contents:: Syncthing-based replication
      :local:

VolSync supports synchronization of PersistentVolume data across a vast number of volumes using a Syncthing-based data mover.

ReplicationSource objects are configured to connect to other Syncthing devices in order to sync data of a provided PVC.
Any changes made to the PVC will be propagated to the rest of the peers sharing the volume.

------



How Syncthing Works In VolSync
==============================

When a ``ReplicationSource`` is created and configured to sync a PVC with other peers, all of the connected peers will maintain their
own Volume containing the synced data. To detect file changes, Syncthing employs two methods: a filesystem watcher, which notifies 
Syncthing of any changes to the local filesystem, and a full filesystem scan which occurs routinely at a specified interval (default is an hour).

In order to launch Syncthing, VolSync creates the following resources in the same namespace as the ``ReplicationSource``:

- Source PVC to use as the Syncthing data source (provided by user)
- Config PVC to persist Syncthing's identity and configuration data across restarts
- Service exposing Syncthing's API which VolSync will use for communication
- Service exposing Syncthing's data port that other devices can connect to
- Opaque Secret containing the Syncthing API keys and SSL certificates
- Deployment that launches Syncthing and connects it with the necessary components

VolSync uses a custom-built Syncthing mover which disables the use of relay servers and global announce, and relying instead on 
being provided with the addresses of other Syncthing peers directly.

.. note::
    Syncthing is peer-to-peer technology which connects to other peers directly rather than going through intermediary servers.
    Because Syncthing lacks centralization, file conflicts are resolved by favoring the `most recent version <https://docs.syncthing.net/users/syncing.html#conflicting-changes>`_.


Configuring a ReplicationSource
-------------------------------

A Basic Syncthing-based configuration consists of the source PVC to be synced, specified in ``.spec.sourcePVC``,
and the type of service that you would like Syncthing to be exposed on.

Here's an example of a basic Syncthing-based ReplicationSource which syncs a PVC named ``todo-database`` and uses 
a ``ClusterIP`` service to expose Syncthing's data port.

.. code-block:: yaml
  :caption: Minimal specification of a ReplicationSource using the Syncthing data mover

  ---
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: sync-todo-database
  spec:
    # The name of the PVC to be synced.
    sourcePVC: todo-database
    syncthing:
      # The type of service to use for Syncthing data connections.
      serviceType: ClusterIP

No other peers have been configured at this point, so our volume will not be shared with anyone else.


.. note::
  Syncthing unions the data in the provided ``PersistentVolume`` with the data from other peers.
  When two pieces of data have the same name, the most recent data will be favored.


Obtaining Your Syncthing Information
------------------------------------

Your Syncthing ID and data address can be found under ``.status.syncthing``. 
Other nodes must specify this information in order to connect to your ReplicationSource.

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


Configuring Other Peers
-----------------------

In order to configure other devices to use for syncing your PVC data, they must be
specified under ``.spec.syncthing.peers`` as an entry containing their ``ID``, ``address``, and ``introducer`` flag - indicating whether or not they should introduce you to other peers sharing the PVC.

Here's an example of a ReplicationSource that specifies a single peer:

.. code-block:: yaml
    :caption: ReplicationSource object configuring the peers it should connect to.

    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationSource
    metadata:
      name: sync-todo-database
    spec:
      sourcePVC: todo-database
      syncthing:
        serviceType: ClusterIP
        # List of peers that this ReplicationSource should connect to.
        peers:
          # The Syncthing ID of the peer.
        - ID: GVONGZX-6FVQPEY-4QWTVLK-TXNJUHA-5UGA625-UBC7HZQ-P5BG2XJ-EHJ4XQ3
          # The address of the peer - this will be used as a data connection.
          address: tcp://10.96.55.168:22000
          # Whether or not the peer should introduce this ReplicationSource to other peers.
          introducer: false



Hub and Spoke Synchronization With VolSync
==========================================

So far we have shown you have to configure each ReplicationSource with every other peer's information.
As you can probably tell, this requires more repetitive configuration as your Syncthing cluster gets larger.
Luckily, there is a feature that can be used to simplify this process.

Introducers To The Rescue
-------------------------

As mentioned in the previous section, VolSync provides an ``introducer`` setting that can be set on a peer-by-peer basis.
When another peer is configured to act as an introducer, it will introduce you to other peers that it's sharing the folder with.
These introductions happen automatically, and are automatically removed once the introducer is removed from the ``.spec.syncthing.peers`` list.

.. note::
  Introduced peers should be left out of the ``.spec.syncthing.peers`` list, as it
  may lead to strange behavior.

Because VolSync disables global announce and global discover as a method of determining how to connect to other peers,
introduced Syncthing nodes will only be introduced and connected if they also configured the intermediary node as an introducer.
When nodes are introduced to you that did not configure the introducing node to introduce them, their device IDs
will still be shared with you, but you will not be able to connect with them as their addresses are not provided.


Configuring Introducers
-----------------------

For Example, suppose we have the following two ReplicationSources:

.. code-block:: yaml
  :caption: Two ReplicationSources configured in the hub-and-spoke pattern.

  # Alice's ReplicationSource
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: alice-rs
  spec:
    sourcePVC: alice-data 
    syncthing:
      serviceType: ClusterIP
      peers:
      # bob's ReplicationSource
      - ID: ZQF2PVB-UMNMXCF-HWMQ7DX-ELOWLPZ-OBNF7JM-XQSTFXE-O23GBWH-R5WPOQZ
        address: tcp://bob.address:22000
        introducer: false

  # Bob's ReplicationSource
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: bob-rs
  spec:
    sourcePVC: bob-data
    syncthing:
      serviceType: ClusterIP
      peers:
      # alice's ReplicationSource
      - ID: 7NDBKMJ-XU2GWGG-4JJ5B5M-ONSDVAK-ZDXHKVM-6X7XYB7-ZG4NYDI-ZQ6FHQ4
        address: tcp://alice.address:22000
        introducer: true


Here, ``alice-rs`` is being configured by ``bob-rs`` to act as an introducer for any nodes that are currently connected to the shared PVC.
At the moment, there are only ``N=2`` Syncthing nodes in the entire cluster. 


Adding More Spokes
------------------

Now, let's suppose that we want to connect Charlie to everyone in the current cluster,
but without having to append his address and ID to the two other existing nodes.

In order to do this, we will need to update Alice's ``peers`` to include Charlie's Syncthing ID and address, 
as well as update Charlie's ``peers`` to include Alice with ``introducer: true`` set.

.. code-block:: yaml
  :caption: Alice's ReplicationSource configured with Bob and Charlie's information.

  # Alice's ReplicationSource
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: alice-rs
  spec:
    sourcePVC: alice-data 
    syncthing:
      serviceType: ClusterIP
      peers:
      # bob's ReplicationSource
      - ID: ZQF2PVB-UMNMXCF-HWMQ7DX-ELOWLPZ-OBNF7JM-XQSTFXE-O23GBWH-R5WPOQZ
        address: tcp://bob.address:22000
        introducer: false
      # charlie
      - ID: LUHH7KT-KYD47H5-NJ5LFD3-EF62KHJ-KW65NUI-5NJ6CTD-FL5IE6M-5XW7CQZ
        address: tcp://charlie.address:22000
        introducer: false


.. code-block:: yaml
  :caption: Charlie's ReplicationSource configured with Alice as a hub.

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: syncthing-3
  spec:
    sourcePVC: syncthing-3
    syncthing:
      serviceType: ClusterIP
      peers:
      # alice
      - ID: 7NDBKMJ-XU2GWGG-4JJ5B5M-ONSDVAK-ZDXHKVM-6X7XYB7-ZG4NYDI-ZQ6FHQ4
        address: tcp://alice.address:22000
        introducer: true

Once Charlie and Alice connect, Alice introduces Charlie to all of the other peers that have Alice configured as an introducer, in this case
she would introduce Charlie and Bob.

Configuring nodes this way allows us to have to only perform two operations anytime that we want to introduce a new node to the rest of the cluster,
rather than having to update every node in the cluster.

Removing Spokes
---------------

In order to remove a spoke from the cluster, simply remove it from the Hub's ``peers`` list.

For example, if Alice wants to remove Charlie, all she needs to do is remove the entry corresponding to his ID, and 
the rest of the Syncthing cluster will automatically remove him from their connections.


.. code-block:: yaml
  :caption: Alice's ReplicationSource configured to remove Charlie.

  # Alice's ReplicationSource
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: alice-rs
  spec:
    sourcePVC: alice-data 
    syncthing:
      serviceType: ClusterIP
      peers:
      # bob's ReplicationSource
      - ID: ZQF2PVB-UMNMXCF-HWMQ7DX-ELOWLPZ-OBNF7JM-XQSTFXE-O23GBWH-R5WPOQZ
        address: tcp://bob.address:22000
        introducer: false
      # charlie
      #- ID: LUHH7KT-KYD47H5-NJ5LFD3-EF62KHJ-KW65NUI-5NJ6CTD-FL5IE6M-5XW7CQZ
      #  address: tcp://charlie.address:22000
      #  introducer: false

Once applied, Alice — along with all of the nodes that she had introduced to Bob — will
remove Charlie from the cluster. As a result, Charlie will be disconnected from the cluster and
will no longer be syncing his version of the PVC.

.. note::
  Using introducers is purely optional, and PVCs can still be synced regardless of how the cluster graph is
  composed, so long that every node is connected to at least one other node in the cluster.


More on Introducers
=======================

Introducers are a great feature when it comes to usability, but there are some scenarios that users should generally avoid.

Transitive configuration
------------------------

For one, Syncthing configures the introduced nodes automatically and uses the introducer as their controller.
This means that if Alice is the introducer for Bob and Charlie and removes Bob from her list of peers,
Charlie's node will automatically remove Bob as well. Since Bob was connected to Charlie through Alice,
once Bob loses Alice as the introducer, he loses Charlie along with any other nodes that Alice had introduced him to.

Cyclic introducers
------------------

Syncthing introducers also contain a mechanism to automatically re-add introduced nodes if they were disconnected
for whatever reason. This means that if you configure two nodes as each other's introducer, you will never 
be able to disconnect them as they'll continue to re-add each other until the end of time.
