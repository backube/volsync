===================
RWO volume affinity
===================

.. sidebar:: Contents

   .. contents:: RWO volume affinity
      :local:

This document presents the design for co-scheduling data movers with application
Pods so that ReadWriteOnce volumes can be live-replicated.

Problem
=======

It is sometimes desirable to configure VolSync to sync data (ReplicationSource)
from a "live" volume (i.e., one that is currently being used by the
application). This corresponds to a ``copyMethod: Direct`` setting in the
ReplicationSource. Some scenarios where this is useful include:

- When the CSI driver backing the source volume does not support clones or
  snapshots
- If the CSI driver is very inefficient at clone/snapshot objects (i.e., it
  internally performs a full data copy)
- If the source volume is not backed by a CSI driver

If the volume to be replicated has an ``accessMode`` of ``ReadWriteMany`` (RWX),
the live volume can easily be replicated since there are no problems with the
application and the VolSync mover accessing the same PVC simultaneously.

However, in the case of ``ReadWriteOnce`` (RWO) volumes, the PVC may only be
accessed by a single Node at a time. If multiple Pods are to simultaneously
access the volume, they must be co-scheduled to the same Node. Unfortunately,
the Kubernetes scheduler does not take this into account when scheduling Pods.
The result is that the VolSync mover pod is unlikely to be assigned to the same
node as the primary application. This will result in it failing to start since
the PVC will not be able to be mounted.

Approach
========

In cases where the ReplicationSource is configured with ``copyMethod: Direct``
and the ``sourcePVC`` has ``accessMode: ReadWriteOnce``, VolSync should ensure
the mover Pod is placed on the same Node as the primary workload. The below
discussion only applies to such cases; all others will not be intentionally
co-scheduled.

Finding Pods
------------

Given a ReplicationSource and its associated ``sourcePVC``, it is necessary to
locate any Pods that are using the PVC. Unfortunately, there is no direct way to
locate the Pod(s) directly from the PVC object.

Since PVCs are namespaced, it is guaranteed that any users of the PVC reside
within that same Namespace. We need to list all Pods in the Namespace and search
their ``.spec.volumes[]`` list to determine whether it contains a
``persistentVolumeClaim.name`` that matches ``sourcePVC``. A number of scenarios
are possible:

No Pods are found to be using the PVC
  The mover can be scheduled without concern for affinity.
Exactly one Pod is using the PVC
  The mover should be scheduled to the same Node.
Multiple Pods are using the PVC
  If multiple Pods are **successfully** using the PVC, they must be scheduled to
  the same node. Therefore, any Pod that is currently in the ``.status.phase:
  Running`` can be used to determine the proper Node for scheduling purposes. It
  is possible that other Pods are *attempting to use* the PVC but are
  ``Pending`` because they cannot mount the volume. To handle this case, when
  looking for matching Pods, preference must be given to the ``Running`` Pods.

Co-scheduling the mover
-----------------------

Given the name of a Pod with which the mover needs to be co-scheduled, the
scheduling can be handled by directly assigning the mover to the same node. The
current node for the application can be read from ``.spec.nodeName``. This can
be copied into the mover's ``.spec.nodeName`` (within the Job template). By
directly specifying the node name, it will skip the scheduling pass and be
directly picked up by kubelet on the named node.

In addition to directly specifying the name of the Node, it is important that
the mover pod have the same set of tolerations as the application Pod to ensure
it has access to the same set of Nodes. This can be handled by directly copying
the list of tolerations from ``.spec.tolerations`` to the mover.

Scheduling changes
------------------

The application's Pod(s) can be rescheduled for a number of reasons, and VolSync
must be able to adapt in order to avoid interfering with the application. To
this end, it is necessary to periodically re-scan the Pods and adjust the mover
placement appropriately. These changes follow the logic documented above,
potentially adding, removing, or changing the `.spec.nodeName` field in the Job
template.

Limitations
===========

These are some limitations of the proposed approach.

Resource constraints
--------------------

The Node that is being used by the application may not have sufficient resources
to run the mover Pod. This will prevent the mover from starting until resources
become available (if ever). The only way to handle this would be to re-schedule
the application Pod to a Node that has more free resources. Adjusting the
application in this way is beyond the scope of VolSync.

Is there a way that we could allow the user to reliably intervene?

Interrupting the mover w/ rescheduling
--------------------------------------

While it is important to ensure that VolSync does not prevent the application
from running, there is a trade-off between responding to changes and needlessly
interrupting the mover. In this design, we err on the side of interrupting the
mover. Since synchronization cycles are typically short, restarting the mover is
unlikely to lose much work. Additionally, application restarts are expected to
be rare, further lowering the cost.

While we could choose to allow the mover to run to completion prior to updating
the node & tolerations, there would need to be special cases for long-running
movers like Syncthing.

Rescheduling delay
------------------

It will be necessary for VolSync to periodically re-reconcile while movement is
InProgress so that updates to the scheduling can be detected and performed. This
is planned to be handled in a time-based manner as opposed to setting up a Watch
on Pods. This can potentially introduce a scheduling delay for the application
of up to the re-reconcile interval (e.g., 1 minute) in cases where the mover is
running and the application gets (re)scheduled.

This may be able to be handled via a Watch on the application Pod. However, it
would be necessary to annotate the application Pod. It's unclear how feasible
this would be.
