============================
Destination Rsync Parameters
============================
Various configuration options exist for rsync functionality. All configuration options
can be listed by running the following.

.. code-block::

   $ kubectl explain replicationdestination.spec

To see specific rsync parameters run the following.

.. code-block::

   $ kubectl explain replicationdestination.spec.rsync

The sections below will describe some of the most commonly used values.

Service Type
============
The serviceType field describes how the `replicationsource` pod will connect
to the `replicationdestination` pod.

Two options exist to define the Kubernetes service object.

- **LoadBalancer** - A Load Balancer is created to allow for the destination pod to be accessed from the source. This
  may require a cloud provider or metal LB to use this functionality.

- **ClusterIP** - A clusterIP is defined which allows for the destination pod to be accessed from pods that have
  access to the serviceIP CIDR. A service mesh or Submariner could also be used to allow for routing using the
  serviceIP CIDR.

Copy Method
===========
The `replicationdestination` CustomResources support a CopyMethod field which will indicate how the data is to be delivered and received. Scribe supports the underlying CSI methods for Snapshots and Clones. The Destination receives incoming source data as a PVC and the copyMethod will determine what happens once the source data is received.

.. code-block:: yaml

  spec:
    rsync:
      serviceType: ClusterIP
      copyMethod: Snapshot
      capacity: 2Gi
      accessModes: [ReadWriteOnce]


The copyMethod supports two values on Destination side:

- **Snapshot** - Data is received from source as a PVC and if copyMethod is Snapshot then Scribe will take a
  VolumeSnapshot of the source PVC and this now becomes the latestImage in the CustomResource.

- **None** - data is received from the source as a PVC and if copyMethod is None, then Scribe will take no action on
  the PVC and it will become the latestImage in the CustomResource.

Capacity
========
Capacity is used to define the size of the Persistent Volume Claim(PVC). This PVC is mounted by the
`replicationdestination` pod and used to store the items copied from the source cluster.

SSH Keys
========
If the field `sshKeys` is not defined Scribe will generate SSH keys automatically but if you would
like to provide your own follow the steps ref::`for generating SSH keys <ssh-keys>`.
