===========================
Syncthing-based replication
===========================

.. toctree::
   :hidden:

   syncthing_example

.. sidebar:: Contents

   .. contents:: Syncthing-based replication
      :local:

VolSync supports active-active synchronization of data across several PersistentVolumes using a Syncthing-based data mover.
ReplicationSource objects are configured to connect to other Syncthing devices in order to sync data of a provided PVC.
Any changes made to the PVC will be propagated to the rest of the peers sharing the volume.

------



How Syncthing Works In VolSync
==============================

Syncthing connects to a cluster of nodes sharing a synchronized volume.
When one of the nodes syncing the volume modifies the data in the PV, the change will be propagated to the rest of the nodes within the Syncthing cluster.
Syncthing also includes an introducer feature which allows one device to be connected to a cluster of other devices upon configuring a single introducer node.
This can be used to create a hub-and-spoke model for replication, or any other kind of network.

When a ReplicationSource is created and configured to sync a PVC with other peers, all of the connected peers will maintain their
own Volume containing the synced data. To detect file changes, Syncthing employs two methods: a filesystem watcher, which notifies
Syncthing of any changes to the local filesystem, and a full filesystem scan which occurs routinely at a specified interval (default is an hour).
Since Syncthing is an "always-on" synchronization system, ReplicationSources will report their synchronization status as always being 'in-progress'.

VolSync uses a custom-built Syncthing mover which disables the use of relay servers and global announce, and relying instead on
being provided with the addresses of other Syncthing peers directly.


.. note::
    Syncthing is peer-to-peer technology which connects to other peers directly rather than going through intermediary servers.
    Because Syncthing lacks centralization, file conflicts are resolved by favoring the `most recent version <https://docs.syncthing.net/users/syncing.html#conflicting-changes>`_.


Configuring a ReplicationSource
===============================

Here's an example of a Syncthing-based ReplicationSource.

.. code-block:: yaml
    :caption: ReplicationSource object configuring the peers it should connect to.

    ---
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

The above ReplicationSource tells VolSync that it should use the Syncthing replication method in order
to sync the ``todo-database`` volume.

A service type of ``ClusterIP`` is used to expose the Syncthing data port, allowing us to connect with other peers within the cluster.
In order for Syncthing to connect to peers outside of the cluster, you will need to either use
``serviceType: LoadBalancer``, or a submariner-type cross-cluster networking configuration.
A single peer is specified for VolSync to sync the ``todo-database`` volume with, however you can specify as many or as few peers as you'd like.
To create a simple ReplicationSource without connecting to other peers, simply omit the ``peers`` field.

In order for two Syncthing-based ReplicationSources to connect to each other, each one must specify the other one in their ``peers`` list.

.. note::
  Syncthing combines the set of files in the provided ``PersistentVolume`` with those from the other peers.
  When two files have the same name, the file with the most recent data will be favored.


Syncthing options
-----------------

Here are all of the options that can be specified for the Syncthing mover:


peers
   A list of the Syncthing devices this ReplicationSource should sync the ``sourcePVC`` with. The peers
   being listed must also specify this ReplicationSource's Syncthing details in their spec for them to
   successfully connect with one another. Each peer contains the following fields:

   - ``ID`` - The peer's device ID.
   - ``address`` - The peer's address that we will attempt to connect on. This will usually be a TCP connection.
   - ``introducer`` - Whether this peer should act as an introducer node or not. If true, this peer will automatically connect us to other nodes that also have it set as an introducer.
serviceType
   The type of service used to expose Syncthing's data connection. Defaults to ``ClusterIP``. Valid values are:

   - ``ClusterIP`` - VolSync will expose the service through a ClusterIP; used for in-cluster networking.
   - ``LoadBalancer`` - The Syncthing data port is exposed through a LoadBalancer, which is used for connecting to other Syncthing instances outside of the cluster.
configCapacity
   Amount of storage to be used by the PVC storing Syncthing's configuration data.
   The default is ``1Gi`` when left unspecified.
configStorageClassName
   The name of the storage class to use for the PVC storing Syncthing's configuration data.
   When unspecified, VolSync will default to the storage class being used by the source PVC.
configVolumeAccessModes
   These are used to set the accessModes of the config PVC. When unspecified, these default to
   the accessModes present on the source PVC.


Source Status
-------------

Once the ReplicationSource has been deployed and Syncthing has properly configured itself,
it will populate the ``.status.syncthing`` field with information about your Syncthing node.

Here's an example of a ReplicationSource with a status:

.. code-block:: yaml
    :caption: ReplicationSource object with the Syncthing ID and address highlighted.
    :emphasize-lines: 24, 26

    ---
    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationSource
      name: sync-todo-database
    spec:
      sourcePVC: todo-database
    metadata:
      syncthing:
        serviceType: ClusterIP
        peers:
        - ID: 7NDBKMJ-XU2GWGG-4JJ5B5M-ONSDVAK-ZDXHKVM-6X7XYB7-ZG4NYDI-ZQ6FHQ4
          address: tcp://10.96.140.222:22000
          introducer: true
    status:
      conditions:
      - lastTransitionTime: "2022-04-27T20:26:23Z"
        message: Synchronization in-progress
        reason: SyncInProgress
        status: "True"
        type: Synchronizing
      lastSyncStartTime: "2022-04-27T20:25:32Z"
      syncthing:
        # This ReplicationSource's Syncthing ID.
        ID: GVONGZX-6FVQPEY-4QWTVLK-TXNJUHA-5UGA625-UBC7HZQ-P5BG2XJ-EHJ4XQ3
        # This ReplicationSource's Syncthing address.
        address: tcp://10.96.55.168:22000
        # The Syncthing peers this ReplicationSource is connected to.
        peers:
        - # The Syncthing ID of the peer we're connected to.
          ID: JDKRGMR-HOX3QQ6-N4OLXBD-VRLS3D4-2DBFELP-6QKIFYB-4ZP3YSF-Q37KAQU
          # The connected peer's Syncthing address.
          address: tcp://10.96.168.12:22000
          # Whether or not we have an active connection with this peer.
          connected: true
          # The connected device's local name. Here this is another Pod's name.
          deviceName: volsync-syncthing-1-76dfbfb4d7-5fhc8
          # The Syncthing ID of the peer that introduced us to this peer
          introducedBy: 7NDBKMJ-XU2GWGG-4JJ5B5M-ONSDVAK-ZDXHKVM-6X7XYB7-ZG4NYDI-ZQ6FHQ4



The above status displays your Syncthing ID in ``.status.syncthing.ID`` and address which other peers will need to specify in order to connect to this ReplicationSource in ``.status.syncthing.address``.

Additionally, it displays a list of peers that this ReplicationSource is connected to.
Each peer listing contains the following fields:

ID
   The connected peer's Syncthing device ID.

address
   The connected peer's address.

connected
   A boolean indicating whether or not this ReplicationSource
   has an active connection to the listed peer.

deviceName
   Friendly name associated with the other device, configured once upon connection.

introducedBy
   The Syncthing ID of the peer that introduced us to this peer.
   This field will only appear for peers that have been introduced to us.


Hub and Spoke Synchronization
=============================

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

  ---
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
      # Bob's ReplicationSource
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
      # Alice's ReplicationSource
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

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: alice-rs
  spec:
    sourcePVC: alice-data
    syncthing:
      serviceType: ClusterIP
      peers:
      # Bob's ReplicationSource
      - ID: ZQF2PVB-UMNMXCF-HWMQ7DX-ELOWLPZ-OBNF7JM-XQSTFXE-O23GBWH-R5WPOQZ
        address: tcp://bob.address:22000
        introducer: false
      # Charlie ReplicationSource
      - ID: LUHH7KT-KYD47H5-NJ5LFD3-EF62KHJ-KW65NUI-5NJ6CTD-FL5IE6M-5XW7CQZ
        address: tcp://charlie.address:22000
        introducer: false


.. code-block:: yaml
  :caption: Charlie's ReplicationSource configured with Alice as a hub.

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: charlie-rs
  spec:
    sourcePVC: charlie-data
    syncthing:
      serviceType: ClusterIP
      peers:
      # Alice's ReplicationSource
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
===================

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



Communicating With Syncthing
============================

Unlike the other data movers, Syncthing never stops running. This changes our approach of controlling it to
having Syncthing always be running, and communicating with it through it's REST API.

Syncthing has a REST API which handles connections through HTTPS. In order to do this securely,
VolSync provisions a self-signed certificate and key for the Syncthing REST API, passing the generated
certificate and key to Syncthing on first launch, and adding the Public Key PEM to VolSync's root CA bundle.

You can provide a custom HTTPS key/certificate pair by overriding the Secret which VolSync uses to store its
communication credentials for Syncthing.

An example of a Secret which overrides the default Syncthing credentials is shown below, all fields must be provided:

.. code-block:: yaml
  :caption: Kubernetes Secret preloading custom HTTPS certificates

  kind: Secret
  apiVersion: v1
  metadata:
    # this should be in the format: volsync-<REPLICATION_SOURCE_NAME>
    name: volsync-my-replication-source
  type: Opaque
  # all of these fields must be provided
  data:
    # loaded by Syncthing
    httpsKeyPEM: <your base64 encoded HTTPS private key>
    # loaded into Syncthing and used by VolSync as a root CA
    httpsCertPEM: <your base64 encoded HTTPS certificate>
    # The API key used by VolSync to authenticate API requests
    apikey: <base64-encoded API Key>
    # These fields are solely for securing the Web UI from being accessed.
    username: <base64-encoded username>
    password: <base64-encoded password>

Once you have deployed this secret in your intended namespace, you will then need to create the ReplicationSource
using the name you specified in the above Secret. For example, a custom secret named "volsync-my-replication-source"
would require you to name the ReplicationSource "my-replication-source".

.. note::
  This Secret must be created **before** creating the ReplicationSource.
  Otherwise, Syncthing will generate its own set of credentials and ignore yours.
