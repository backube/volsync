============
Installation
============
The following directions will walk through the process of deploying Scribe.

Deploying
=========
Once the repository has been cloned run the following within the scribe directory to deploy the required
components.

.. note::
   Volume SnapShot capabilities are required for Scribe. If you are using Kind to develop functionality
   for Scribe or another Kubernetes provider ensure that you are using a CSI capable storageclass.

There are three options for using Scribe. Choose the option that relates to your situation.

Development
===========

If you are developing Scribe run the following as it will output the logs of
the controller to your terminal.

.. code-block:: bash

   make install
   make run

Kubernetes
==========
If you are running Kubernetes issue the following command.

.. code-block:: bash

   make deploy

OpenShift
=========

A special make parameter is provided to allow for Scribe to be installed
on OpenShift. Using this method ensures proper *SecurityContextContstraints*
are applied to the cluster.

.. code-block:: bash

   make deploy-openshift


Verify Scribe is running by checking the output of kubectl describe.

.. code-block:: bash

   kubectl describe pods -n scribe-system

At this point it is now possible to use the Rsync and Rclone capabilities of Scribe.

Depending on the replication method you are wanting to use choose :ref:`rclone replication <rclone>` or
:ref:`rsync replication <rsync>`.
