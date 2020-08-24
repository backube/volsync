======================
Configuration and CRDs
======================

This document covers the rationale for how Scribe is configured and the
structure of the CustomResourceDefinitions.

.. contents::
   :depth: 2

Representation of relationships
===============================

One of the main interaction points between users and Scribe will be centered
around configuring the replication relationships between volumes. When looking
at the :ref:`use cases <case-for-use-cases>` presented in the overview of
Scribe, there are several commonalities and differences.

Replication triggers
--------------------

Depending on the use case, the "trigger" for replication may be different. For
example, in the case of asynchronous replication for disaster recovery, it is
desirable to have the volume(s) replicated at some predictable frequency (e.g.,
every five minutes). This bounds the amount of data loss that would be incurred
during a failover. Some of the other use cases could benefit from scheduled
replication (e.g., every day at 3:00am) such as the case of replicating from
production to a testing environment. Still other cases may want the replication
to be triggered on-demand or via a webhook since it may be desirable to
replicate data once a certain action or processing has completed.

Bi-directional vs. uni-directional
----------------------------------

Use cases such as disaster recovery naturally desire the replication to be
bi-directional (i.e., reversible) so that once the primary site recovers, it can
be brought back into sync and the application transitioned back. However, many
of the other use cases only desire uni-directional replication--- the primary
will always remain so.

Further, when volumes are being actively replicated-to (i.e., they are the
secondary), they are not in a usable state. Some storage systems actively block
their usage until they are "promoted" to an active state, halting or reversing
the replication. At best, even if not blocked, the secondary should not be used
while replication is ongoing due to the potential of accessing inconsistent
data. This has implications on the representation of the "volume" within a
Kubernetes environment. For example, it is assumed that a PV/PVC, if bound, is
usable by a pod, so exposing a secondary volume as a PV/PVC pair to the user is
likely to cause confusion.

Based on the above, a clean interface for the user is likely to be one where a
primary PVC is replicated to a destination location as a uni-directional
relationship, and the secondary is not visible as a PVC until a "promotion"
action is taken.

The lack of a secondary PVC until promotion is what precludes the bi-directional
relationship. Instead, two uni-directional relationships could be created. The
second, "reverse" relationship would not initially be active since its source
PVC would not exist until a secondary volume is promoted.

Proposed CRDs
=============

Since one of the main objectives in the design is to allow storage system
specific replication methods, this must be considered when designing the CRDs
that will control replication. In order to accommodate separate release
timelines and licensing models, it is also desirable for those replication
methods to be external to the main Scribe operator. Only a baseline, general
replication method needs to be directly integrated.

To achieve the desired flexibility, the CRDs can be structured similar to the
Kubernetes `StorageClass <https://kubernetes.io/docs/concepts/storage/storage-classes/>`_ object which defines a "provisioner" and permits a set
of provisioner-specific parameters passed as an arbitrary set of key/value
strings.

With the above considerations in mind, the primary side of the replication relationship could be defined as:

.. code-block:: yaml
   :caption: CRD defining the source volume to replicate

    apiVersion: scribe/v1alpha1
    kind: Source
    metadata:
      name: myVolMirror
      namespace: myNamespace
    spec:
      # Source PVC to replicate
      source: my-pvc
      # When/how often to replicate
      trigger:
        # Cronspec for mirroring frequency or schedule
        schedule: "*/10 * * * * *"
      # Method of replication. Either built-in "rsync" or an external method
      # (e.g., "ceph.io/rbd-async")
      replicationMethod: rsync
      # Method-specific configuration parameters
      parameters:  # map[string]string
        param1: value2
    status:
      # Method-specific status
      methodStatus:  # map[string]string
        status1: value2
      conditions:  # general conditions

The secondary side is configured similarly to the primary, but without the
trigger specification:

.. code-block:: yaml
   :caption: CRD defining the replication destination

    apiVersion: scribe/v1alpha1
    kind: Destination
    metadata:
      name: myVolMirror
      namespace: myNamespace
    spec:
      replicationMethod: rsync
      parameters:
        param1: value2
    status:
      methodStatus:
        status1: value2
      conditions:
