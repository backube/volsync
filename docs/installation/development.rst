===========
Development
===========

If you are developing VolSync, there are a few options to get up-and-running.
All of these options will assume the use of a local `kind cluster
<https://kind.sigs.k8s.io/>`_.

Once you have kind installed, there is a convenient script in the ``hack/``
directory that will get a cluster running and properly configured.

.. code-block:: console

   $ ./hack/setup-kind-cluster.sh

Once you have a cluster running, you can either build and deploy the operator in
the cluster, or you can run the operator locally against the cluster.

.. tabs::

   .. tab:: Build & deploy

      The below command will build all containers (operator and movers) from the
      local source, inject them into the running kind cluster, then use the
      local helm templates to start the operator.

      .. code-block:: console

         # Build, inject, and run
         $ ./hack/run-in-kind.sh

   .. tab:: Run locally

      The below commands will run the operator binary locally, but the mover
      containers will be pulled from Quay (``latest`` tag). This option is good
      when developing the operator code since it permits fast rebuilds and easy
      access to the operator logs.

      .. code-block:: console

         # Install VolSync CRDs into the cluster
         $ make install

         # Run the operator locally
         $ make run

If you will be working with the Rclone or Restic movers, you may want to deploy
Minio in the kind cluster to act as an object repository. It can be started via:

.. code-block:: console

   $ ./hack/run-minio.sh
