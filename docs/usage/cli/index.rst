============================
VolSync CLI / kubectl plugin
============================

.. toctree::
   :hidden:

   migration
   replication

VolSync provides a CLI interface to assist in performing common operations using
the VolSync operator.

All the tasks that can be accomplished via this CLI can also be performed by
directly manipulating VolSync's ReplicationSource and ReplicationDestination
objects. It is meant as a simple shortcut for common operations:

- :doc:`Setting up asynchronous data replication<replication>`
- :doc:`Migrating data into Kubernetes<migration>`

Installation
============

The VolSync CLI (kubectl plugin) can be installed in several ways:

- Via krew (easiest)
- Downloading the latest released binary from GitHub
- From source (requires a working golang installation)

.. tabs::

    .. tab:: Krew

        `Krew <https://krew.sigs.k8s.io/>`_ is a plugin manager for the
        ``kubectl`` command. It automates the process of downloading,
        installing, and updating kubectl plugins.

        If you have Krew installed, you can install the VolSync plugin via:

        .. code-block:: console

            # Install the VolSync plugin
            $ kubectl krew install volsync
            Updated the local copy of plugin index.
            Installing plugin: volsync
            Installed plugin: volsync
            \
            | Use this plugin:
            | 	kubectl volsync
            | Documentation:
            | 	https://github.com/backube/volsync
            /
            WARNING: You installed plugin "volsync" from the krew-index plugin repository.
              These plugins are not audited for security by the Krew maintainers.
              Run them at your own risk.

            # Use it...
            $ kubectl volsync --version
            volsync version v0.4.0+b710c5f

        The plugin can be uninstalled via:

        .. code-block:: console

            # Uninstall the VolSync plugin
            $ kubectl krew uninstall volsync
            Uninstalled plugin: volsync

        Future upgrades are also possible via ``kubectl krew upgrade volsync``.

    .. tab:: Binary release

        The plugin is available on the `VolSync Releases page
        <https://github.com/backube/volsync/releases>`_. Download the
        ``kubectl-volsync.tar.gz`` and place the included ``kubectl-volsync``
        binary into your PATH. The plugin should then be available as a
        sub-command of ``kubectl``:

        .. code-block:: console

            $ kubectl volsync --version
            volsync version v0.4.0+b710c5f

        To uninstall, just delete the ``kubectl-volsync`` binary.

    .. tab:: Source

        The plugin can be installed directly from source. This requires a
        working golang environment, but it also allows easily choosing the
        version to be installed (even the latest code from ``main``).

        The latest **Released** version can be installed via:

        .. code-block:: console

            $ go install github.com/backube/volsync/kubectl-volsync@latest
            go: downloading github.com/backube/volsync v0.4.0

            $ which kubectl-volsync
            ~/go/bin/kubectl-volsync

        The **latest code from main** can be installed via:

        .. code-block:: console

            $ go install github.com/backube/volsync/kubectl-volsync@main
            go: downloading github.com/backube/volsync v0.3.1-0.20220512205923-e33a7e4d88b6

Once installation is complete, navigate to one of the documentation sub-pages
for some CLI usage examples.
