.. These are available flags for scribe plugin that may be set by including as "./config.yaml" or on the command line.

Scribe Plugin Options with Defaults
====================================

These may be passed as :code:`key: value` pairs in a file, on the command-line, or a combination of both.
Command-line values override the config file values.

Scribe Plugin will look for this file in :code:`./config.yaml`, :code:`~/.scribeconfig/config.yaml`, or from
the command-line passed :code:`--config` value that is a path to a local file.

.. code:: yaml

    dest-address: <remote address to connect to for replication>
    dest-name: <dest-namespace>-destination
    dest-namespace: <current namespace>
    dest-kube-context: <kubectl config current-context>
    dest-kube-clustername: <current-context clustername>
    dest-access-mode: one of ReadWriteOnce|ReadOnlyMany|ReadWriteMany
    dest-capacity: <source-capacity>
    dest-cron-spec: <continuous>
    dest-pvc: <if not provided, scribe will provision one>
    dest-service-type: 'ClusterIP'
    dest-ssh-user: 'root'
    dest-storage-class-name: <default sc>
    dest-volume-snapshot-class-name: <default vsc>
    dest-copy-method: one of None|Clone|Snapshot
    dest-port: 22
    dest-provider: <external replication provider, pass as 'domain.com/provider'>
    dest-provider-params: <key=value configuration parameters, if external provider; pass as 'key=value,key1=value1'>
    dest-path: /
    source-name: <source-namespace>-source
    source-namespace: <current namespace>
    source-kube-context: <current-context>
    source-kube-clustername: <current-context clustername>
    source-access-mode: one of ReadWriteOnce|ReadOnlyMany|ReadWriteMany
    source-capacity: "2Gi"
    source-cron-spec: "*/3 * * * *"
    source-pvc: <name of existing PVC to replicate>
    source-service-type: 'ClusterIP'
    source-ssh-user: 'root'
    source-storage-class-name: <default sc>
    source-volume-snapshot-class-name: <default vsc>
    source-copy-method: one of None|Clone|Snapshot
    source-port: 22
    source-provider: <external replication provider, pass as 'domain.com/provider'>
    source-provider-params: <key=value configuration parameters, if external provider; pass as 'key=value,key1=value1'>
    ssh-keys-secret: <scribe-rsync->dest-src-<name-of-replication-destination>
