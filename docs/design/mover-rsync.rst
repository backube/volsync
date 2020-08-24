======================
Rsync-based data mover
======================

This document covers the design of the rsync-based data mover.

.. contents::
   :depth: 2

Overview
========

To meet the goal of being able to replicate arbitrary volumes, Scribe must have
a built-in, baseline replication method. `Rsync <https://rsync.samba.org/>`_ is
a well-known and reasonably efficient method to synchronize file data between
two locations. It supports both data compression as well as differential
transfer of data. Further, its support of ssh as a transport allows the data to
be transferred securely, authenticating both sides of the communication.

Replication flow
================

#) A point-in-time image of the primary application's data is captured by
   cloning the application's PVC. This new "replica source" PVC serves as the
   source for one iteration of replication.
#) A data mover pod is started on the primary side that syncs the data to a data
   mover pod on the secondary side. The data is transferred via rsync (running
   in the mover pods) over ssh. A shared set of keys allows mutual
   authentication of the data movers.
#) After successfully replicating the data to a target PVC on the secondary, the
   secondary PVC is snapshotted to create a point-in-time copy that is identical
   to the image captured in step 1.
#) The process can be repeated, beginning again with step 1. Subsequent
   transfers will only need to transfer changed data since the target PVC on the
   secondary is re-used with each iteration.

.. diagram::

   /-------------\   +---------+                                       +----------=+   /------------=\
   |             |   |{s}      |                                       |{s}        |   |             |
   | Application +<->+ Primary |                                       | Secondary +<->+ Application |
   |             |   | PVC     |                                       | PVC       |   |             |
   \-------------/   +---------+                                       +-----------+   \-------------/
                         |                                                  ^
                         |Clone                                             |Restore
                         v                                                  :
                     +---------+                  +---------+          +----------+
                     |{s}      |                  |{s}      |          |          |
                     | Replica |                  | Replica |   Snap   | Replica  |
                     | Source  |                  | Target  +--------->+ Snapshot |
                     | PVC     |                  | PVC     |          |          |
                     +---------+                  +---------+          +----------+
                         |                            ^
                         |                            |
                         v                            |
                     /--------\                   /--------\
                     |        |                   |        |
                     | Data   |                   | Data   |
                     | Mover  +----------=------->+ Mover  |
                     |        |                   |        |
                     \--------/                   \--------/
    

                Primary site                                        Secondary site

Failover
--------

When the primary application has failed, the secondary site should take over. In
order to start the application on the secondary site, the synchronized data must
be made accessible in a PVC.

As a part of bringing up the application, its PVC is created from the most
recent "replica snapshot". This promotion of the snapshot to a PVC is only
necessary during failover. The majority of the time (i.e., while the primary is
properly functioning), old replica snapshots will be replaced with a new
snapshot at the end of each round of synchronization.

Resynchronization
-----------------

After the primary site recovers, its data needs to be brought back in sync with
the secondary (currently the active site). This is accomplished by having a
reverse synchronization path identical to the flow above but with data flowing
from the secondary site to the primary.

The replication from secondary to primary can be configured a priori, with the
data movement only happening after failover. For example, the reverse
replication would use "Secondary PVC" from the above diagram as the volume to
replicate. In normal operation, this volume would not exist, idling the reverse
path. Once the secondary site becomes the active site, that PVC would exist,
allowing the reverse synchronization to flow, resulting in replicated snapshots
on the primary side. These can later be used to recreate the "Primary PVC", thus
restoring the application to the primary site.

Setup
=====

As a part of configuring the rsync replication, a CustomResource needs to be
created on both the source and destination cluster. This configuration must
contain:

Connection information
  Synchronization is handled via a push model--- the source creates the
  connection to the destination cluster. As such, the source must be provided
  with the host/port information necessary to contact the destination.
Authentication credentials
  An ssh connection is used to carry the rsync traffic. This connection is made
  via shared public keys between both sides of the connection. This allows the
  destination (ssh server) to authenticate the source (client) as well as
  allowing the source to validate the destination (by checking an associated ssh
  host key).

In order to make the configuration as easy as possible, the destination CR
should be created first. When reconciling, the operator will generate the
appropriate ssh keys and connection information in a Kubernetes Secret, placing
a reference to that secret in the Destination CR's ``status.methodStatus`` map.

This Secret will then be copied to the source cluster and referenced in
``spec.parameters`` when creating the Source CR.
