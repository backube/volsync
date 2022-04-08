=======================
Rsync-based replication
=======================

.. toctree::
   :hidden:

   database_example
   external_rsync
   ssh_keys

.. sidebar:: Contents

   .. contents:: Rsync-based replication
      :local:

Rsync-based replication supports 1:1 asynchronous replication of volumes for use
cases such as:

- Disaster recovery
- Mirroring to a test environment
- Sending data to a remote site for processing

With this method, VolSync synchronizes data from a ReplicationSource to a
ReplicationDestination using `Rsync <https://rsync.samba.org/>`_ across an ssh
connection. By using Rsync, the amount of data transferred during each
synchronization is kept to a minimum, and the ssh connection ensures that the
data transfer is both authenticated and secure.

------

The Rsync method is typically configured to use a "push" model for the data
replication. A schedule or other trigger is used on the source side of the
relationship to trigger each replication iteration.

During each iteration, (optionally) a point-in-time (PiT) copy of the source
volume is created and used as the source data. The VolSync Rsync data mover then
connects to the destination using ssh :ref:`(exposed via a Service) <RsyncServiceExplanation>`
and sends any updates. At the conclusion of the transfer, the destination
(optionally) creates a VolumeSnapshot to preserve the updated data.

VolSync is configured via two CustomResources (CRs), one on the source side and
one on the destination side of the replication relationship.

Destination configuration
=========================

Start by configuring the destination; an example is shown below:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: myDest
     namespace: myns
   spec:
     rsync:
       copyMethod: Snapshot
       capacity: 10Gi
       accessModes: ["ReadWriteOnce"]
       storageClassName: my-sc
       volumeSnapshotClassName: my-vsc

In the above example, a 10 GiB RWO volume will be provisioned using the
StorageClass ``my-sc`` to serve as the destination for replicated data. This
volume is used by the rsync data mover to receive the incoming data transfers.

Since the ``copyMethod`` specified above is ``Snapshot``, a VolumeSnapshot will
be created, using the VolumeSnapshotClass named ``my-vsc``, at the end of each
synchronization interval. It is this snapshot that would be used to gain access
to the replicated data. The name of the current VolumeSnapshot holding the
latest synced data will be placed in the ReplicationDestination's
``.status.latestImage``.

Destination status
------------------

VolSync provides status information on the state of the replication via the
``.status`` field in the ReplicationDestination object:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: myDest
     namespace: myns
   spec:
     rsync:
     # ... omitted ...
   status:
     conditions:
       - lastTransitionTime: "2021-01-14T19:43:07Z"
         message: Reconcile complete
         reason: ReconcileComplete
         status: "True"
         type: Reconciled
     lastSyncDuration: 31.333710313s
     lastSyncTime: "2021-01-14T19:43:07Z"
     latestImage:
       apiGroup: snapshot.storage.k8s.io
       kind: VolumeSnapshot
       name: volsync-dest-test-20210114194305
     rsync:
       address: 10.99.236.225
       sshKeys: volsync-rsync-dest-src-test

In the above example,

- No errors were detected (the Reconciled condition is True)
- The destination ssh server is available at the IP specified in
  ``.status.rsync.address``. This should be used when configuring the
  corresponding ReplicationSource.
- The ssh keys for the source to use are available in the Secret
  ``.status.rsync.sshKeys``. This Secret will need to be :ref:`copied to the source <RsyncKeyCopy>` so
  that it can authenticate.

After at least one synchronization has taken place, the following will also be
available:

- ``lastSyncTime`` contains the time of the last successful data synchronization.
- ``latestImage`` references the object with the most recent copy of the data. If
  the copyMethod is Snapshot, this will be a VolumeSnapshot object. If the
  copyMethod is Direct, this will be the PVC that is used as the destination by
  VolSync.

Additional destination options
------------------------------

There are a number of more advanced configuration parameters that are supported
for configuring the destination. All of the following options would be placed
within the ``.spec.rsync`` portion of the ReplicationDestination CustomResource.

.. include:: ../inc_dst_opts.rst

sshKeys
   This is the name of a Secret that contains the ssh keys for authenticating
   the connection with the source. If not provided, the destination keys will be
   automatically generated and corresponding source keys will be placed in a new
   Secret. The name of that new Secret will be placed in
   ``.status.rsync.sshKeys``.
serviceType
   VolSync creates a Service to allow the source to connect to the destination.
   This field determines the :ref:`type of that Service <RsyncServiceExplanation>`. Allowed values are ClusterIP
   or LoadBalancer. The default is ClusterIP.
port
   This determines the TCP port number that is used to connect via ssh. The
   default is 22.

Source configuration
====================

An example source configuration is shown here:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mySource
     namespace: source
   spec:
     sourcePVC: mysql-pv-claim
     trigger:
       schedule: "*/5 * * * *"
     rsync:
       sshKeys: volsync-rsync-dest-src-database-destination
       address: my.host.com
       copyMethod: Clone

In the above example, the PVC named ``mysql-pv-claim`` will be replicated every
5 minutes using the Rsync replication method. At the start of each iteration, a
clone of the source PVC will be created to generate a point-in-time copy for the
iteration. The source will then use the ssh keys in the named Secret
(``.spec.rsync.sshKeys``) to authenticate to the destination. The connection
will be made to the address specified in ``.spec.rsync.address``.

The synchronization schedule, ``.spec.trigger.schedule``, is defined by a
`cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule
very flexible. Both intervals (shown above) as well as specific times and/or
days can be specified.

When configuring the source, the user must manually create the Secret referenced
in ``.spec.rsync.sshKeys`` by :ref:`copying the contents <RsyncKeyCopy>` from the Secret generated
previously on the destination (and made available in the destination's
``.status.rsync.sshKeys``).

Additionally, this ReplicationSource specifies a ``copyMethod`` of ``Clone``
which will directly generate a point-in-time copy of the source volume. However,
not all CSI drivers support volume cloning (most notably the ebs-csi driver). In
such cases, the ``copyMethod: Snapshot`` can be used to indirectly create a copy
of the volume by first taking a snapshot, then restoring it. In this case, the
user should also provide the ``volumeSnapshotClassName: <vsc-name>`` option to
indicate which VolumeSnapshotClass VolSync should use when creating the
temporary snapshot.

Source status
-------------

The state of the replication from the source's point of view is available in the
``.status`` field of the ReplicationSource:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mySource
     namespace: source
   spec:
     sourcePVC: mysql-pv-claim
     trigger:
       schedule: "*/5 * * * *"
     rsync:
       # ... omitted ...
   status:
     conditions:
       - lastTransitionTime: "2021-01-14T19:42:38Z"
         message: Reconcile complete
         reason: ReconcileComplete
         status: "True"
         type: Reconciled
     lastSyncDuration: 7.774288635s
     lastSyncTime: "2021-01-14T20:10:07Z"
     nextSyncTime: "2021-01-14T20:15:00Z"
     rsync: {}

In the above example,

- No errors were detected (the Reconciled condition is True).
- The last synchronization was completed at ``.status.lastSyncTime`` and took
  ``.status.lastSyncDuration`` seconds.
- The next scheduled synchronization is at ``.status.nextSyncTime``.

.. note::

   The length of time required to synchronize the data is determined by the rate
   of change for data in the volume and the bandwidth between the source and
   destination. In order to avoid missed intervals, ensure there is sufficient
   bandwidth between the source and destination such that ``lastSyncTime``
   remains safely below the synchronization interval
   (``.spec.trigger.schedule``).

Additional source options
-------------------------

There are a number of more advanced configuration parameters that are supported
for configuring the source. All of the following options would be placed within
the .spec.rsync portion of the ReplicationSource CustomResource.

.. include:: ../inc_src_opts.rst

address
   This specifies the address of the replication destination's ssh server. It
   can be taken directly from the ReplicationDestination's
   ``.status.rsync.address`` field.
sshKeys
   This is the name of a Secret that contains the ssh keys for authenticating
   the connection with the destination. If not provided, the source keys will be
   automatically generated and corresponding destination keys will be placed in
   a new Secret. The name of that new Secret will be placed in
   .status.rsync.sshKeys.
path
   This determines the path within the destination volume where the data should
   be written. In order to create a replica of the source volume, this should be
   left as the default of ``/``.
port
   This determines the TCP port number that is used to connect via ssh. The
   default is 22.
sshUser
   This is the username to use when connecting to the destination. The default
   value is "root".

For a concrete example, see the :doc:`database synchronization example <database_example>`.

Rsync-specific considerations
=============================

This section explains some additional considerations when setting up rsync-based
replication.

.. _RsyncKeyCopy:

Copying the SSH key secret
--------------------------

When setting up the replication, it is necessary for the ReplicationSource to
have a copy of the SSH keys so that it can connect to the network endpoint
created by the ReplicationDestination. While these keys can be :doc:`generated
manually <ssh_keys>`, the recommended method is to allow VolSync to generate the
keys when setting up the ReplicationDestination. The resulting Secret should
then be copied to the source cluster.

Below is an example of a ReplicationDestination object. The VolSync operator has
generated the SSH keys that should be used in the source, and it has provided
the name of the Secret containing them in the ``.status.rsync.sshKeys`` field:

.. code-block:: yaml
    :caption: ReplicationDestination with SSH key Secret highlighted
    :emphasize-lines: 27

    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationDestination
    metadata:
      creationTimestamp: "2022-02-17T13:56:16Z"
      generation: 1
      name: database-destination
      namespace: dest
      resourceVersion: "2307"
      uid: 71f0512b-8a6b-438c-9b9a-0dd2c0f4e7b8
    spec:
      rsync:
        accessModes:
        - ReadWriteOnce
        capacity: 2Gi
        copyMethod: Snapshot
        serviceType: ClusterIP
    status:
      conditions:
      - lastTransitionTime: "2022-02-17T13:56:30Z"
        message: Reconcile complete
        reason: ReconcileComplete
        status: "True"
        type: Reconciled
      lastSyncStartTime: "2022-02-17T13:56:16Z"
      rsync:
        address: 10.96.150.107
        sshKeys: volsync-rsync-dst-src-database-destination


This Secret exists in the same Namespace as the associated
Replicationdestination. It has the following contents:

.. code-block:: yaml
    :caption: Secret as created by VolSync

    apiVersion: v1
    data:
      destination.pub: c3NoL...
      source: LS0tL...
      source.pub: c3NoLX...
    kind: Secret
    metadata:
      creationTimestamp: "2022-02-17T13:56:30Z"
      name: volsync-rsync-dst-src-database-destination
      namespace: dest
      ownerReferences:
      - apiVersion: volsync.backube/v1alpha1
        blockOwnerDeletion: true
        controller: true
        kind: ReplicationDestination
        name: database-destination
        uid: 71f0512b-8a6b-438c-9b9a-0dd2c0f4e7b8
      resourceVersion: "2296"
      uid: 61ab5402-318f-46df-b36f-cd209f3d1455
    type: Opaque

The above Secret contains 3 fields: the source's public, the source's private,
and the destination's public keys.

This Secret must be copied to the source cluster, into the same Namespace where
the source PVC and ReplicationSource will reside. That can be accomplished as
follows:

.. code-block:: console

    $ kubectl -n dest get secret volsync-rsync-dst-src-database-destination -oyaml > secret.yaml

Once saved to the local file, prepare it for the new cluster/namespace by
removing the following fields from the ``metadata`` area:

- ``creationTimestamp``
- ``namespace``
- ``ownerReferences``
- ``resourceVersion``
- ``uid``

After removing the above fields, the Secret is as follows:

.. code-block:: yaml
    :caption: Prepared ``secret.yaml``

    apiVersion: v1
    data:
      destination.pub: c3NoL...
      source: LS0tL...
      source.pub: c3NoLX...
    kind: Secret
    metadata:
      name: volsync-rsync-dst-src-database-destination
    type: Opaque

Assuming the source objects will be in Namespace ``source``, this Secret can be
added to the source cluster via:

.. code-block:: console

    $ kubectl -n source create -f secret.yaml
    secret/volsync-rsync-dst-src-database-destination created

This Secret should then be referenced when creating the corresponding
ReplicationSource. For example:

.. code-block:: yaml
    :caption: ReplicationSource showing reference to SSH key Secret
    :emphasize-lines: 11

    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationSource
    metadata:
      name: database-source
      namespace: source
    spec:
      sourcePVC: mysql-pv-claim
      trigger:
        schedule: "*/10 * * * *"
      rsync:
        sshKeys: volsync-rsync-dest-src-database-destination
        address: my.host.com
        copyMethod: Clone

.. _RsyncServiceExplanation:

Choosing between Service types (ClusterIP vs LoadBalancer)
----------------------------------------------------------

When using Rsync-based replication, the ReplicationSource needs to be able to
make a network connection to the ReplicationDestination. This requires network
connectivity from the source to the destination cluster.

When a ReplicationDestination object is created, VolSync creates a corresponding
Service object to serve as the network endpoint. The type of Service
(LoadBalancer or ClusterIP) should be specified in the ReplicationDestination's
``.spec.rsync.serviceType`` field.

.. code-block:: yaml
    :caption: ReplicationDestination with service type highlighted
    :emphasize-lines: 12

    apiVersion: volsync.backube/v1alpha1
    kind: ReplicationDestination
    metadata:
      name: database-destination
      namespace: dest
    spec:
      rsync:
        accessModes:
        - ReadWriteOnce
        capacity: 2Gi
        copyMethod: Snapshot
        serviceType: ClusterIP

The clusters' networking configuration between the two clusters affects the
proper choice of Service type.

If ``ClusterIP`` is specified, the Service will receive an IP address allocated
from the "cluster network" address pool. By default, this collection of
addresses are not accessible from outside the cluster, making it a poor choice
for cross-cluster replication. However, various networking addons such as
`Submariner <https://submariner.io/>`_ bridge the cluster networks, making this
a good option.

If ``LoadBalancer`` is specified, an externally accessible IP address will be
allocated. This requires cluster support for load balancers such as those
provided by the various cloud providers or `MetalLB
<https://metallb.universe.tf/>`_ in the case of physical clusters. While this is
the easiest method for allocating an accessible address in cloud environments,
load balancers tend to incur additional costs and be limited in number.

To summarize the above trade-offs, when running on one of the public clouds,
using a LoadBalancer is a quick way to get started and will work for replicating
small numbers of volumes. If replicating a large number of volumes, an overlay
network solution such as Submariner in combination with ClusterIP addresses will
likely be more scalable.
