============
Installation
============

The following directions will walk through the process of deploying Scribe.

.. note::
   Volume snapshot and clone capabilities are required for some Scribe
   functionality. It is recommended that you use a CSI driver and StorageClass
   capable of snapshotting and cloning volumes.

There are three options for installing Scribe. Choose the option that relates to
your situation.

.. warning::
   Scribe requires the Kubernetes snapshot controller to be installed
   within a cluster. If the controller is not deployed review the
   snapshot controller documentation https://github.com/kubernetes-csi/external-snapshotter.

Kubernetes & OpenShift
======================

While the operator can be deployed via the ``make deploy`` or ``make
deploy-openshift`` targets, the recommended method for deploying Scribe is via
the Helm chart.

.. code-block:: bash

   # Add the Backube Helm repo
   $ helm repo add backube https://backube.github.io/helm-charts/

   # Deploy the chart in your cluster
   $ helm install --create-namespace scribe-system scribe backube/scribe

Verify Scribe is running by checking the output of ``kubectl get pods``:

.. code-block:: bash

   $ kubectl -n scribe-system get pods
   NAME                          READY   STATUS    RESTARTS   AGE
   scribe-686c8557bc-cr6k9       2/2     Running   0          13s

At this point it is now possible to use the Rsync and Rclone capabilities of
Scribe.

Continue on to the :doc:`usage docs </usage/index>`.

Development
===========

If you are developing Scribe, run the following as it will run the operator
locally and output the logs of the controller to your terminal.

.. code-block:: bash

   # Install Scribe CRDs into the cluster
   $ make install

   # Run the operator locally
   $ make run
