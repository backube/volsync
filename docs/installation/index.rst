============
Installation
============

.. toctree::
   :hidden:

   development
   rbac

The following directions will walk through the process of deploying VolSync.

.. note::
   Volume snapshot and clone capabilities are required for some VolSync
   functionality. It is recommended that you use a CSI driver and StorageClass
   capable of snapshotting and cloning volumes.

There are several methods for installing VolSync. Choose the option that relates to
your situation.

.. warning::
   VolSync requires the Kubernetes snapshot controller to be installed
   within a cluster. If the controller is not deployed review the
   snapshot controller documentation https://github.com/kubernetes-csi/external-snapshotter.

Kubernetes & OpenShift
======================

The recommended method for deploying VolSync is via its `Helm
chart <https://artifacthub.io/packages/helm/backube-helm-charts/volsync>`_.

.. code-block:: console

   # Add the Backube Helm repo
   $ helm repo add backube https://backube.github.io/helm-charts/

   # Deploy the chart in your cluster
   $ helm install --create-namespace -n volsync-system volsync backube/volsync

Verify VolSync is running by checking the output of ``kubectl get deploy``:

.. code-block:: console

   $ kubectl -n volsync-system get deploy/volsync
   NAME      READY   UP-TO-DATE   AVAILABLE   AGE
   volsync   1/1     1            1           60s

Configuring CSI storage
-----------------------

To make the most of VolSync's capabilities, it's important that the volumes
being replicated are using CSI-based storage drivers and that volume
snapshotting is properly configured.

The currently configured StorageClasses can be viewed via:

.. code-block:: console

   $ kubectl get storageclasses

And the VolumeSnapshotClasses can be viewed via:

.. code-block:: console

   $ kubectl get volumesnapshotclasses

StorageClasses that carry the ``storageclass.kubernetes.io/is-default-class:
"true"`` and VolumeSnapshotClasses that carry the
``snapshot.storage.kubernetes.io/is-default-class: "true"`` annotations are
marked as the defaults on the cluster, meaning that if the class is not
specified, these defaults will be used. However, it is not necessary to set or
modify the default on your cluster since the classes can be specified directly
in the ReplicationSource and ReplicationDestination objects used by VolSync.

Below are examples of configured CSI storage on a few different cloud platforms.
Your configuration may be different.

.. tabs::

   .. group-tab:: AWS

      The EBS CSI driver on AWS-based clusters is usually named ``gp2-csi`` or
      ``gp3-csi``.

      .. code-block:: console

         # List StorageClasses
         $ kubectl get storageclasses
         NAME            PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
         gp2 (default)   kubernetes.io/aws-ebs   Delete          WaitForFirstConsumer   true                   25m
         gp2-csi         ebs.csi.aws.com         Delete          WaitForFirstConsumer   true                   25m
         gp3-csi         ebs.csi.aws.com         Delete          WaitForFirstConsumer   true                   25m


         # View details of the gp2-csi SC
         $ kubectl get storageclass/gp2-csi -oyaml
         allowVolumeExpansion: true
         apiVersion: storage.k8s.io/v1
         kind: StorageClass
         metadata:
            creationTimestamp: "2022-02-08T14:03:20Z"
            name: gp2-csi
            resourceVersion: "5288"
            uid: 24d2cee6-1346-4c3e-8742-39dec08e3e50
         parameters:
            encrypted: "true"
            type: gp2
         provisioner: ebs.csi.aws.com
         reclaimPolicy: Delete
         volumeBindingMode: WaitForFirstConsumer

   .. group-tab:: Azure

      The CSI driver on Azure-based clusters is usually named ``managed-csi``.

      .. code-block:: console

         # List StorageClasses
         $ kubectl get storageclasses
         NAME                        PROVISIONER                RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
         managed-csi                 disk.csi.azure.com         Delete          WaitForFirstConsumer   true                   45m
         managed-premium (default)   kubernetes.io/azure-disk   Delete          WaitForFirstConsumer   true                   46m

         # View details of the managed-csi SC
         $ kubectl get storageclass/managed-csi -oyaml
         allowVolumeExpansion: true
         apiVersion: storage.k8s.io/v1
         kind: StorageClass
         metadata:
            creationTimestamp: "2022-02-08T14:57:23Z"
            name: managed-csi
            resourceVersion: "5853"
            uid: 3aeba0d1-6c52-481c-9dc1-786ae84a2f7b
         parameters:
            skuname: Premium_LRS
         provisioner: disk.csi.azure.com
         reclaimPolicy: Delete
         volumeBindingMode: WaitForFirstConsumer

   .. group-tab:: GCP

      The CSI driver on GCP-based clusters is usually named ``standard-csi``.

      .. code-block:: console

         # List StorageClasses
         $ kubectl get storageclasses
         NAME                 PROVISIONER             RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
         standard (default)   kubernetes.io/gce-pd    Delete          WaitForFirstConsumer   true                   15m
         standard-csi         pd.csi.storage.gke.io   Delete          WaitForFirstConsumer   true                   15m


         # View details of the standard-csi SC
         $ kubectl get storageclass/standard-csi -oyaml
         allowVolumeExpansion: true
         apiVersion: storage.k8s.io/v1
         kind: StorageClass
         metadata:
            creationTimestamp: "2022-02-08T13:24:53Z"
            name: standard-csi
            resourceVersion: "5976"
            uid: 066a43fc-798f-49a7-b62a-0350e8946364
         parameters:
            replication-type: none
            type: pd-standard
         provisioner: pd.csi.storage.gke.io
         reclaimPolicy: Delete
         volumeBindingMode: WaitForFirstConsumer

   .. group-tab:: vSphere

      The CSI driver on vSphere-based clusters is usually named ``thin-csi``.

      .. code-block:: console

         # List StorageClasses
         $ kubectl get storageclasses
         NAME             PROVISIONER                    RECLAIMPOLICY   VOLUMEBINDINGMODE      ALLOWVOLUMEEXPANSION   AGE
         thin (default)   kubernetes.io/vsphere-volume   Delete          Immediate              false                  20m
         thin-csi         csi.vsphere.vmware.com         Delete          WaitForFirstConsumer   true                   18m

         # View details of the thin-csi SC
         $ kubectl get storageclass/thin-csi -oyaml
         allowVolumeExpansion: true
         apiVersion: storage.k8s.io/v1
         kind: StorageClass
         metadata:
            creationTimestamp: "2022-02-08T16:48:52Z"
            name: thin-csi
            resourceVersion: "9789"
            uid: 80d45374-8447-47eb-950c-2568af070d6e
         parameters:
            StoragePolicyName: openshift-storage-policy-ci-ln-54d2r5t-c1627-jvkws
         provisioner: csi.vsphere.vmware.com
         reclaimPolicy: Delete
         volumeBindingMode: WaitForFirstConsumer

You should also verify the presence of a corresponding
VolumeSnapshotClass. Note that the name of the SC and VSC do not need to
be the same, but the provisioner/driver should be.

.. tabs::

   .. group-tab:: AWS

      .. code-block:: console

         # List VolumeSnapshotClasses
         $ kubectl get volumesnapshotclasses
         NAME          DRIVER            DELETIONPOLICY   AGE
         csi-aws-vsc   ebs.csi.aws.com   Delete           23m


         # View details of the csi-aws-vsc VSC
         $ kubectl get volumesnapshotclass/csi-aws-vsc -oyaml
         apiVersion: snapshot.storage.k8s.io/v1
         deletionPolicy: Delete
         driver: ebs.csi.aws.com
         kind: VolumeSnapshotClass
         metadata:
            annotations:
               snapshot.storage.kubernetes.io/is-default-class: "true"
            creationTimestamp: "2022-02-08T14:03:20Z"
            generation: 1
            name: csi-aws-vsc
            resourceVersion: "5301"
            uid: d990af7b-d2ae-4a49-8cfe-fd5ae93902df

      .. important::

         **The AWS EBS CSI driver does not support volume cloning.** When
         configuring replication with VolSync, be sure to choose a
         ``copyMethod`` of ``Snapshot`` for the source volume. Choosing
         ``Clone`` will not work.

   .. group-tab:: Azure

      .. code-block:: console

         # List VolumeSnapshotClasses
         $ kubectl get volumesnapshotclasses
         NAME                DRIVER               DELETIONPOLICY   AGE
         csi-azuredisk-vsc   disk.csi.azure.com   Delete           48m

         # View details of the csi-azuredisk-vsc VSC
         $ kubectl get volumesnapshotclass/csi-azuredisk-vsc -oyaml
         apiVersion: snapshot.storage.k8s.io/v1
         deletionPolicy: Delete
         driver: disk.csi.azure.com
         kind: VolumeSnapshotClass
         metadata:
            annotations:
               snapshot.storage.kubernetes.io/is-default-class: "true"
            creationTimestamp: "2022-02-08T14:57:23Z"
            generation: 1
            name: csi-azuredisk-vsc
            resourceVersion: "5847"
            uid: 1d105f8c-4e49-48e1-8ead-927f90f4bb2e
         parameters:
            incremental: "true"

   .. group-tab:: GCP

      .. code-block:: console

         # List VolumeSnapshotClasses
         $ kubectl get volumesnapshotclasses
         NAME             DRIVER                  DELETIONPOLICY   AGE
         csi-gce-pd-vsc   pd.csi.storage.gke.io   Delete           17m


         # View details of the csi-gce-pd-vsc VSC
         $ kubectl get volumesnapshotclass/csi-gce-pd-vsc -oyaml
         apiVersion: snapshot.storage.k8s.io/v1
         deletionPolicy: Delete
         driver: pd.csi.storage.gke.io
         kind: VolumeSnapshotClass
         metadata:
            annotations:
               snapshot.storage.kubernetes.io/is-default-class: "true"
            creationTimestamp: "2022-02-08T13:24:53Z"
            generation: 1
            name: csi-gce-pd-vsc
            resourceVersion: "5981"
            uid: 886de96d-820c-403b-8570-fcfb37939532

   .. group-tab:: vSphere

      At this time (Feb 2022), volume snapshotting is an alpha feature in the
      vSphere CSI driver and not enabled by default. If you are interested in
      trying it out, please consult VMware's documentation.

Next, consider :doc:`granting users access to VolSync's custom resources <rbac>`
so that they can manage their own data replication.

Continue to the :doc:`usage docs </usage/index>`.
