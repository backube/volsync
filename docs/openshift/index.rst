==============================
Getting Started with OpenShift
==============================


Requirements
============
To use Scribe with OpenShift the deployment must have the ability to use the CSI drivers for the underlying storage provider. For example on AWS, the storage class must use the gp2-csi rather than the GP2.

The easiest way to accomplish this is to deploy or update to OpenShift 4.6.

Deploying Clusters
==================
This demonstration will use two clusters on AWS. Follow the directions on https://cloud.redhat.com/openshift to deploy the two clusters.

Storage class
=============
Two storage classes are available after the installation.

.. code-block:: bash
   
   $ oc get sc
   NAME                PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
   gp2 (default)       kubernetes.io/aws-ebs   Delete          WaitForFirstConsumer   true                   179m
   gp2-csi             ebs.csi.aws.com         Delete          WaitForFirstConsumer   true                   179m


It is advised to modify the storageclasses to set the default storage class to *gp2-csi*.

.. code-block:: bash

   $ oc annotate sc/gp2-csi storageclass.kubernetes.io/is-default-class="true"
   $ oc annotate sc/gp2 storageclass.kubernetes.io/is-default-class-


Volume Snapshot class
=====================
Create the following VolumeSnapshotClass

.. code-block:: bash

   $ oc create -f - << SNAPCLASS
   ---
   apiVersion: snapshot.storage.k8s.io/v1beta1
   kind: VolumeSnapshotClass
   metadata:
     name: gp2-csi
   driver: ebs.csi.aws.com
   deletionPolicy: Delete
   SNAPCLASS

Finally set *gp2-csi* as the default VolumeSnapshotClass.

.. code-block:: bash
   
   $ oc annotate volumesnapshotclass/gp2-csi snapshot.storage.kubernetes.io/is-default-class="true"


Deploying Scribe
================
A special make parameter is provided to allow for Scribe to be installed
on OpenShift. Using this method ensures proper *SecurityContextContstraints*
are applied to the cluster.

.. code-block:: bash

   make deploy-openshift

Using Scribe
============
The OpenShift cluster(s) should now be able to use Scribe. To test Scribe
follow the steps to `use the rsync replication method <http://https://scribe-replication.readthedocs.io/en/latest/getting_started/index.html#using-rsync>`_
