==============================
Migrating data into Kubernetes
==============================

.. code-block:: console

    $ kubectl volsync migration
    Copy data from an external file system into a Kubernetes PersistentVolume.

    This set of commands is designed to help provision a PV and copy data from a
    directory tree into that newly provisioned volume.

    Usage:
    kubectl-volsync migration [command]

    Available Commands:
    create      Create a new migration destination
    delete      Delete a new migration destination
    rsync       Rsync data from source to destination

Example Usage
=============

.. contents:: Example steps
   :local:

The following example uses the ``kubectl volsync migration`` subcommand to
migrate data from a stand-alone storage system into a Kubernetes
PersistentVolumeClaim.

External storage
  A locally mounted directory tree (could be local disk or network-attached
  storage such as NFS or GlusterFS)
Destination cluster
  OpenShift running on GCP with their CSI driver.
  Note: The VolSync operator must be installed in the destination cluster.

Create the migration destination
--------------------------------

Begin by creating a Namespace to hold the PVC (and eventually the application
that will use the data).

.. code-block:: console

    $ kubectl create ns destination
    namespace/destination created

Create a target for the data migration. If a capacity and accessModes are
provided and the PVC does not already exist, the VolSync CLI will create the
PVC. Otherwise, it will use the existing PVC.

.. code-block:: console

    $ kubectl volsync migration create -r mig-example --capacity 2Gi --accessmodes ReadWriteOnce --storageclass standard-csi --pvcname destination/mydata
    I0302 12:50:42.498947  168200 request.go:665] Waited for 1.007067079s due to client-side throttling, not priority and fairness, request: GET:https://api.ci-ln-72rwmxb-72292.origin-ci-int-gce.dev.rhcloud.com:6443/apis/project.openshift.io/v1?timeout=32s
    I0302 12:50:43.925309  168200 migration_create.go:329] pvc: "mydata" not found, creating the same
    I0302 12:50:43.974092  168200 migration_create.go:267] Namespace: "destination" is found, proceeding with the same
    I0302 12:50:44.021410  168200 migration_create.go:314] Created Destination PVC: "mydata" in NameSpace: "destination" and Cluster: "" 
    I0302 12:50:44.073745  168200 migration_create.go:357] Created ReplicationDestination: "destination-mydata-migration-dest" in Namespace: "destination" and Cluster: ""

    $ kubectl get -n destination pvc/mydata
    NAME     STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
    mydata   Bound    pvc-c9040e1f-e3dd-49e4-aa5d-194079181f55   2Gi        RWO            standard-csi   3m6s

Copy the data into the PVC
--------------------------

Once the destination has been created, we can use the CLI to transfer data into the cluster.

The data currently resides in the ``/tmp/data`` directory:

.. code-block:: console

    $ ls /tmp/data
    ./  ../  linux-4.1.51/

    $ du -sh /tmp/data
    643M	/tmp/data

Sync this data into the cluster:

.. code-block:: console

    $ kubectl volsync migration rsync -r mig-example --source /tmp/data/
    ...
    Number of files: 52,680 (reg: 49,453, dir: 3,213, link: 14)
    Number of created files: 52,680 (reg: 49,453, dir: 3,213, link: 14)
    Number of deleted files: 0
    Number of regular files transferred: 49,453
    Total file size: 556.98M bytes
    Total transferred file size: 556.97M bytes
    Literal data: 556.97M bytes
    Matched data: 0 bytes
    File list size: 524.26K
    File list generation time: 0.001 seconds
    File list transfer time: 0.000 seconds
    Total bytes sent: 150.77M
    Total bytes received: 961.29K

    sent 150.77M bytes  received 961.29K bytes  10.46M bytes/sec
    total size is 556.98M  speedup is 3.67

Incremental changes can also be transferred:

.. code-block:: console

    $ echo "hello" > /tmp/data/hi.txt

    $ kubectl volsync migration rsync -r mig-example --source /tmp/data/
    I0302 13:37:37.698258  174966 request.go:665] Waited for 1.004977118s due to client-side throttling, not priority and fairness, request: GET:https://api.ci-ln-72rwmxb-72292.origin-ci-int-gce.dev.rhcloud.com:6443/apis/snapshot.storage.k8s.io/v1beta1?timeout=32s
    I0302 13:37:39.093025  174966 migration_rsync.go:132] Extracting ReplicationDestination secrets
    I0302 13:37:39.177009  174966 migration_rsync.go:190] Migrating Data from "/tmp/data/" to "\destination\mydata"
    .d..t...... ./
    <f+++++++++ hi.txt

    Number of files: 52,681 (reg: 49,454, dir: 3,213, link: 14)
    Number of created files: 1 (reg: 1)
    Number of deleted files: 0
    Number of regular files transferred: 1
    Total file size: 556.98M bytes
    Total transferred file size: 6 bytes
    Literal data: 6 bytes
    Matched data: 0 bytes
    File list size: 0
    File list generation time: 0.001 seconds
    File list transfer time: 0.000 seconds
    Total bytes sent: 806.41K
    Total bytes received: 3.60K

    sent 806.41K bytes  received 3.60K bytes  147.28K bytes/sec
    total size is 556.98M  speedup is 687.61

Clean up
--------

Once all the data has been transferred, the VolSync destination objects can be cleaned up:

.. code-block:: console

    $ kubectl volsync migration delete -r mig-example

Use the data in-cluster
-----------------------

We can now start a pod attached to the PVC and view the data:

.. code-block:: yaml
   :caption: pod.yaml

    ---
    kind: Pod
    apiVersion: v1
    metadata:
      name: busybox
    spec:
      containers:
        - name: busybox
          image: busybox
          command: ["/bin/sh", "-c"]
          args: ["sleep 999999"]
          volumeMounts:
            - name: data
              mountPath: "/mnt"
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: mydata

.. code-block:: console

    $ kubectl -n destination apply -f pod.yaml
    pod/busybox created

    $ kubectl -n destination exec -it pod/busybox -- ls -al /mnt
    total 12
    drwx--x--x    3 101587   101587        4096 Mar  2 18:37 .
    dr-xr-xr-x    1 root     root            73 Mar  2 18:39 ..
    -rw-------    1 101587   101587           6 Mar  2 18:37 hi.txt
    drwx--x--x   23 101587   101587        4096 Mar 27  2018 linux-4.1.51

    $ kubectl -n destination exec -it pod/busybox -- du -sh /mnt
    655.4M	/mnt

