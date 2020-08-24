=================
A case for Scribe
=================

.. contents::
   :depth: 2

Motivation
==========

As Kubernetes is used in an increasing number of critical roles, businesses are
in need of strategies for being able to handle disaster recovery. While each
business has its own requirements and budget, there are common building blocks
employed across many DR configurations. One such building block is asynchronous
replication of storage (PV/PVC) data between clusters.

While some storage systems natively support async replication (e.g, Ceph’s RBD
or products from Dell/EMC and NetApp), there are many that lack this capability,
such as Ceph’s cephfs or storage provided by the various cloud providers.
Additionally, it is sometimes advantageous to have different storage systems for
the source and destination, making vendor-specific replication schemes
unworkable. For example, it can be advantageous to have different storage  in
the cloud vs. on-prem due to resource or environmental constraints.

This project proposes to create a general method for supporting asynchronous,
cross-cluster replication that can work with any storage system supporting a
CSI-based storage driver. Given a single configuration interface, the controller
would implement replication using the most efficient method available. For
example, a simplistic CSI driver without snapshot capabilities should still be
supported via a best-effort data copy, but a storage system w/ inbuilt
replication capabilities should be able to use those mechanisms for fast,
efficient data transfer.

.. _case-for-use-cases:

Use cases
=========

While disaster recovery is the most obvious use for asynchronous storage
replication, there are a number of different scenarios that could benefit.

Case (1) - Async DR
-------------------

As an application owner, I’d like to ensure my application’s data is replicated
off-site to a potentially different secondary cluster in case there is a failure
of the main cluster. The remote copy should be crash-consistent such that my
application can restart at the remote site.

Once a failure has been repaired, I’d like to be able to “reverse” the
synchronization so that my primary site can be brought back in sync when the
systems recover.

Case (2) - Off-site analytics
-----------------------------

As a data warehouse owner, I’d like to periodically replicate my primary data to
one or more secondary locations where it can be accessed, read-only, by a
scale-out ML or analytics platform.

Case (3) - Testing w/ production data
-------------------------------------

As a software developer, I’d like to periodically replicate the data from the
production environment into an isolated staging environment for continuous
testing with real data prior to deploying application updates.

Case (4) - Application migration
--------------------------------

As an application owner, I’d like to migrate my production stateful application
to a different storage system (either on the same or different Kubernetes
cluster) with minimal downtime. I’d like to have the bulk of the data
synchronized in the background, allowing for minimal downtime during the actual
switchover.

Proposed solution
=================

Using CustomResources, it should be possible for a user to designate a
PersistentVolumeClaim on one cluster (the source) to be replicated to a
secondary location (the destination), typically on a different cluster. An
operator that watches this CR would then initialize and control the replication
process.

As stated above, remote replication should be supported regardless of the
capabilities of the underlying storage system. To accomplish this, the Scribe
operator would have one or more built-in generic replication methods plus a
mechanism to allow offloading the replication directly to the storage system
when possible.

Replication by Scribe is solely targeted at replicating PVCs, not objects.
However, the source and destination volumes should not need to be of the same
volume access mode (e.g., RWO, RWX), StorageClass, or even use the same CSI
driver, but they would be expected to be of the same volume mode (e.g., Block,
Filesystem).

Potential replication methods
-----------------------------

For specific storage systems to be able to optimize, the replication and
configuration logic must be modular. The method to use will likely need to be
specified by the user as there’s no standard Kubernetes method to query for
capabilities of CSI drivers or vendor storage systems. When evaluating the
replication method, if the operator does not recognize the specified method as
one internal to the operator, it would ignore the replication object so that an
different (storage system-specific) operator could respond. This permits
vendor-specific replication methods without requiring them to exist in the main
Scribe codebase.

There are several methods that could be used for replication. From
(approximately) least-to-most efficient:

#) Copy of live PVC into another PVC

   - This wouldn’t require any advanced capabilities of the CSI driver,
     potentially not even dynamic provisioning
   - Would not create crash-consistent copies. Volume data would be inconsistent
     and individual files could be corrupted. (Gluster’s georep works like this,
     so it may have some value)
   - For RWO volumes, the copy process would need to be co-scheduled w/ the
     primary workload
   - Copy would be via rsync-like delta copy

#) Snapshot-based replication

   - Requires CSI driver to support snapshot
   - Source would be snapshotted, the snapshot would be used to create a new
     volume that would then be replicated to the remote site
   - Copy would be via rsync-like delta copy
   - Remote site would snapshot after each complete transfer

#) Clone-based replication

   - Requires CSI driver to support clone
   - Source would be cloned directly to create the source for copying
   - Copy would be via rsync-like delta copy
   - Remote site would snapshot after each complete transfer

#) Storage system specific

   - A storage system specific mechanism would need to both set up the
     relationship and handle the sync.
   - Our main contribution here would be a unifying API to provide a more
     consistent interface for the user.

Built-in replication
--------------------

With the exception of the storage system specific method, the other options
require the replication to be handled by Scribe, copying the data from the
source to the destination volume.

It is desirable for Scribe's replication to be relatively efficient and only
transfer data that has changed. As a starting point for development, it should
be possible to use a pod running `rsync <https://rsync.samba.org/>`_,
transferring data over an ssh connection.

Initial implementation
======================

The initial Scribe implementation should be focused on providing a minimal
baseline of functionality that provides value. As such, the focus will be
providing clone-based replication via an `rsync data mover <mover-rsync.html>`_, and this
implementation will assume both the source and destination are Kubernetes
clusters.
