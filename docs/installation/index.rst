============
Installation
============

The following directions will walk through the process of deploying VolSync.

.. note::
   Volume snapshot and clone capabilities are required for some VolSync
   functionality. It is recommended that you use a CSI driver and StorageClass
   capable of snapshotting and cloning volumes.

There are three options for installing VolSync. Choose the option that relates to
your situation.

.. warning::
   VolSync requires the Kubernetes snapshot controller to be installed
   within a cluster. If the controller is not deployed review the
   snapshot controller documentation https://github.com/kubernetes-csi/external-snapshotter.

Kubernetes & OpenShift
======================

While the operator can be deployed via the ``make deploy`` or ``make
deploy-openshift`` targets, the recommended method for deploying VolSync is via
the Helm chart.

.. code-block:: bash

   # Add the Backube Helm repo
   $ helm repo add backube https://backube.github.io/helm-charts/

   # Deploy the chart in your cluster
   $ helm install --create-namespace -n volsync-system volsync backube/volsync

Verify VolSync is running by checking the output of ``kubectl get pods``:

.. code-block:: bash

   $ kubectl -n volsync-system get pods
   NAME                          READY   STATUS    RESTARTS   AGE
   volsync-686c8557bc-cr6k9       2/2     Running   0          13s

Configure default CSI storage
-----------------------------

AWS
^^^

.. code-block:: bash

    $ kubectl annotate sc/gp2 storageclass.kubernetes.io/is-default-class="false" --overwrite
    $ kubectl annotate sc/gp2-csi storageclass.kubernetes.io/is-default-class="true" --overwrite

    # Install a VolumeSnapshotClass
    $ kubectl create -f - << SNAPCLASS
    ---
    apiVersion: snapshot.storage.k8s.io/v1beta1
    kind: VolumeSnapshotClass
    metadata:
      name: gp2-csi
    driver: ebs.csi.aws.com
    deletionPolicy: Delete
    SNAPCLASS

    # Set gp2-csi as default VolumeSnapshotClass
    $ kubectl annotate volumesnapshotclass/gp2-csi snapshot.storage.kubernetes.io/is-default-class="true"

GCE
^^^

.. code-block:: bash

    $ kubectl annotate sc/standard storageclass.kubernetes.io/is-default-class="false" --overwrite
    $ kubectl annotate sc/standard-csi storageclass.kubernetes.io/is-default-class="true" --overwrite

    # Install a VolumeSnapshotClass
    $ kubectl create -f - << SNAPCLASS
    ---
    apiVersion: snapshot.storage.k8s.io/v1beta1
    kind: VolumeSnapshotClass
    metadata:
      name: standard-csi
    driver: pd.csi.storage.gke.io
    deletionPolicy: Delete
    SNAPCLASS

    # Set standard-csi as default VolumeSnapshotClass
    $ kubectl annotate volumesnapshotclass/standard-csi snapshot.storage.kubernetes.io/is-default-class="true"

At this point it is now possible to use the Rsync and Rclone capabilities of
VolSync.

Continue on to the :doc:`usage docs </usage/index>`.

Development
===========

If you are developing VolSync, run the following as it will run the operator
locally and output the logs of the controller to your terminal.

.. code-block:: bash

   # Install VolSync CRDs into the cluster
   $ make install

   # Run the operator locally
   $ make run
