============================
VolSync CLI / kubectl plugin
============================

.. toctree::
   :hidden:

   replication

VolSync provides a CLI interface to assist in performing common operations using
the VolSync operator.

All the tasks that can be accomplished via this CLI can also be performed by
directly manipulating VolSync's ReplicationSource and ReplicationDestination
objects. It is meant as a simple shortcut for common operations:

- :doc:`Setting up asynchronous data replication<replication>`
- Migrating data into Kubernetes

Installation
============

The plugin can installed via:

.. code-block:: console

    $ go install github.com/backube/volsync/kubectl-volsync@main
    go: downloading github.com/backube/volsync v0.3.1-0.20220214161039-2a78c57773a4

    $ which kubectl-volsync
    ~/go/bin/kubectl-volsync

Assuming that the above installation directory is in your ``PATH``, the VolSync CLI will be available as a sub-command of ``kubectl`` or ``oc``:

.. code-block:: console

    $ kubectl volsync --help
    This plugin can be used to configure replication relationships using the
    VolSync operator.

    The plugin has a number of sub-commands that are organized based on common
    data movement tasks such as:

      *  Creating a cross-cluster data replication relationship
      *  Migrating data into a Kubernetes cluster
      *  Establishing a simple PV backup schedule

    Usage:
      kubectl-volsync [command]

    Available Commands:
      completion  generate the autocompletion script for the specified shell
      help        Help about any command
      migration   Migrate data into a PersistentVolume
      replication Replicate data between two PersistentVolumes
