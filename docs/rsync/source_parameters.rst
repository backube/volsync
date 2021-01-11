============================
Source Rsync Parameters
============================
Various configuration options exist for rsync functionality. All configuration options
can be listed by running the following.

.. code-block::

   $ kubectl explain replicationsource.spec

To see specific rsync parameters run the following.

.. code-block::

   $ kubectl explain replicationsource.spec.rsync

The sections below will describe some of the most commonly used values.

Source PVC
==========
The `sourcePVC` field describes the Persistent Volume Claim(PVC) that will be copied from the source namespace/cluster to the destination namespace/cluster.

Trigger
========
`Trigger` defines the event that will cause the `replicationsource` objects to create the required job pod to perform the process of replication.

Schedule is defined similar to cron within the Linux operating system. Depending on your application could dictate how frequently or infrequently the replication should occur. It is important to evaluate how much data will change between sync cycles and the size of the data as it could cause replication attempts to stack up.

Copy Method
===========
The `replicationsource` will create a copy of the source volume and transfer the data to the destination as a PVC.

.. code-block:: yaml

  spec:
    sourcePVC: mysql-pv-claim
    trigger:
      schedule: "*/3 * * * *"
    rsync:
      sshKeys: scribe-rsync-dest-src-database-destination
      address: my.host.com
      copyMethod: Clone


The copyMethod supports three values on the Source side:

* **Snapshot** - will use the VolumeSnapshot capabilities of CSI and create a point in time copy of the PVC.
* **Clone** - will use the VolumeClone capabilities of CSI and create a copy of the PVC
* **None** - will not use any CSI capabilities (no VolumeSnapshot or VolumeClone) and simply copy the volume as is.

SSH Keys
========
The `sshKey` field is required as it defines the key that has access to the `replicationdestination` pod. When creating the `replicationdestination` object if a key is not defined secrets are automatically created for Scribe to use. If you opt for keys to automatically be created then the secret must be copied from the destination cluster and namespace to the source cluster and namespace. If you would like to provide your own keys follow the steps ref::`for generating SSH keys <ssh-keys>`.

Address
=========
The address defines the `ClusterIP` or `LoadBalancer` that was created by the `replicationdestination` object in the `serviceType` field. In the destination namespace and cluster run the following to define the value which will be used for the `address` field.

.. code-block::

   $ kubectl get replicationdestination database-destination -n dest --template={{.status.rsync.address}}
