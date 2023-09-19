====================================
Moving data into Kubernetes w/ Rsync
====================================
While VolSync is typically used to replicate data between Kubernetes clusters,
it is sometimes necessary to replicate data into a cluster from outside. For
example, when containerizing a previously standalone workload, that
application's data needs to be moved into the cluster and onto a PVC.

In this configuration, VolSync manages the destination (via a
ReplicationDestination object), but instead of having a VolSync
ReplicationSource as the sender, it will be an external program that plays that
role. It will transmit the data to the destination by initiating the Rsync over
SSH connection directly.

The VolSync CLI incorporates this functionality via its ``migration`` set of
sub-commands. For more information and a walk-through of how to perform
synchronization of data into a Kubernetes PVC, please see:

- :doc:`CLI/kubectl plugin installation<../cli/index>`
- :doc:`VolSync migration command<../cli/migration>`
