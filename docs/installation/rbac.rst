================
RBAC permissions
================

Once the VolSync operator has been installed, it is ready for use in the
cluster, but only those with cluster administrator privileges have permission to
use it.

In order for the operator to be used, it is necessary to have the ability to
access VolSync's ReplicationSource and ReplicationDestination custom resource
objects. It is recommended that users be allowed to manage data replication
within the namespaces that they are assigned. This enables "self-service" data
protection for the cluster's users.

The below RBAC rules give users access to VolSync's CRs within the namespaces
that they manage. It also grants access to VolumeSnapshot objects so that users
can easily "promote" the latest destination snapshot, if necessary, during
recovery/fail-over.

.. code-block:: yaml
    :caption: volsync-rbac.yaml

    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: volsync-edit
      labels:
        # Grant access to namespace admins
        rbac.authorization.k8s.io/aggregate-to-admin: "true"
        # Grant access to namespace editors
        rbac.authorization.k8s.io/aggregate-to-edit: "true"
    rules:
      # Give users full control of ReplicationSource and ReplicationDestination
      # objects so they can manage data replication
      - apiGroups:
          - volsync.backube
        resources:
          - replicationdestinations
          - replicationsources
        verbs:
          - create
          - delete
          - deletecollection
          - get
          - list
          - patch
          - update
          - watch
      - apiGroups:
          - volsync.backube
        resources:
          - replicationdestinations/status
          - replicationsources/status
        verbs:
          - get
          - list
          - watch
      # Give users the ability to view VolumeSnapshots so they can "promote" the
      # destination snapshots into usable PVCs
      - apiGroups:
          - snapshot.storage.k8s.io
        resources:
          - volumesnapshots
          - volumesnapshots/status
        verbs:
          - get
          - list
          - watch

    ---
    apiVersion: rbac.authorization.k8s.io/v1
    kind: ClusterRole
    metadata:
      name: volsync-view
      labels:
        # Grant access to namespace viewers
        rbac.authorization.k8s.io/aggregate-to-view: "true"
    rules:
      # Give users read access to ReplicationSource and ReplicationDestination
      # objects so they can monitor data replication
      - apiGroups:
          - volsync.backube
        resources:
          - replicationdestinations
          - replicationsources
          - replicationdestinations/status
          - replicationsources/status
        verbs:
          - get
          - list
          - watch
      # Give users the ability to monitor (destination) VolumeSnapshots
      - apiGroups:
          - snapshot.storage.k8s.io
        resources:
          - volumesnapshots
          - volumesnapshots/status
        verbs:
          - get
          - list
          - watch
