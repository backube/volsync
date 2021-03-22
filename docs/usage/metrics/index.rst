====================
Metrics & monitoring
====================

In order to support monitoring of replication relationships, Scribe exports a
number of metrics that can be scraped with Prometheus. These metrics permit
monitoring whether volumes are "in sync" and how long the synchronization
iterations take.

Available metrics
=================

The following metrics are provided by Scribe for each replication object (source
or destination):

scribe_missed_intervals_total
   This is a count of the number of times that a replication iteration failed to
   complete before the next scheduled start. This metric is only valid for
   objects that have a schedule (``.spec.trigger.schedule``) specified. For
   example, when using the rsync mover with a schedule on the source but not on
   the destination, only the metric for the source side is meaningful.
scribe_sync_duration_seconds
   This is a summary of the time required for each sync iteration. By monitoring
   this value it is possible to determine how much "slack" exists in the
   synchronization schedule (i.e., how much less is the sync duration than the
   schedule frequency).
scribe_volume_out_of_sync
   This is a gauge that has the value of either "0" or "1", with a "1"
   indicating that the volumes are not currently synchronized. This may be due
   to an error that is preventing synchronization or because the most recent
   synchronization iteration failed to complete prior to when the next should
   have started. This metric also requires a schedule to be defined.

Each of the above metrics include the following labels to assist with monitoring
and alerting:

obj_name
   This is the name of the Scribe CustomResource
obj_namespace
   This is the Kubernetes Namespace that contains the CustomResource
role
   This contains the value of either "source" or "destination" depending on
   whether the CR is a ReplicationSource or a ReplicationDestination.
method
   This indicates the synchronization method being used. Currently, "rsync" or
   "rclone".

As an example, the below raw data comes from a single rsync-based relationship
that is replicating data using the ReplicationSource ``dsrc`` in the ``srcns``
namespace to the ReplicationDestination ``dest`` in the ``dstns`` namespace.

.. code-block:: none
   :caption: Example raw metrics data

    $ curl -s http://127.0.0.1:8080/metrics | grep scribe

    # HELP scribe_missed_intervals_total The number of times a synchronization failed to complete before the next scheduled start
    # TYPE scribe_missed_intervals_total counter
    scribe_missed_intervals_total{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 0
    scribe_missed_intervals_total{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 0
    # HELP scribe_sync_duration_seconds Duration of the synchronization interval in seconds
    # TYPE scribe_sync_duration_seconds summary
    scribe_sync_duration_seconds{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination",quantile="0.5"} 179.725047058
    scribe_sync_duration_seconds{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination",quantile="0.9"} 544.86628289
    scribe_sync_duration_seconds{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination",quantile="0.99"} 544.86628289
    scribe_sync_duration_seconds_sum{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 828.711667153
    scribe_sync_duration_seconds_count{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 3
    scribe_sync_duration_seconds{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source",quantile="0.5"} 11.547060835
    scribe_sync_duration_seconds{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source",quantile="0.9"} 12.013468222
    scribe_sync_duration_seconds{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source",quantile="0.99"} 12.013468222
    scribe_sync_duration_seconds_sum{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 33.317039014
    scribe_sync_duration_seconds_count{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 3
    # HELP scribe_volume_out_of_sync Set to 1 if the volume is not properly synchronized
    # TYPE scribe_volume_out_of_sync gauge
    scribe_volume_out_of_sync{method="rsync",obj_name="dest",obj_namespace="dstns",role="destination"} 0
    scribe_volume_out_of_sync{method="rsync",obj_name="dsrc",obj_namespace="srcns",role="source"} 0


Obtaining metrics
=================

The above metrics can be collected by Prometheus. If the cluster does not
already have a running instance set to scrape metrics, one will need to be
started.

Configuring Prometheus
----------------------

.. content-tabs::

   .. tab-container:: kube
      :title: Kubernetes

      The following steps start a simple Prometheus instance to scrape metrics
      from Scribe. Some platforms may already have a running Prometheus operator
      or instance, making these steps unnecessary.

      Start the Prometheus operator:

      .. code-block:: none

        $ kubectl apply -f https://raw.githubusercontent.com/prometheus-operator/prometheus-operator/v0.46.0/bundle.yaml

      Start Prometheus by applying the following block of yaml via:

      .. code-block:: none

        $ kubectl create ns scribe-system
        $ kubectl -n scribe-system apply -f -

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
              namespace: scribe-system  # Change if necessary!
          ---
          apiVersion: monitoring.coreos.com/v1
          kind: Prometheus
          metadata:
            name: prometheus
          spec:
            serviceAccountName: prometheus
            serviceMonitorSelector:
              matchLabels:
                control-plane: scribe-controller
            resources:
              requests:
                memory: 400Mi

   .. tab-container:: ocp
      :title: OpenShift

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


Monitoring Scribe
-----------------

The metrics port for Scribe is (by default) `protected via kube-auth-proxy
<https://book.kubebuilder.io/reference/metrics.html>`_. In order to grant
Prometheus the ability to scrape the metrics, its ServiceAccount must be granted
access to the ``scribe-metrics-reader`` ClusterRole. This can be accomplished by
(substitute in the namespace & SA name of the Prometheus server):

.. code-block:: none

   $ kubectl create clusterrolebinding metrics --clusterrole=scribe-metrics-reader --serviceaccount=<namespace>:<service-account-name>

Optionally, authentication of the metrics port can be disabled by setting the
Helm chart value ``metrics.disableAuth`` to ``false`` when deploying Scribe.

A ServiceMonitor needs to be defined in order to scrape metrics. If the
ServiceMonitor CRD was defined in the cluster when the Scribe chart was
deployed, this has already been added. If not, apply the following into the
namespace where Scribe is deployed. Note that the ``control-plane`` labels may
need to be adjusted.

.. code-block:: yaml
  :caption: Scribe ServiceMonitor

  ---
  apiVersion: monitoring.coreos.com/v1
  kind: ServiceMonitor
  metadata:
    name: scribe-monitor
    namespace: scribe-system
    labels:
      control-plane: scribe-controller
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
        control-plane: scribe-controller
