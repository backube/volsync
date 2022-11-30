========================================
Migrate MySQL w/ TLS-based rsync
========================================

.. toctree::
   :hidden:

.. sidebar:: Contents

   .. contents:: Migration of MySQL DB
      :local:

This tutorial will show how to migrate a MySQL database using VolSync's
rsync-tls data mover.

Environment
===========

This example will walk through deploying, setting up replication, then migrating
MySQL between two OpenShift clusters in AWS.

Prerequisites:

- Access to two OpenShift clusters (``east2`` and ``west2``). Optional: just use
  different Namespaces in the same cluster
- ``kubectl`` access to the cluster(s).
- VolSync is installed in both clusters
- Working installation of Helm
- A CSI-based StorageClass for the database's persistent data.
- The persistent storage used for the database will come from EBS, using the
  ebs-csi driver:

  .. code-block:: console

    $ kubectl --context east2 get storageclass/gp3-csi -oyaml
    allowVolumeExpansion: true
    apiVersion: storage.k8s.io/v1
    kind: StorageClass
    metadata:
      annotations:
        storageclass.kubernetes.io/is-default-class: "true"
      creationTimestamp: "2022-12-01T13:18:24Z"
      name: gp3-csi
      resourceVersion: "5233"
      uid: b7a8872e-f356-4498-8324-7d11061bc043
    parameters:
      encrypted: "true"
      type: gp3
    provisioner: ebs.csi.aws.com
    reclaimPolicy: Delete
    volumeBindingMode: WaitForFirstConsumer


Deploy MySQL
============

The Bitnami Helm chart for MySQL will be used to deploy the database. Start by
adding the Bitnami Helm repo:

.. code-block:: console

  $ helm repo add bitnami https://charts.bitnami.com/bitnami
  helm repo add bitnami https://charts.bitnami.com/bitnami

Then install the mysql chart into the ``mysql`` namespace:

.. code-block:: console

  $ helm install --kube-context east2 --create-namespace --namespace mysql \
    --set auth.rootPassword=mypassword \
    --set primary.podSecurityContext.enabled=false \
    --set primary.containerSecurityContext.enabled=false \
    --set primary.persistence.storageClass=gp3-csi \
    mysql bitnami/mysql
  NAME: mysql
  LAST DEPLOYED: Thu Dec  1 11:46:59 2022
  NAMESPACE: mysql
  STATUS: deployed
  REVISION: 1
  TEST SUITE: None
  NOTES:
  CHART NAME: mysql
  CHART VERSION: 9.4.4
  APP VERSION: 8.0.31

  ** Please be patient while the chart is being deployed **

  Tip:

    Watch the deployment status using the command: kubectl get pods -w --namespace mysql

  Services:

    echo Primary: mysql.mysql.svc.cluster.local:3306

  Execute the following to get the administrator credentials:

    echo Username: root
    MYSQL_ROOT_PASSWORD=$(kubectl get secret --namespace mysql mysql -o jsonpath="{.data.mysql-root-password}" | base64 -d)

  To connect to your database:

    1. Run a pod that you can use as a client:

        kubectl run mysql-client --rm --tty -i --restart='Never' --image  docker.io/bitnami/mysql:8.0.31-debian-11-r10 --namespace mysql --env MYSQL_ROOT_PASSWORD=$MYSQL_ROOT_PASSWORD --command -- bash

    2. To connect to primary service (read/write):

        mysql -h mysql.mysql.svc.cluster.local -uroot -p"$MYSQL_ROOT_PASSWORD"

Ensure that the database has successfully deployed:

.. code-block:: console

  $ kubectl --context east2 -n mysql get all,pvc
  NAME          READY   STATUS    RESTARTS   AGE
  pod/mysql-0   1/1     Running   0          3m32s

  NAME                     TYPE        CLUSTER-IP    EXTERNAL-IP   PORT(S)    AGE
  service/mysql            ClusterIP   172.30.81.8   <none>        3306/TCP   3m32s
  service/mysql-headless   ClusterIP   None          <none>        3306/TCP   3m32s

  NAME                     READY   AGE
  statefulset.apps/mysql   1/1     3m32s

  NAME                                 STATUS   VOLUME                                     CAPACITY   ACCESS MODES   STORAGECLASS   AGE
  persistentvolumeclaim/data-mysql-0   Bound    pvc-e90001a9-8493-45cc-87af-f9b4aefa790d   8Gi        RWO            gp3-csi        3m32s

Create a destination Namespace
==============================

Create a corresponding Namespace on the destination (west2) cluster:

.. code-block:: console

  $ kubectl --context west2 create ns mysql
  namespace/mysql created

Create a key to secure the replication
======================================

Generate a shared key for the replication relationship:

.. code-block:: console

  $ KEY=1:$(openssl rand -hex 32)

Add it as a Secret in the ``mysql`` Namespace on both clusters:

.. code-block:: console

  $ kubectl --context east2 -n mysql create secret generic east-west --from-literal psk.txt=$KEY
  secret/east-west created
  $ kubectl --context west2 -n mysql create secret generic east-west --from-literal psk.txt=$KEY
  secret/east-west created

Create the ReplicationDestination
=================================

This step creates the ReplicationDestination on the west cluster. Since there is
no network tunnel between the clusters, you will use a LoadBalancer Service so
that the source (east) cluster will be able to connect. You will use a volume
size that matches the source (8Gi).

.. code-block:: console

  $ kubectl --context west2 -n mysql create -f - <<EOF
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationDestination
  metadata:
    name: mysql
  spec:
    rsyncTLS:
      copyMethod: Snapshot
      capacity: 8Gi
      accessModes: ["ReadWriteOnce"]
      storageClassName: gp3-csi
      volumeSnapshotClassName: csi-aws-vsc
      keySecret: east-west
      serviceType: LoadBalancer
  EOF
  replicationdestination.volsync.backube/mysql created

Verify that the destination has been properly created by looking at the new
ReplicationDestination object:

.. code-block:: console

  $ kubectl --context west2 -n mysql get replicationdestination/mysql -oyaml
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationDestination
  metadata:
    creationTimestamp: "2022-12-06T13:31:38Z"
    generation: 1
    name: mysql
    namespace: mysql
    resourceVersion: "302334"
    uid: f3149aee-c399-462b-ac92-63ff756f4da8
  spec:
    rsyncTLS:
      accessModes:
      - ReadWriteOnce
      capacity: 8Gi
      copyMethod: Snapshot
      keySecret: east-west
      serviceType: LoadBalancer
      storageClassName: gp3-csi
      volumeSnapshotClassName: csi-aws-vsc
  status:
    conditions:
    - lastTransitionTime: "2022-12-06T13:31:38Z"
      message: Synchronization in-progress
      reason: SyncInProgress
      status: "True"
      type: Synchronizing
    lastSyncStartTime: "2022-12-06T13:31:38Z"
    rsyncTLS:
      address: abad73aa2ca4441ed8c9e13f1095c453-95258c1d3ff95327.elb.us-west-2.amazonaws.com

Above, the ``.status.conditions.Synchronizing`` status is ``True``, and the
external address is available in ``.status.rsyncTLS.address``. This is the
address you will use to configure the source. It may take a few minutes for the
LoadBalancer address to be assigned and appear in the status.

Create the ReplicationSource
============================

On the east cluster, you will create the corresponding ReplicationSource. Since
the EBS CSI driver does not support volume cloning, you will need to specify a
``copyMethod`` of ``Snapshot``.

Create the source:

.. code-block:: console

  $ kubectl --context east2 -n mysql create -f - <<EOF
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: mysql
  spec:
    sourcePVC: data-mysql-0
    trigger:
      schedule: "*/5 * * * *"
    rsyncTLS:
      keySecret: east-west
      address: abad73aa2ca4441ed8c9e13f1095c453-95258c1d3ff95327.elb.us-west-2.amazonaws.com
      copyMethod: Snapshot
      volumeSnapshotClassName: csi-aws-vsc
  EOF
  replicationsource.volsync.backube/mysql created

Verify that the source has been created:

.. code-block:: console

  $ kubectl --context east2 -n mysql get replicationsource/mysql -oyaml
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    creationTimestamp: "2022-12-06T13:33:51Z"
    generation: 1
    name: mysql
    namespace: mysql
    resourceVersion: "438380"
    uid: 5721c629-a1d9-447e-a009-92d752344b3f
  spec:
    rsyncTLS:
      address: abad73aa2ca4441ed8c9e13f1095c453-95258c1d3ff95327.elb.us-west-2.amazonaws.com
      copyMethod: Snapshot
      keySecret: east-west
      volumeSnapshotClassName: csi-aws-vsc
    sourcePVC: data-mysql-0
    trigger:
      schedule: '*/5 * * * *'
  status:
    conditions:
    - lastTransitionTime: "2022-12-06T13:50:21Z"
      message: Waiting for next scheduled synchronization
      reason: WaitingForSchedule
      status: "False"
      type: Synchronizing
    lastSyncDuration: 21.447114717s
    lastSyncTime: "2022-12-06T13:50:21Z"
    nextSyncTime: "2022-12-06T13:55:00Z"
    rsyncTLS: {}

Above, you can see that synchronization has completed (``lastSyncTime`` is set),
and the replication took 21 seconds (``lastSyncDuration``).

Next, you will log into the database and make changes.

Log in to the database
======================

Exec into the pod, using the password set during deployment (``mypassword``):

.. code-block:: console

  $ kubectl --context east2 -n mysql exec -it sts/mysql -- mysql -u root -p
  Enter password:
  Welcome to the MySQL monitor.  Commands end with ; or \g.
  Your MySQL connection id is 660
  Server version: 8.0.31 Source distribution

  Copyright (c) 2000, 2022, Oracle and/or its affiliates.

  Oracle is a registered trademark of Oracle Corporation and/or its
  affiliates. Other names may be trademarks of their respective
  owners.

  Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

  mysql>

Add a new database
==================

List the current databases with the ``show`` command:

.. code-block:: console

  mysql> show databases;
  +--------------------+
  | Database           |
  +--------------------+
  | information_schema |
  | my_database        |
  | mysql              |
  | performance_schema |
  | sys                |
  +--------------------+
  5 rows in set (0.00 sec)

Use the ``create`` command to add a new database, then log out:

.. code-block:: console

  mysql> create database newdb;
  Query OK, 1 row affected (0.00 sec)

  mysql> show databases;
  +--------------------+
  | Database           |
  +--------------------+
  | information_schema |
  | my_database        |
  | mysql              |
  | newdb              |
  | performance_schema |
  | sys                |
  +--------------------+
  6 rows in set (0.00 sec)

  mysql> quit;
  Bye

Wait for the changes to replicate
=================================

The system is configured to replicate every 5 minutes, so check the current
time, then wait for the next synchronization interval to run:

.. code-block:: console

  $ date
  Tue Dec  6 08:59:08 AM EST 2022

  $ kubectl --context east2 -n mysql get replicationsource/mysql -oyaml
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    creationTimestamp: "2022-12-06T13:33:51Z"
    generation: 1
    name: mysql
    namespace: mysql
    resourceVersion: "442579"
    uid: 5721c629-a1d9-447e-a009-92d752344b3f
  spec:
    rsyncTLS:
      address: abad73aa2ca4441ed8c9e13f1095c453-95258c1d3ff95327.elb.us-west-2.amazonaws.com
      copyMethod: Snapshot
      keySecret: east-west
      volumeSnapshotClassName: csi-aws-vsc
    sourcePVC: data-mysql-0
    trigger:
      schedule: '*/5 * * * *'
  status:
    conditions:
    - lastTransitionTime: "2022-12-06T14:00:22Z"
      message: Waiting for next scheduled synchronization
      reason: WaitingForSchedule
      status: "False"
      type: Synchronizing
    lastSyncDuration: 22.094321374s
    lastSyncTime: "2022-12-06T14:00:22Z"
    nextSyncTime: "2022-12-06T14:05:00Z"
    rsyncTLS: {}

Above, you can see that the lastSyncTime is after the time when the database
changes were made, so these changes should now be available on the destination.

The replicated data is now held in a VolumeSnapshot on the destination (west
cluster). This needs to be converted to a PVC for use by the database.

Pause replication
=================

To make it easier to restore the latest replicated image, you should pause
replication on the destination cluster.

Add ``paused: True`` to the ReplicationDestination's ``.spec`` using ``kubectl
edit``:

.. code-block:: console

  $ kubectl --context west2 -n mysql edit replicationdestination/mysql
  # ... add "paused: True", and save
  replicationdestination.volsync.backube/mysql edited

Determine the latest image
==========================

Determine the name of the snapshot that holds the most recent copy of replicated
data:

.. code-block:: console

  $ kubectl --context west2 -n mysql get replicationdestination/mysql -oyaml
  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationDestination
  metadata:
    creationTimestamp: "2022-12-06T13:31:38Z"
    generation: 2
    name: mysql
    namespace: mysql
    resourceVersion: "321738"
    uid: f3149aee-c399-462b-ac92-63ff756f4da8
  spec:
    paused: true
    rsyncTLS:
      accessModes:
      - ReadWriteOnce
      capacity: 8Gi
      copyMethod: Snapshot
      keySecret: east-west
      serviceType: LoadBalancer
      storageClassName: gp3-csi
      volumeSnapshotClassName: csi-aws-vsc
  status:
    conditions:
    - lastTransitionTime: "2022-12-06T14:15:21Z"
      message: Synchronization in-progress
      reason: SyncInProgress
      status: "True"
      type: Synchronizing
    lastSyncDuration: 5m5.367038355s
    lastSyncStartTime: "2022-12-06T14:15:21Z"
    lastSyncTime: "2022-12-06T14:15:21Z"
    latestImage:
      apiGroup: snapshot.storage.k8s.io
      kind: VolumeSnapshot
      name: volsync-mysql-dst-20221206141521
    rsyncTLS:
      address: abad73aa2ca4441ed8c9e13f1095c453-95258c1d3ff95327.elb.us-west-2.amazonaws.com

Above, you can see that the latest Snapshot is
``volsync-mysql-dst-20221206141521``.

Restore the Snapshot
====================

Create a new PVC, using the ``latestImage`` from the ReplicationDestination:

.. code-block:: console

  $ kubectl --context west2 -n mysql create -f - <<EOF
  apiVersion: v1
  kind: PersistentVolumeClaim
  metadata:
    name: restored
  spec:
    accessModes: [ReadWriteOnce]
    resources:
      requests:
        storage: 8Gi
    storageClassName: gp3-csi
    dataSource:
      apiGroup: snapshot.storage.k8s.io
      kind: VolumeSnapshot
      name: volsync-mysql-dst-20221206141521
  EOF
  persistentvolumeclaim/restored created

The PVC is now waiting for a Pod to attempt to use the data:

.. code-block:: console

  $ kubectl --context west2 -n mysql describe pvc/restored
  Name:          restored
  Namespace:     mysql
  StorageClass:  gp3-csi
  Status:        Pending
  Volume:
  Labels:        <none>
  Annotations:   <none>
  Finalizers:    [kubernetes.io/pvc-protection]
  Capacity:
  Access Modes:
  VolumeMode:    Filesystem
  DataSource:
    APIGroup:  snapshot.storage.k8s.io
    Kind:      VolumeSnapshot
    Name:      volsync-mysql-dst-20221206141521
  Used By:     <none>
  Events:
    Type    Reason                Age               From                         Message
    ----    ------                ----              ----                         -------
    Normal  WaitForFirstConsumer  6s (x2 over 20s)  persistentvolume-controller  waiting for first consumer to be created before binding

Start the database on the destination
=====================================

Deploy the MySQL Helm chart on the destination, but instead of provisioning
a new PVC for the database, you will use the one you have just restored.

.. code-block:: console

  helm install --kube-context west2 --namespace mysql \
    --set auth.rootPassword=mypassword \
    --set primary.podSecurityContext.enabled=false \
    --set primary.containerSecurityContext.enabled=false \
    --set primary.persistence.existingClaim=restored \
    mysql bitnami/mysql
  NAME: mysql
  LAST DEPLOYED: Tue Dec  6 09:26:39 2022
  NAMESPACE: mysql
  STATUS: deployed
  REVISION: 1
  TEST SUITE: None
  NOTES:
  CHART NAME: mysql
  CHART VERSION: 9.4.4
  APP VERSION: 8.0.31

  ** Please be patient while the chart is being deployed **

  Tip:

    Watch the deployment status using the command: kubectl get pods -w --namespace mysql

  Services:

    echo Primary: mysql.mysql.svc.cluster.local:3306

  Execute the following to get the administrator credentials:

    echo Username: root
    MYSQL_ROOT_PASSWORD=$(kubectl get secret --namespace mysql mysql -o jsonpath="{.data.mysql-root-password}" | base64 -d)

  To connect to your database:

    1. Run a pod that you can use as a client:

        kubectl run mysql-client --rm --tty -i --restart='Never' --image  docker.io/bitnami/mysql:8.0.31-debian-11-r10 --namespace mysql --env MYSQL_ROOT_PASSWORD=$MYSQL_ROOT_PASSWORD --command -- bash

    2. To connect to primary service (read/write):

        mysql -h mysql.mysql.svc.cluster.local -uroot -p"$MYSQL_ROOT_PASSWORD"

Verify the restored contents
============================

Log into the database and verify that ``newdb`` is present:

.. code-block:: console

  $ kubectl --context west2 -n mysql exec -it sts/mysql -- mysql -u root -p
  Enter password:
  Welcome to the MySQL monitor.  Commands end with ; or \g.
  Your MySQL connection id is 34
  Server version: 8.0.31 Source distribution

  Copyright (c) 2000, 2022, Oracle and/or its affiliates.

  Oracle is a registered trademark of Oracle Corporation and/or its
  affiliates. Other names may be trademarks of their respective
  owners.

  Type 'help;' or '\h' for help. Type '\c' to clear the current input statement.

  mysql> show databases;
  +--------------------+
  | Database           |
  +--------------------+
  | information_schema |
  | my_database        |
  | mysql              |
  | newdb              |
  | performance_schema |
  | sys                |
  +--------------------+
  6 rows in set (0.00 sec)

  mysql> quit;
  Bye

Congratulations, you have just migrated the database to the west cluster!
