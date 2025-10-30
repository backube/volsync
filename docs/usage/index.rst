=====
Usage
=====

.. toctree::
   :hidden:

   permissionmodel
   moverserviceaccount
   resourcerequirements
   triggers
   pvccopytriggers
   metrics/index
   rclone/index
   restic/index
   kopia/index
   rsync/index
   rsync-tls/index
   syncthing/index
   cli/index
   volume-populator/index
   movervolumes

There are five different replication methods built into VolSync. Choose the method that best fits your use-case:

:doc:`Rclone replication <rclone/index>`
   Use Rclone-based replication for multi-way (1:many) scenarios such as
   distributing data to edge clusters from a central site.
:doc:`Restic backup <restic/index>`
   Create a Restic-based backup of the data in a PersistentVolume.
:doc:`Kopia backup <kopia/index>`
   Create a Kopia-based backup of the data in a PersistentVolume with advanced
   features like compression, parallelism, and concurrent access.
:doc:`Rsync replication (via TLS) <rsync-tls/index>`
   Use Rsync-based replication for 1:1 replication of volumes in scenarios such
   as disaster recovery, mirroring to a test environment, or sending data to a
   remote site for processing.
:doc:`Rsync replication (via ssh) <rsync/index>`
   This is the original rsync-based mover for 1:1 data replication. New
   deployments should favor the TLS-based implementation since the mover
   requires fewer privileges.
:doc:`Syncthing replication <syncthing/index>`
	 Use Syncthing-based replication for multi-way (many:many), live, eventually consistent data replication
	 in scenarios where the data is spread-out and updated in real-time, such as a wiki application,
	 or a private distributed file-store.

Permission model
================

The data replication mover Pods run in the user's source and destination
Namespaces. The permissions that are given to these Pods control what data can
be replicated. They also affect the security of the cluster. Please see the
:doc:`permission model documentation <permissionmodel>` for more details.

Service account
===============

The data replication Pods that run in the user's source and destination Namespaces
will use service accounts that are normally created by VolSync. Advanced users can use
their own service account instead. Please see the
:doc:`mover service account documentation <moverserviceaccount>` for more details.

Resource requirements
=====================

The data replication mover Pods can be configured to set specific resource requirements. This can include CPU and memory
resource requirements or resource limits. Please see the
:doc:`resource requirements documentation <resourcerequirements>` for more details.

Triggers
========

VolSync :doc:`supports several types of triggers <triggers>` to specify when to schedule the replication.

PVC Annotations for Copy Triggers
=================================

VolSync :doc:`supports source PVC annotations <pvccopytriggers>` to coordinate triggering when VolSync takes a copy
(snapshot or clone) for a replication.

Metrics
=======

VolSync :doc:`exposes a number of metrics <metrics/index>` that permit monitoring
the status of replication relationships via Prometheus.

Volume Populator
================

VolSync provides a :doc:`Volume Populator <volume-populator/index>` to allow creation of PVCs that reference a
ReplicationDestination as a dataSourceRef.

Additional Mover Volumes
========================

VolSync provides an :doc:`additional mover volumes <movervolumes>` feature to allow advanced users to mount
additional PVCs or secrets to the data mover Pods.
