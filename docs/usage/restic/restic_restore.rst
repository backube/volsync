Restic backup
-------------

To restore from the backup, create a destination, deploy ``restic-config`` and ``ReplicationDestination``
on the destination.

.. code:: bash

    kubectl create ns dest
    kubectl create -f example/source-restic/ -n dest

To start restic restore, deploy ReplicationDestination in ``dest`` namespace

.. code:: yaml

    ---
    apiVersion: scribe.backube/v1alpha1
    kind: ReplicationDestination
    metadata:
    name: database-destination
    namespace: dest
    spec:
    sourcePVC: mysql-pv-claim
    trigger:
        schedule: "*/5 * * * *"
    restic:
        repository: restic-config
        copyMethod: Snapshot
        accessModes: [ReadWriteOnce]
        capacity: 2Gi


.. code:: bash   

    kubectl create -f examples/scribe_v1alpha1_replicationdestination_restic.yaml -n dest

Once the restore is complete, scribe will store the PiT copy of the source restic snapshots as a
VolumeSnapshot in ``.status.latestImage.name`` field of ``ReplicationDestination`` object. 

.. code:: bash

    Status:
        Conditions:
            Last Transition Time:  2021-04-06T15:17:05Z
            Message:               Reconcile complete
            Reason:                ReconcileComplete
            Status:                True
            Type:                  Reconciled
        Last Sync Duration:      16.223168383s
        Last Sync Time:          2021-04-06T15:17:09Z
        Latest Image:
            API Group:     snapshot.storage.k8s.io
            Kind:          VolumeSnapshot
            Name:          scribe-dest-database-destination-20210406151702
        Next Sync Time:  2021-04-06T15:20:00Z

To verify restore, deploy the MySQL database to the ``dest`` namespace which will use the data that has
been restored from sourcePVC backup.

First we need to identify the latest snapshot from the ``ReplicationDestination`` object. Record the value
of the latest snapshot as it will be used to create a pvc.

.. code:: bash

    kubectl get replicationdestination database-destination -n dest --template={{.status.latestImage.name}}
    $ scribe-dest-database-destination-20210405101000

Use the ``volumesnapshot`` from above as the data source for the destination pvc.
Then create the Deployment, Service, PVC, and Secret for the destination MySQL database.

.. code:: bash

    sed -i 's/snapshotToReplace/scribe-dest-database-destination-20210405101000/g' examples/destination-database/mysql-pvc.yaml
    kubectl create -n dest -f examples/destination-database/


Validate that the mysql pod is running within the environment.

.. code:: bash

   $ kubectl get pods -n dest
   NAME                                           READY   STATUS    RESTARTS   AGE
   mysql-8b9c5c8d8-v6tg6                          1/1     Running   0          38m

Connect to the mysql pod and list the databases to verify the synced database
exists.

.. code:: bash

   $ kubectl exec --stdin --tty -n dest `kubectl get pods -n dest | grep mysql | awk '{print $1}'` -- /bin/bash
   $ mysql -u root -p$MYSQL_ROOT_PASSWORD
   > show databases;
   +--------------------+
   | Database           |
   +--------------------+
   | information_schema |
   | mysql              |
   | performance_schema |
   | synced             |
   | sys                |
   +--------------------+
   5 rows in set (0.00 sec)

   > exit
   $ exit