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
RustFS in the kind cluster as a disposable S3-compatible object repository. It
can be started via the script below. This fixture is for local development and
testing; it is not a claim of production S3 compatibility.

.. code-block:: console

   $ ./hack/run-rustfs.sh

The script installs the pinned RustFS Helm chart and server, then initializes
the ``restic-e2e`` bucket with a pinned RustFS CLI image. To enable HTTPS with a
self-signed certificate for custom-CA testing, run:

.. code-block:: console

   $ RUSTFS_NAMESPACE=rustfs-tls RUSTFS_USE_TLS=1 ./hack/run-rustfs.sh

The installer discovers the cluster's default StorageClass (``csi-hostpath-sc``
in the Kind setup); set ``RUSTFS_STORAGE_CLASS`` to override it. If the cluster
has no default or has multiple defaults, specify the override explicitly.

The installer prefers the repository-pinned Helm at ``bin/helm``; run
``make helm`` to download it, or set ``HELM_BIN`` to another Helm 3 or 4
executable. HTTP installs need no post-renderer. TLS installs use Helm 3's
executable post-renderer or Helm 4's temporary ``postrenderer/v1`` subprocess
plugin. The HTTPS fixture also requires OpenSSL and Python 3.

On OpenShift, the installer leaves UID/GID and ``fsGroup`` assignment to the
restricted SCC while keeping the server and bucket-init Job security settings
explicit. On other clusters it uses UID/GID and ``fsGroup`` 10001 for writable
volumes.
