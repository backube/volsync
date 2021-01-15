========
Overview
========

.. toctree::
   :caption: Contents
   :hidden:
   :titlesonly:
   :includehidden:

   self
   installation/index
   usage/index
   design/index

*Asynchronous volume replication for Kubernetes CSI storage*

Scribe is a Kubernetes operator that performs asynchronous replication of
persistent volumes within, or across, clusters. The replication provided by
Scribe is independent of the storage system. This allows replication to and from
storage types that don't normally support remote replication. Additionally, it
can replicate across different types (and vendors) of storage.

The project is still in the early stages, but feel free to give it a try.

To get started, see the :doc:`installation instructions <installation/index>`.

Check us out on GitHub ➡️ https://github.com/backube/scribe
