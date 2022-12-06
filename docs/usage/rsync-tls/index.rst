===========================
Rsync-TLS-based replication
===========================

.. toctree::
   :hidden:

   mysql-migration

.. sidebar:: Contents

   .. contents:: Rsync-based replication
      :local:

Rsync-based replication supports 1:1 asynchronous replication of volumes for use
cases such as:

- Disaster recovery
- Mirroring to a test environment
- Sending data to a remote site for processing

With this method, VolSync synchronizes data from a ReplicationSource to a
ReplicationDestination using `Rsync <https://rsync.samba.org/>`_ across a
TLS-protected tunnel, provided by `stunnel <https://stunnel.org>`_. By using
Rsync, the amount of data transferred during each synchronization is kept to a
minimum, and the TLS connection ensures that the data transfer is both
authenticated and secure.

------

Rsync-over-TLS uses a "push" model for the data replication. A schedule or other
trigger is used on the source side of the relationship to trigger each
replication iteration. The destination continuously waits for incoming data.

During each iteration, (optionally) a point-in-time (PiT) copy of the source
volume is created and used as the source data. The VolSync Rsync data mover then
connects to the destination using stunnel :ref:`(exposed via a Service)
<RsyncTLSServiceExplanation>` and sends any updates. At the conclusion of the
transfer, the destination (optionally) creates a VolumeSnapshot to preserve the
updated data.

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
     name: my-dest
     namespace: myns
   spec:
     rsyncTLS:
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
     name: my-dest
     namespace: myns
   spec:
     rsyncTLS:
     # ... omitted ...
   status:
   status:
     conditions:
       - lastTransitionTime: "2022-11-29T13:27:54Z"
         message: Synchronization in-progress
         reason: SyncInProgress
         status: "True"
         type: Synchronizing
     lastSyncStartTime: "2022-11-29T13:27:54Z"
     rsyncTLS:
       address: 10.96.231.114
       keySecret: volsync-rsync-tls-my-dest

In the above example,

- The destination is waiting for data (The Synchronizing condition is True)
- The destination TLS endpointis available at the IP specified in
  ``.status.rsyncTLS.address``. This should be used when configuring the
  corresponding ReplicationSource.
- The TLS key is available in the Secret ``.status.rsyncTLS.keySecret``. This
  Secret will need to be copied to the source so that it can authenticate.

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
within the ``.spec.rsyncTLS`` portion of the ReplicationDestination CustomResource.

.. include:: ../inc_dst_opts.rst

keySecret
   This is the name of a Secret that contains the TLS-PSK key for authenticating
   the connection with the source. If not provided, the key will be
   automatically generated and placed in ``.status.rsyncTLS.keySecret``.
moverSecurityContext
   This field allows specifying the `PodSecurityContext
   <https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#podsecuritycontext-v1-core>`_
   that will be used by the data mover. It can be used to customize the user,
   fsGroup, etc.
serviceType
   VolSync creates a Service to allow the source to connect to the destination.
   This field determines the :ref:`type of that Service <RsyncTLSServiceExplanation>`. Allowed values are ClusterIP
   or LoadBalancer. The default is ClusterIP.

Source configuration
====================

An example source configuration is shown here:

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: my-source
     namespace: source
   spec:
     sourcePVC: mysql-pv-claim
     trigger:
       schedule: "*/5 * * * *"
     rsyncTLS:
       keySecret: volsync-rsync-dest-src-database-destination
       address: my.host.com
       copyMethod: Clone

In the above example, the PVC named ``mysql-pv-claim`` will be replicated every
5 minutes using the rsync-TLS replication method. At the start of each iteration, a
clone of the source PVC will be created to generate a point-in-time copy for the
iteration. The source will then use the TLS key in the named Secret
(``.spec.rsyncTLS.keySecret``) to authenticate to the destination. The connection
will be made to the address specified in ``.spec.rsyncTLS.address``.

The synchronization schedule, ``.spec.trigger.schedule``, is defined by a
`cronspec <https://en.wikipedia.org/wiki/Cron#Overview>`_, making the schedule
very flexible. Both intervals (shown above) as well as specific times and/or
days can be specified.

When configuring the source, the user must manually create the Secret referenced
in ``.spec.rsyncTLS.keySecret`` by copying the contents from the Secret generated
previously on the destination.

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
   status:
     conditions:
       - lastTransitionTime: "2022-11-29T17:25:13Z"
         message: Waiting for next scheduled synchronization
         reason: WaitingForSchedule
         status: "False"
         type: Synchronizing
     lastSyncDuration: 28.818695981s
     lastSyncTime: "2022-11-29T17:25:28Z"
     nextSyncTime: "2022-11-29T17:30:00Z"
     rsyncTLS: {}

In the above example,

- The last synchronization was completed at ``.status.lastSyncTime`` and took
  ``.status.lastSyncDuration`` seconds.
- The next scheduled synchronization is at ``.status.nextSyncTime``.

.. note::

   The length of time required to synchronize the data is determined by the rate
   of change for data in the volume and the bandwidth between the source and
   destination. In order to avoid missed intervals, ensure there is sufficient
   bandwidth between the source and destination such that ``lastSyncDuration``
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
keySecret
   This is the name of a Secret that contains the TLS-PSK key for authenticating
   the connection with the source. If not provided, the key will be
   automatically generated and placed in ``.status.rsyncTLS.keySecret``.
moverSecurityContext
   This field allows specifying the `PodSecurityContext
   <https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.25/#podsecuritycontext-v1-core>`_
   that will be used by the data mover. It can be used to customize the user,
   fsGroup, etc.

Rsync-specific considerations
=============================

This section explains some additional considerations when setting up
rsync-TLS-based replication.

.. _TLSKeys:

TLS authentication
------------------

The TLS connection provided by stunnel is secured using TLS-PSK. This means that
the ReplicationSource and ReplicationDestination need to have access to a shared
key. The ``keySecret`` field in the CustomResources determine the location of
the key. If the name of a Secret in not provided in
``.spec.rsyncTLS.keySecret``, it will be automatically generated and the name of
the Secret placed into the ``.status.rsyncTLS.keySecret``.

This optional generation means that the key can either be automatically
generated, then copied to the other side or it can be pre-generated and supplied
to both sides when the replication is configured. The pre-generation approach
would be more suitable for gitops-type workflows.

The Secret itself contains a single field, named ``psk.txt``. This field follows
the `format expected by stunnel <https://www.stunnel.org/auth.html>`_:

    ``<id>:<at least 32 hex digits>``

For example:

    ``1:23b7395fafc3e842bd8ac0fe142e6ad1``

The corresponding Secret would be:

.. code-block:: yaml
    :caption: Example ``secret.yaml``

    apiVersion: v1
    data:
      # echo -n 1:23b7395fafc3e842bd8ac0fe142e6ad1 | base64
      psk.txt: MToyM2I3Mzk1ZmFmYzNlODQyYmQ4YWMwZmUxNDJlNmFkMQ==
    kind: Secret
    metadata:
      name: tls-key-secret
    type: Opaque

Rsync-TLS mover permissions
---------------------------

Due to limitations of rsync, when run in the normal, unprivileged mode, the data
mover Pod must run with a non-zero UID. This may require specifying a Pod
Security Context in the ReplicationSource and ReplicationDestination objects to
explicitly set the UID for the mover. Please see the documentation on the
:doc:`mover permission model </usage/permissionmodel>` for more details.

.. _RsyncTLSServiceExplanation:

Choosing between Service types (ClusterIP vs LoadBalancer)
----------------------------------------------------------

When using Rsync-TLS-based replication, the ReplicationSource needs to be able to
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
      rsyncTLS:
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
