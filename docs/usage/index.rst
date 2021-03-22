=====
Usage
=====

.. toctree::
   :hidden:

   metrics/index
   rclone/index
   rsync/index

There are two different replication methods built into Scribe. Choose the method that best fits your use-case:

:doc:`Rclone replication <rclone/index>`
   Use Rclone-based replication for multi-way (1:many) scenarios such as
   distributing data to edge clusters from a central site.
:doc:`Rsync replication <rsync/index>`
   Use Rsync-based replication for 1:1 replication of volumes in scenarios such
   as disaster recovery, mirroring to a test environment, or sending data to a
   remote site for processing.

Metrics
=======

Scribe :doc:`exposes a number of metrics <metrics/index>` that permit monitoring
the status of replication relationships via Prometheus.
