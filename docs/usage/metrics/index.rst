====================
Metrics & monitoring
====================

In order to support monitoring of replication relationships, VolSync exports a
number of metrics that can be scraped with Prometheus. These metrics permit
monitoring whether volumes are "in sync" and how long the synchronization
iterations take.

Available metrics
=================

The following metrics are provided by VolSync for each replication object (source
or destination):

volsync_missed_intervals_total
   This is a count of the number of times that a replication iteration failed to
   complete before the next scheduled start. This metric is only valid for
   objects that have a schedule (``.spec.trigger.schedule``) specified. For
   example, when using the rsync mover with a schedule on the source but not on
   the destination, only the metric for the source side is meaningful.
volsync_sync_duration_seconds
   This is a summary of the time required for each sync iteration. By monitoring
   this value it is possible to determine how much "slack" exists in the
   synchronization schedule (i.e., how much less is the sync duration than the
   schedule frequency).
volsync_volume_out_of_sync
   This is a gauge that has the value of either "0" or "1", with a "1"
   indicating that the volumes are not currently synchronized. This may be due
   to an error that is preventing synchronization or because the most recent
   synchronization iteration failed to complete prior to when the next should
   have started. This metric also requires a schedule to be defined.

Each of the above metrics include the following labels to assist with monitoring
and alerting:

obj_name
   This is the name of the VolSync CustomResource
obj_namespace
   This is the Kubernetes Namespace that contains the CustomResource
role
   This contains the value of either "source" or "destination" depending on
   whether the CR is a ReplicationSource or a ReplicationDestination.
method
   This indicates the synchronization method being used. Currently, "rsync",
   "rclone", or "kopia".

As an example, the below raw data comes from a single rsync-based relationship
that is replicating data using the ReplicationSource ``dsrc`` in the ``srcns``
namespace to the ReplicationDestination ``dest`` in the ``dstns`` namespace.

.. code-block:: none
   :caption: Example raw metrics data

    $ curl -s http://127.0.0.1:8080/metrics | grep volsync

    # HELP volsync_missed_intervals_total The number of times a synchronization failed to complete before the next scheduled start
    # TYPE volsync_missed_intervals_total counter
    volsync_missed_intervals_total{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 0
    volsync_missed_intervals_total{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 0
    # HELP volsync_sync_duration_seconds Duration of the synchronization interval in seconds
    # TYPE volsync_sync_duration_seconds summary
    volsync_sync_duration_seconds{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination",quantile="0.5"} 179.725047058
    volsync_sync_duration_seconds{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination",quantile="0.9"} 544.86628289
    volsync_sync_duration_seconds{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination",quantile="0.99"} 544.86628289
    volsync_sync_duration_seconds_sum{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 828.711667153
    volsync_sync_duration_seconds_count{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 3
    volsync_sync_duration_seconds{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source",quantile="0.5"} 11.547060835
    volsync_sync_duration_seconds{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source",quantile="0.9"} 12.013468222
    volsync_sync_duration_seconds{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source",quantile="0.99"} 12.013468222
    volsync_sync_duration_seconds_sum{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 33.317039014
    volsync_sync_duration_seconds_count{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 3
    # HELP volsync_volume_out_of_sync Set to 1 if the volume is not properly synchronized
    # TYPE volsync_volume_out_of_sync gauge
    volsync_volume_out_of_sync{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 0
    volsync_volume_out_of_sync{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 0


Obtaining metrics
=================

The above metrics can be collected by Prometheus. If the cluster does not
already have a running instance set to scrape metrics, one will need to be
started.

Configuring Prometheus
----------------------

.. tabs::

   .. tab:: Kubernetes

      The following steps start a simple Prometheus instance to scrape metrics
      from VolSync. Some platforms may already have a running Prometheus operator
      or instance, making these steps unnecessary.

      Start the Prometheus operator:

      .. code-block:: none

        $ kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.46.0/bundle.yaml

      Start Prometheus by applying the following block of yaml via:

      .. code-block:: none

        $ kubectl create ns volsync-system
        $ kubectl -n volsync-system apply -f -

      .. code-block:: yaml

          apiVersion: v1
          kind: ServiceAccount
          metadata:
            name: prometheus
          ---
          apiVersion: rbac.authorization.k8s.io/v1
          kind: ClusterRole
          metadata:
            name: prometheus
          rules:
            - apiGroups: [""]
              resources:
                - nodes
                - services
                - endpoints
                - pods
              verbs: ["get", "list", "watch"]
            - apiGroups: [""]
              resources:
                - configmaps
              verbs: ["get"]
            - nonResourceURLs: ["/metrics"]
              verbs: ["get"]
          ---
          apiVersion: rbac.authorization.k8s.io/v1
          kind: ClusterRoleBinding
          metadata:
            name: prometheus
          roleRef:
            apiGroup: rbac.authorization.k8s.io
            kind: ClusterRole
            name: prometheus
          subjects:
            - kind: ServiceAccount
              name: prometheus
              namespace: volsync-system  # Change if necessary!
          ---
          apiVersion: monitoring.coreos.com/v1
          kind: Prometheus
          metadata:
            name: prometheus
          spec:
            serviceAccountName: prometheus
            serviceMonitorSelector:
              matchLabels:
                control-plane: volsync-controller
            resources:
              requests:
                memory: 400Mi

   .. tab:: OpenShift

      If necessary, `create a monitoring configuration
      <https://docs.openshift.com/container-platform/4.7/monitoring/configuring-the-monitoring-stack.html#creating-user-defined-workload-monitoring-configmap_configuring-the-monitoring-stack>`_
      in the ``openshift-user-workload-monitoring`` namespace and `enable user
      workload monitoring
      <https://docs.openshift.com/container-platform/4.7/monitoring/enabling-monitoring-for-user-defined-projects.html#enabling-monitoring-for-user-defined-projects_enabling-monitoring-for-user-defined-projects>`_:

      .. code-block:: yaml
        :caption: Example user workload monitoring configuration

        ---
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: user-workload-monitoring-config
          namespace: openshift-user-workload-monitoring
        data:
          config.yaml: |
            # Allocate persistent storage for user Prometheus
            prometheus:
              volumeClaimTemplate:
                spec:
                  resources:
                    requests:
                      storage: 40Gi
            # Allocate persistent storage for user Thanos Ruler
            thanosRuler:
              volumeClaimTemplate:
                spec:
                  resources:
                    requests:
                      storage: 40Gi

      .. code-block:: yaml
        :caption: Enabling user workload monitoring

        ---
        apiVersion: v1
        kind: ConfigMap
        metadata:
          name: cluster-monitoring-config
          namespace: openshift-monitoring
        data:
          config.yaml: |
            # Allocate persistent storage for alertmanager
            alertmanagerMain:
              volumeClaimTemplate:
                spec:
                  resources:
                    requests:
                      storage: 40Gi
            # Enable user workload monitoring stack
            enableUserWorkload: true
            # Allocate persistent storage for cluster prometheus
            prometheusK8s:
              volumeClaimTemplate:
                spec:
                  resources:
                    requests:
                      storage: 40Gi


Monitoring VolSync
------------------

The metrics port for VolSync is (by default) `protected via kube-auth-proxy
<https://book.kubebuilder.io/reference/metrics.html>`_. In order to grant
Prometheus the ability to scrape the metrics, its ServiceAccount must be granted
access to the ``volsync-metrics-reader`` ClusterRole. This can be accomplished by
(substitute in the namespace & SA name of the Prometheus server):

.. code-block:: none

   $ kubectl create clusterrolebinding metrics --clusterrole=volsync-metrics-reader --serviceaccount=<namespace>:<service-account-name>

Optionally, authentication of the metrics port can be disabled by setting the
Helm chart value ``metrics.disableAuth`` to ``false`` when deploying VolSync.

A ServiceMonitor needs to be defined in order to scrape metrics. If the
ServiceMonitor CRD was defined in the cluster when the VolSync chart was
deployed, this has already been added. If not, apply the following into the
namespace where VolSync is deployed. Note that the ``control-plane`` labels may
need to be adjusted.

.. code-block:: yaml
  :caption: VolSync ServiceMonitor

  ---
  apiVersion: monitoring.coreos.com/v1
  kind: ServiceMonitor
  metadata:
    name: volsync-monitor
    namespace: volsync-system
    labels:
      control-plane: volsync-controller
  spec:
    endpoints:
      - interval: 30s
        path: /metrics
        port: https
        scheme: https
        tlsConfig:
          # Using self-signed cert for connection
          insecureSkipVerify: true
    selector:
      matchLabels:
        control-plane: volsync-controller


Kopia-specific metrics
======================

In addition to the standard VolSync metrics above, the Kopia mover provides
comprehensive metrics for monitoring backup and restore operations, repository
health, and performance characteristics. These metrics use the
``volsync_kopia`` namespace to distinguish them from general VolSync metrics.

Common Kopia labels
-------------------

All Kopia-specific metrics include these additional labels beyond the standard
VolSync labels:

repository
   Name of the Kopia repository
operation
   Type of operation being performed (backup, restore, maintenance)

Kopia metrics categories
------------------------

Backup/Restore Performance Metrics
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

volsync_kopia_operation_duration_seconds
   **Type:** Summary
   
   Duration of Kopia operations (backup, restore, maintenance) in seconds.
   Use for monitoring operation performance trends, setting SLA alerts for
   backup/restore times, and identifying performance degradation.

volsync_kopia_data_processed_bytes
   **Type:** Summary
   
   Amount of data processed during Kopia operations in bytes. Use for tracking
   data growth over time, capacity planning for storage backends, and
   correlating data size with operation duration.

volsync_kopia_data_transfer_rate_bytes_per_second
   **Type:** Summary
   
   Data transfer rate during Kopia operations in bytes per second. Use for
   monitoring network performance, detecting bandwidth constraints, and
   comparing performance across different repositories.

volsync_kopia_compression_ratio
   **Type:** Summary
   
   Compression ratio achieved during backup operations
   (compressed_size/original_size). Use for monitoring compression
   effectiveness, optimizing compression settings, and estimating storage
   savings.

volsync_kopia_operation_success_total
   **Type:** Counter
   
   Total number of successful Kopia operations. Use for calculating success
   rates, tracking operation volume, and generating availability reports.

volsync_kopia_operation_failure_total
   **Type:** Counter
   
   Total number of failed Kopia operations with additional ``failure_reason``
   label indicating the cause:
   
   * ``prerequisites_failed``: Repository connectivity or configuration issues
   * ``job_creation_failed``: Kubernetes job creation failure  
   * ``job_execution_failed``: Job runtime failure

Repository Health Metrics
~~~~~~~~~~~~~~~~~~~~~~~~~~

volsync_kopia_repository_connectivity
   **Type:** Gauge
   
   Repository connectivity status (1 if connected, 0 if not). Use for alerting
   on repository connectivity issues, monitoring repository availability, and
   tracking uptime statistics.

volsync_kopia_maintenance_operations_total
   **Type:** Counter
   
   Total number of repository maintenance operations performed with
   ``maintenance_type`` label (scheduled, manual). Use for tracking maintenance
   operation frequency, verifying maintenance scheduling, and monitoring
   repository health activities.

volsync_kopia_maintenance_duration_seconds
   **Type:** Summary
   
   Duration of repository maintenance operations in seconds. Use for monitoring
   maintenance performance, planning maintenance windows, and detecting
   maintenance issues.

volsync_kopia_repository_size_bytes
   **Type:** Gauge
   
   Total size of the Kopia repository in bytes. Use for monitoring repository
   growth, capacity planning, and cost optimization for cloud storage.

volsync_kopia_repository_objects_total
   **Type:** Gauge
   
   Total number of objects in the Kopia repository. Use for tracking repository
   complexity, monitoring deduplication effectiveness, and performance
   correlation analysis.

Snapshot Management Metrics
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

volsync_kopia_snapshot_count
   **Type:** Gauge
   
   Current number of snapshots in the repository. Use for monitoring snapshot
   accumulation, verifying retention policy effectiveness, and capacity
   planning.

volsync_kopia_snapshot_creation_success_total
   **Type:** Counter
   
   Total number of successful snapshot creations. Use for tracking backup
   success rates, generating backup reports, and monitoring backup reliability.

volsync_kopia_snapshot_creation_failure_total
   **Type:** Counter
   
   Total number of failed snapshot creations with ``failure_reason`` label.
   Use for alerting on backup failures, troubleshooting backup issues, and
   tracking backup reliability trends.

volsync_kopia_snapshot_size_bytes
   **Type:** Summary
   
   Size of individual snapshots in bytes. Use for monitoring snapshot size
   distribution, identifying data growth patterns, and optimizing backup
   strategies.

volsync_kopia_deduplication_ratio
   **Type:** Summary
   
   Deduplication efficiency ratio (deduplicated_size/original_size). Use for
   monitoring deduplication effectiveness, optimizing storage efficiency, and
   calculating storage savings.

volsync_kopia_retention_compliance
   **Type:** Gauge
   
   Retention policy compliance status (1 if compliant, 0 if not) with
   ``retention_type`` label (hourly, daily, weekly, monthly, yearly). Use for
   monitoring retention policy compliance, alerting on retention violations,
   and auditing backup retention.

Cache and Performance Metrics
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

volsync_kopia_cache_hit_rate
   **Type:** Gauge
   
   Cache hit rate as a percentage (0-100). Use for monitoring cache
   effectiveness, optimizing cache configuration, and performance
   troubleshooting.

volsync_kopia_cache_size_bytes
   **Type:** Gauge
   
   Current size of the Kopia cache in bytes. Use for monitoring cache
   utilization, optimizing cache capacity allocation, and tracking cache
   growth.

volsync_kopia_parallel_operations_active
   **Type:** Gauge
   
   Number of currently active parallel operations. Use for monitoring
   parallelism utilization, detecting resource contention, and optimizing
   parallelism settings.

volsync_kopia_job_retries_total
   **Type:** Counter
   
   Total number of job retries due to failures with ``retry_reason`` label
   (``job_pod_failure``). Use for monitoring job reliability, identifying
   transient vs persistent issues, and optimizing retry strategies.

volsync_kopia_queue_depth
   **Type:** Gauge
   
   Current depth of the operation queue. Use for monitoring operation queuing,
   detecting processing bottlenecks, and optimizing queue processing.

Policy and Configuration Metrics
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

volsync_kopia_policy_compliance
   **Type:** Gauge
   
   Policy compliance status (1 if compliant, 0 if not) with ``policy_type``
   label (global, retention, compression). Use for monitoring policy
   compliance, alerting on policy violations, and auditing configuration
   compliance.

volsync_kopia_configuration_errors_total
   **Type:** Counter
   
   Total number of configuration errors encountered with ``error_type`` label:
   
   * ``repository_validation_failed``: Repository secret validation failure
   * ``custom_ca_validation_failed``: Custom CA configuration failure
   * ``policy_config_validation_failed``: Policy configuration failure

volsync_kopia_custom_actions_executed_total
   **Type:** Counter
   
   Total number of custom actions executed with ``action_type`` label
   (before_snapshot, after_snapshot). Use for monitoring custom action usage,
   tracking action execution frequency, and verifying action configuration.

volsync_kopia_custom_actions_duration_seconds
   **Type:** Summary
   
   Duration of custom action execution in seconds. Use for monitoring action
   performance, optimizing action scripts, and detecting action timeouts.

Kopia alerting recommendations
------------------------------

Critical Alerts
~~~~~~~~~~~~~~~~

* ``volsync_kopia_repository_connectivity == 0`` - Repository unreachable
* ``rate(volsync_kopia_operation_failure_total[5m]) > 0.1`` - High failure rate
* ``volsync_kopia_retention_compliance == 0`` - Retention policy violation

Warning Alerts
~~~~~~~~~~~~~~~

* ``volsync_kopia_operation_duration_seconds{quantile="0.9"} > 3600`` - Slow operations (>1 hour)
* ``rate(volsync_kopia_job_retries_total[10m]) > 0.05`` - Increased retry rate
* ``volsync_kopia_cache_hit_rate < 50`` - Poor cache performance

Informational Alerts
~~~~~~~~~~~~~~~~~~~~~

* ``increase(volsync_kopia_maintenance_operations_total[24h]) == 0`` - No maintenance in 24h
* ``volsync_kopia_repository_size_bytes > 1e12`` - Repository size > 1TB
