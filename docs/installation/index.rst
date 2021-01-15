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

Development
===========

If you are developing Scribe, run the following as it will output the logs of
the controller to your terminal.

.. code-block:: bash

   # Install Scribe CRDs into the cluster
   $ make install

   # Run the operator locally
   $ make run

Kubernetes
==========

If you are running Kubernetes issue the following command.

.. code-block:: bash

   $ make deploy

OpenShift
=========

A special make target is provided to allow Scribe to be installed on OpenShift.
Using this method ensures the proper `SecurityContextConstraint
<https://docs.openshift.com/container-platform/4.6/rest_api/security_apis/securitycontextconstraints-security-openshift-io-v1.html>`_
is applied to the cluster.

.. code-block:: bash

   $ make deploy-openshift


In the case of the Kubernetes or OpenShift instructions above, verify Scribe is
running by checking the output of ``kubectl get pods``:

.. code-block:: bash

   $ kubectl -n scribe-system get pods
   NAME                                         READY   STATUS    RESTARTS   AGE
   scribe-controller-manager-77d89c5879-bvscw   2/2     Running   0          27s

At this point it is now possible to use the Rsync and Rclone capabilities of
Scribe.

Continue on to the :doc:`usage docs </usage/index>`.

