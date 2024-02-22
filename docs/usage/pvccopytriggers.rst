=================================
PVC annotations for copy triggers
=================================

.. toctree::
   :hidden:

When doing a replication of a source PVC, it can be desirable to perform some operation such as a quiesce on the
application source prior to performing the replication. VolSync has not implemented any hooks in order to do this
as they can require privileges such as being able to exec into users containers.

A user can always schedule their replications themselves via manual triggers if they want to peform some automation,
but now there's also the option of using annotations on the source PVC.

With a ``ReplicationSource`` that uses a CopyMode of ``Snapshot`` or ``Clone``, it's possible to use
annotations on the Source PVC in order to coordinate when the snapshot or clone gets taken during a replication
cycle.

When VolSync schedules a synchronization for the ``ReplicationSource``, if the source has the annotation
``volsync.backube/use-copy-trigger``, then VolSync will pause before taking the Snapshot or Clone and wait for the user
to indicate/trigger that VolSync can proceed. After which VolSync will also update via pvc annotations once the Snapshot
or Clone is complete so that the user can choose to perform actions such as an unquiesce.

Source PVC annotations being used
=================================

For the user to edit/modify (VolSync will not touch these annotations):

.. code-block:: console
  :caption: User-level Source PVC annotations

  volsync.backube/use-copy-trigger
  volsync.backube/copy-trigger

For VolSync to edit/modify (Users should not modify these annotations):

.. code-block:: console
  :caption: VolSync Source PVC annotations

  volsync.backube/latest-copy-status
  volsync.backube/latest-copy-trigger

Mover support
=============

The current set of movers that support PVC annotations for copy triggers is:

- rclone
- restic
- rsync-tls
- rsync

Example Source PVC annotation coordination with VolSync
=======================================================

Here is an example scenario using a replicationsource with restic to backup a
PVC to a remote store. PVC copy triggers will be used in order to coordinate
performing actions on the application using the PVC prior to the snapshot
being taken.

.. code-block:: yaml
  :caption: Example ReplicationSource

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: test-rs
    namespace: test-ns
  spec:
    sourcePVC: data-pvc
    trigger:
      schedule: "*/30 * * * *"
    restic:
      pruneIntervalDays: 1
      repository: restic-secret
      retain:
        hourly: 3
        daily: 2
        monthly: 1
      copyMethod: Snapshot
      cacheCapacity: 1Gi

The example ``replicationsource`` will run a sync every 30 minutes and make a
snapshot of the source PVC called ``data-pvc`` every time it syncs.

.. code-block:: yaml
  :caption: Source PVC

  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-pvc
    namespace: test-ns
    annotations:
      # If this annotation is set with any value, VolSync will use copy triggers
      volsync.backube/use-copy-trigger: ""
  spec:
    accessModes: [ReadWriteOnce]
    resources:
      requests:
        storage: 10Gi

When the ``replicationsource`` runs a sync, it will see the ``volsync.backube/use-copy-trigger`` annotation
on the source PVC and pause before taking a snapshot.

VolSync will then add the ``volsync.backube/latest-copy-status`` annotation with value ``WaitingForTrigger``.

.. code-block:: yaml
  :caption: Source PVC - VolSync is waiting for copy trigger before taking snapshot or clone

  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-pvc
    namespace: test-ns
    annotations:
      volsync.backube/use-copy-trigger: ""
      volsync.backube/latest-copy-status: "WaitingForTrigger"
  ...

At this point the user can run commands to pause or quiesce their application.

.. note::
  VolSync will update the ``replicationsource`` ``status.latestMoverStatus`` with an error if the use does not set a
  copy-trigger within 10 minutes of setting the ``volsync.backube/latest-copy-status`` to ``WaitingForTrigger``.
  VolSync will keep reconciling the ``replicationsource`` however.

Now to indicate that VolSync can proceed to create a copy of the source PVC (a snapshot or clone), the user needs to
add the annotation ``volsync.backube/copy-trigger`` to a unique value.

.. code-block:: yaml
  :caption: Source PVC - User sets a new unique copy-trigger annotation on the PVC

  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-pvc
    namespace: test-ns
    annotations:
      volsync.backube/use-copy-trigger: ""
      volsync.backube/copy-trigger: "trigger-1" # User updated to unique value
      volsync.backube/latest-copy-status: "WaitingForTrigger"
  ...

VolSync will now start to make the copy of the PVC (a snapshot or clone) and update the
``volsync.backube/latest-copy-status`` to ``InProgress``.

.. code-block:: yaml
  :caption: Source PVC - VolSync proceeds to take copy (snapshot or clone)

  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-pvc
    namespace: test-ns
    annotations:
      volsync.backube/use-copy-trigger: ""
      volsync.backube/copy-trigger: "trigger-1"
      volsync.backube/latest-copy-status: "InProgress" # Snapshot is being taken
  ...

One the snapshot or clone is complete, VolSync will again update the ``volsync.backube/latest-copy-status``, this time
to ``Completed``. VolSync will also add another annotation ``volsync.backube/latest-copy-trigger`` which will match the
value of the ``volsync.backube/copy-trigger`` set by the user.

.. code-block:: yaml
  :caption: Source PVC - VolSync has completed copy (snapshot or clone)

  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-pvc
    namespace: test-ns
    annotations:
      volsync.backube/use-copy-trigger: ""
      volsync.backube/copy-trigger: "trigger-1"
      volsync.backube/latest-copy-status: "Completed" # Snapshot is complete
      volsync.backube/latest-copy-trigger: "trigger-1"
  ...

VolSync will proceed to run the sync at this point, but since the copy has completed, users can now perform actions
on their application such as an unquiesce.

Next sync iteration, VolSync will again update the ``volsync.backube/latest-copy-status`` to ``WaitingForTrigger``.

.. code-block:: yaml
  :caption: Source PVC - VolSync is again waiting for copy trigger before taking snapshot or clone

  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: data-pvc
    namespace: test-ns
    annotations:
      volsync.backube/use-copy-trigger: ""
      volsync.backube/copy-trigger: "trigger-1"
      volsync.backube/latest-copy-status: "WaitingForTrigger" # The next sync is waiting for a new copy-trigger
      volsync.backube/latest-copy-trigger: "trigger-1"
  ...

VolSync will wait before making a copy of the Source PVC until the user updates
``volsync.backube/copy-trigger`` to a value that does not match the value of ``volsync.backube/latest-copy-trigger``.
