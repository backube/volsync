========================
Asynchronous replication
========================

.. code-block:: console

    $ kubectl volsync replication
    Replicate the contents of one PersistentVolume to another.

    This set of commands is designed to set up and manage a replication
    relationship between two different PVCs in the same Namespace, across
    Namespaces, or in different clusters. The contents of the volume can be
    replicated either on-demand or based on a provided schedule.

    Usage:
      kubectl-volsync replication [command]

    Available Commands:
      create          Create a new replication relationship
      delete          Delete an existing replication relationship
      schedule        Set replication schedule for the relationship
      set-destination Set the destination of the replication
      set-source      Set the source of the replication
      sync            Run a single synchronization

Example usage
=============

.. contents:: Example steps
   :local:

The following example uses the ``kubectl volsync replication`` subcommand to set
up and manage cross-cluster asynchronous replication of a PVC.

Application
  A simple busybox pod that has a PVC attached
Source cluster
  Kind cluster running on a local laptop using the hostpath CSI driver
Destination cluster
  OpenShift running on GCP with their CSI driver

Kubectl configuration
---------------------

The following steps assume that you have a kubeconfig defined that will allow
access to both clusters (source and destination) by switching between contexts.

.. code-block:: console

    $ kubectl config get-contexts
    CURRENT   NAME        CLUSTER                  AUTHINFO     NAMESPACE
    *         gcp         ci-ln-nm63l9k-72292      admin
              kind        kind-kind                kind-kind

A configuration like the above will allow directing requests to the different
clusters via ``kubectl --context <name>``. Likewise, some of the VolSync CLI
commands will refer to this context name (e.g.,
``<context>/<namespace>/<resource>``).

Please see `the Kubernetes documentation
<https://kubernetes.io/docs/tasks/access-application-cluster/configure-access-multiple-clusters/>`_
for details on how to set up your kubeconfig to access multiple clusters.

Deploy the application
----------------------

The application is a simple busybox pod and an attached PVC.

.. code-block:: yaml
   :caption: pvc.yaml

    ---
    kind: PersistentVolumeClaim
    apiVersion: v1
    metadata:
      name: datavol
    spec:
      storageClassName: csi-hostpath-sc
      accessModes:
        - ReadWriteOnce
      resources:
        requests:
          storage: 3Gi

.. code-block:: yaml
   :caption: pod.yaml

    ---
    kind: Pod
    apiVersion: v1
    metadata:
      name: busybox
    spec:
      containers:
        - name: ubi
          image: busybox
          command: ["/bin/sh", "-c"]
          args: ["sleep 999999"]
          volumeMounts:
            - name: data
              mountPath: "/mnt"
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: datavol

Create the namespace and application objects:

.. code-block:: console

   $ kubectl --context kind create ns source
   namespace/source created

   $ kubectl --context kind -n source create -f pvc.yaml
   persistentvolumeclaim/datavol created

   $ kubectl --context kind -n source create -f pod.yaml
   pod/busybox created

Set up replication
------------------

Create a replication relationship. We are naming the relationship "example":

.. code-block:: console

   $ kubectl volsync replication -r example create

Set the source of the replication:

- The hostpath CSI driver supports volume cloning, so we'll use "Clone" as our
  method to create a point-in-time copy
- The name of the PVC to replicate is given as ``<cluster-context>/<namespace>/<name>``

.. code-block:: console

   $ kubectl volsync replication -r example set-source --copymethod Clone --pvcname kind/source/datavol

Set the destination:

.. code-block:: console

   # Create a namespace on the destination cluster
   $ kubectl --context gcp create ns destns
   namespace/destns created

   $ kubectl volsync replication -r example set-destination --copymethod Snapshot --storageclass standard-csi --volumesnapshotclass csi-gce-pd-vsc --servicetype LoadBalancer --destination gcp/destns/datavol

Begin replicating on a 5 minute schedule:

.. code-block:: console

   $ kubectl volsync replication -r example schedule --cronspec '*/5 * * * *'
   I0216 13:51:22.165811  275823 replication.go:381] waiting for keys & address of destination to be available
   I0216 13:51:32.296465  275823 replication.go:406] creating resources on Source

Examining VolSync resources
---------------------------

The above commands deployed a ReplicationSource and ReplicationDestination object on the two clusters:

.. code-block:: console

    $ kubectl --context kind -n source get replicationsource -oyaml
    apiVersion: v1
    items:
    - apiVersion: volsync.backube/v1alpha1
      kind: ReplicationSource
      metadata:
        creationTimestamp: "2022-02-16T20:07:30Z"
        generation: 1
        labels:
          volsync.backube/relationship: 90d56bef-551d-4ede-b6a7-0783cabdafb6
        name: datavol-87srf
        namespace: source
        resourceVersion: "13695"
        uid: 7511b291-b768-4a2e-96cf-2eafd3854469
      spec:
        rsync:
          address: 34.121.93.205
          copyMethod: Clone
          sshKeys: datavol-87srf
        sourcePVC: datavol
        trigger:
          schedule: '*/5 * * * *'
      status:
        conditions:
        - lastTransitionTime: "2022-02-16T20:07:58Z"
          message: Waiting for next scheduled synchronization
          reason: WaitingForSchedule
          status: "False"
          type: Synchronizing
        - lastTransitionTime: "2022-02-16T20:07:30Z"
          message: Reconcile complete
          reason: ReconcileComplete
          status: "True"
          type: Reconciled
        lastSyncDuration: 28.732770544s
        lastSyncTime: "2022-02-16T20:07:58Z"
        nextSyncTime: "2022-02-16T20:10:00Z"
        rsync: {}
    kind: List
    metadata:
      resourceVersion: ""
      selfLink: ""

    $ kubectl --context gcp -n destns get replicationdestination -oyaml
    apiVersion: v1
    items:
    - apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        creationTimestamp: "2022-02-16T20:06:10Z"
        generation: 1
        labels:
          volsync.backube/relationship: 90d56bef-551d-4ede-b6a7-0783cabdafb6
        name: datavol
        namespace: destns
        resourceVersion: "42743"
        uid: 040dc4ad-6f37-43f1-9da4-b28d956f2bb7
      spec:
        rsync:
          accessModes:
          - ReadWriteOnce
          capacity: 3Gi
          copyMethod: Snapshot
          serviceType: LoadBalancer
          storageClassName: standard-csi
          volumeSnapshotClassName: csi-gce-pd-vsc
      status:
        conditions:
        - lastTransitionTime: "2022-02-16T20:06:10Z"
          message: Reconcile complete
          reason: ReconcileComplete
          status: "True"
          type: Reconciled
        - lastTransitionTime: "2022-02-16T20:08:00Z"
          message: Synchronization in-progress
          reason: SyncInProgress
          status: "True"
          type: Synchronizing
        lastSyncDuration: 1m50.209297869s
        lastSyncStartTime: "2022-02-16T20:08:00Z"
        lastSyncTime: "2022-02-16T20:08:00Z"
        latestImage:
          apiGroup: snapshot.storage.k8s.io
          kind: VolumeSnapshot
          name: volsync-datavol-dst-20220216200800
        rsync:
          address: 34.121.93.205
          sshKeys: volsync-rsync-dst-src-datavol
    kind: List
    metadata:
      resourceVersion: ""
      selfLink: ""

When creating the resources, the CLI:

- Created the ReplicationDestination
- Waited for the LoadBalancer address and SSH keys to become available
- Copied the SSH keys from the destination cluster to a Secret in the source
  cluster
- Created the ReplicationSource referencing the Secret, the remote address, and
  having the supplied cronspec schedule

Manual synchronization
----------------------

The above steps establish a replication schedule wherein the source is
periodically replicated to the destination. During planned migration events, it
is desirable to force a synchronization and synchronously wait for completion.

Assuming the CLI has been used as described above, a manual synchronization can
be triggered via:

.. code-block:: console

   $ kubectl volsync replication -r example sync
   I0216 15:19:19.832648  290779 replication.go:381] waiting for keys & address of destination to be available
   I0216 15:19:19.954913  290779 replication.go:406] creating resources on Source
   I0216 15:19:19.988886  290779 replication_sync.go:90] waiting for synchronization to complete

When this command returns, a new synchronization (and VolumeSnapshot) will have
been completed. To resume periodic synchronization, re-issue the ``kubectl
volsync replication schedule`` command.

Removing the replication
------------------------

When the replication relationship is no longer needed, it can be removed via:

.. code-block:: console

    $ kubectl volsync replication -r example delete

The above command removes the VolSync CRs and the SSH key Secret.
