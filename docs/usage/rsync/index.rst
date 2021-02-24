=======================
Rsync-based replication
=======================

.. toctree::
   :hidden:

   database_example
   ssh_keys
   external_rsync

.. sidebar:: Contents

   .. contents:: Rsync-based replication

Rsync-based replication supports 1:1 asynchronous replication of volumes for use
cases such as:

- Disaster recovery
- Mirroring to a test environment
- Sending data to a remote site for processing

With this method, Scribe synchronizes data from a ReplicationSource to a
ReplicationDestination using `Rsync <https://rsync.samba.org/>`_ across an ssh
connection. By using Rsync, the amount of data transferred during each
synchronization is kept to a minimum, and the ssh connection ensures that the
data transfer is both authenticated and secure.

------

The Rsync method is typically configured to use a "push" model for the data
replication. A schedule or other trigger is used on the source side of the
relationship to trigger each replication iteration.

During each iteration, (optionally) a point-in-time (PiT) copy of the source
volume is created and used as the source data. The Scribe Rsync data mover then
connects to the destination using ssh (exposed via a Service or load balancer)
and sends any updates. At the conclusion of the transfer, the destination
(optionally) creates a VolumeSnapshot to preserve the updated data.

Scribe is configured via two CustomResources (CRs), one on the source side and
one on the destination side of the replication relationship.

Destination configuration
=========================

Start by configuring the destination; a minimal example is shown below:

.. code:: yaml

   ---
   apiVersion: scribe/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: myDest
     namespace: myns
   spec:
     rsync:
       copyMethod: Snapshot
       capacity: 10Gi
       accessModes: ["ReadWriteOnce"]

In the above example, a 10 GiB RWO volume will be provisioned using the default
StorageClass to serve as the destination for replicated data. This volume is
used by the rsync data mover to receive the incoming data transfers.

Since the ``copyMethod`` specified above is ``Snapshot``, a VolumeSnapshot will
be created at the end of each synchronization interval. It is this snapshot that
would be used to gain access to the replicated data. The name of the current VolumeSnapshot holding the latest synced data will be placed in ``.status.latestImage``.

Destination status
------------------

Scribe provides status information on the state of the replication via the
``.status`` field in the ReplicationDestination object:

.. code:: yaml

   ---
   apiVersion: scribe/v1alpha1
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
       name: scribe-dest-test-20210114194305
     rsync:
       address: 10.99.236.225
       sshKeys: scribe-rsync-dest-src-test

In the above example,

- No errors were detected (the Reconciled condition is True)
- The destination ssh server is available at the IP specified in
  ``.status.rsync.address``. This should be used when configuring the
  corresponding ReplicationSource.
- The ssh keys for the source to use are available in the Secret
  ``.status.rsync.sshKeys``.

After at least one synchronization has taken place, the following will also be
available:

- lastSyncTime contains the time of the last successful data synchronization.
- latestImage references the object with the most recent copy of the data. If
  the copyMethod is Snapshot, this will be a VolumeSnapshot object. If the
  copyMethod is None, this will be the PVC that is used as the destination by
  Scribe.

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
   Scribe creates a Service to allow the source to connect to the destination.
   This field determines the type of that Service. Allowed values are ClusterIP
   or LoadBalancer. The default is ClusterIP.
port
   This determines the TCP port number that is used to connect via ssh. The
   default is 22.

Source configuration
====================

A minimal source configuration is shown here:

.. code:: yaml

   ---
   apiVersion: scribe.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mySource
     namespace: source
   spec:
     sourcePVC: mysql-pv-claim
     trigger:
       schedule: "*/5 * * * *"
     rsync:
       sshKeys: scribe-rsync-dest-src-database-destination
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

Source status
-------------

The state of the replication from the source's point of view is available in the
``.status`` field of the ReplicationSource:

.. code:: yaml

   ---
   apiVersion: scribe.backube/v1alpha1
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
