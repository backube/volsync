=====
Usage
=====

.. toctree::
   :hidden:

   triggers
   metrics/index
   rclone/index
   restic/index
   rsync/index
   cli/index

There are three different replication methods built into VolSync. Choose the method that best fits your use-case:

:doc:`Rclone replication <rclone/index>`
   Use Rclone-based replication for multi-way (1:many) scenarios such as
   distributing data to edge clusters from a central site.
:doc:`Restic backup <restic/index>`
   Create a Restic-based backup of the data in a PersistentVolume.
:doc:`Rsync replication <rsync/index>`
   Use Rsync-based replication for 1:1 replication of volumes in scenarios such
   as disaster recovery, mirroring to a test environment, or sending data to a
   remote site for processing.

Triggers
========

VolSync :doc:`supports several types of triggers <triggers>` to specify when to schedule the replication.

Metrics
=======

VolSync :doc:`exposes a number of metrics <metrics/index>` that permit monitoring
the status of replication relationships via Prometheus.

