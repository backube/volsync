==============================
KopiaMaintenance CRD Reference
==============================

.. sidebar:: Contents

   .. contents:: KopiaMaintenance
      :local:

Overview
========

The KopiaMaintenance Custom Resource Definition (CRD) provides streamlined management of Kopia repository maintenance operations in VolSync. This namespace-scoped resource offers a simple, direct approach to configuring maintenance schedules for your Kopia repositories.

What is KopiaMaintenance?
-------------------------

KopiaMaintenance is a Kubernetes custom resource that manages automated maintenance operations for Kopia repositories. It creates and manages CronJobs that perform essential repository maintenance tasks including:

- Garbage collection of unused data blocks
- Repository compaction and optimization
- Index maintenance for improved performance
- Verification of repository integrity
- Automatic maintenance ownership management

Key Features
------------

- **Namespace-scoped**: Each KopiaMaintenance resource manages repositories within its namespace
- **Direct repository configuration**: Explicit 1:1 mapping between maintenance resources and repositories
- **Simple API**: Focused design without complex selectors or priority systems
- **Resource management**: Configure CPU and memory limits for maintenance operations
- **Flexible scheduling**: Support for standard cron expressions and aliases

When to Use KopiaMaintenance
----------------------------

**Use KopiaMaintenance when you need:**

- Automated maintenance for Kopia repositories
- Namespace-isolated maintenance management
- Clear, explicit maintenance configuration
- Control over maintenance resource consumption
- Simple deployment without cross-namespace complexity

**Continue using embedded maintenanceCronJob in ReplicationSource when:**

- You have existing configurations that work well
- You prefer configuration alongside your backup definitions
- You need minimal setup for single repositories

API Specification
=================

Basic Structure
---------------

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: <maintenance-name>
     namespace: <target-namespace>
   spec:
     repository:
       repository: <repository-secret-name>
       customCA:  # Optional
         configMapName: <ca-configmap-name>
         key: <ca-cert-key>
     trigger:  # New trigger support
       schedule: "0 2 * * *"  # Scheduled trigger
       # OR
       manual: "trigger-1"    # Manual trigger
     enabled: true
     suspend: false
     successfulJobsHistoryLimit: 3
     failedJobsHistoryLimit: 1
     resources:
       requests:
         memory: "256Mi"
         cpu: "100m"
       limits:
         memory: "1Gi"
         cpu: "500m"
     # Cache configuration (new)
     cacheCapacity: 10Gi
     cacheStorageClassName: fast-ssd
     cacheAccessModes:
       - ReadWriteOnce
     # OR use existing PVC
     cachePVC: existing-cache-pvc

Field Reference
---------------

Required Fields
^^^^^^^^^^^^^^^

**repository** (*KopiaRepositorySpec*, required)
   Defines the repository configuration for maintenance.
   The repository secret must exist in the same namespace as the KopiaMaintenance resource.

**repository.repository** (*string*, required)
   Name of the secret containing repository configuration.
   Secret must contain Kopia repository connection details (URL, credentials, etc.)

Optional Fields
^^^^^^^^^^^^^^^

**repository.customCA** (*ReplicationSourceKopiaCA*, optional)
   Custom CA configuration for repository access.

   - **configMapName**: Name of ConfigMap containing CA certificate
   - **key**: Key within ConfigMap containing the certificate (default: "ca.crt")
   - **secretName**: Alternative to ConfigMap, name of Secret containing CA certificate

**trigger** (*KopiaMaintenanceTriggerSpec*, optional)
   Defines when maintenance will be performed. Supports scheduled and manual triggers.

   - **schedule**: Cron schedule for maintenance execution (mutually exclusive with manual)
   - **manual**: String value for manual trigger (mutually exclusive with schedule)
   - Default: If no trigger specified, defaults to ``schedule: "0 2 * * *"``

**schedule** (*string*, optional, deprecated)
   Cron schedule for maintenance execution.

   - **DEPRECATED**: Use ``trigger.schedule`` instead. This field will be removed in a future version.
   - Default: ``"0 2 * * *"`` (daily at 2 AM)
   - Supports standard cron expressions and aliases (``@daily``, ``@weekly``, ``@monthly``)

**enabled** (*boolean*, optional)
   Determines if maintenance should be performed.

   - Default: ``true``
   - When ``false``, no maintenance jobs will be created

**suspend** (*boolean*, optional)
   Temporarily stop maintenance without deleting configuration.

   - Default: ``false``
   - When ``true``, prevents new Jobs from being created while allowing existing Jobs to complete

**successfulJobsHistoryLimit** (*integer*, optional)
   Number of successful maintenance Jobs to retain.

   - Default: ``3``
   - Minimum: ``0``

**failedJobsHistoryLimit** (*integer*, optional)
   Number of failed maintenance Jobs to retain.

   - Default: ``1``
   - Minimum: ``0``

**resources** (*ResourceRequirements*, optional)
   Compute resources for maintenance containers.

   - Default requests: 256Mi memory
   - Default limits: 1Gi memory
   - Configure based on repository size and performance requirements

**activeDeadlineSeconds** (*integer*, optional)
   Maximum duration in seconds for maintenance jobs before termination.

   - Default: ``10800`` (3 hours)
   - Minimum: ``600`` (10 minutes)
   - Increase for very large repositories that require longer maintenance windows
   - Jobs exceeding this limit are terminated by Kubernetes

**serviceAccountName** (*string*, optional)
   Custom ServiceAccount for maintenance jobs.
   If not specified, uses default maintenance ServiceAccount.

**podSecurityContext** (*PodSecurityContext*, optional)
   Pod-level security context for maintenance jobs.
   Allows configuring security settings such as runAsUser, fsGroup, and other standard Kubernetes pod security options.
   Container automatically inherits these settings.
   Default: ``runAsUser: 1000, fsGroup: 1000, runAsNonRoot: true``

**containerSecurityContext** (*SecurityContext*, optional)
   Container-level security context for maintenance jobs.
   For advanced use cases where you need fine-grained control over container security.

   **IMPORTANT:** For setting the user ID, use ``podSecurityContext.runAsUser`` instead.
   The container automatically inherits runAsUser from the pod-level context.

   Use this field only for advanced security controls like capabilities, privileged mode,
   seLinux, or seccomp profiles.

   Default: Security hardening settings are applied automatically (readOnlyRootFilesystem,
   allowPrivilegeEscalation: false, capabilities dropped)

**moverPodLabels** (*map[string]string*, optional)
   Additional labels for maintenance pods.
   Applied alongside VolSync-managed labels.

**affinity** (*Affinity*, optional)
   Pod affinity rules for maintenance jobs.
   Supports nodeAffinity, podAffinity, and podAntiAffinity.

**nodeSelector** (*map[string]string*, optional)
   Node selector constraints for maintenance pods.
   Use to schedule maintenance jobs on specific nodes.

   Example: ``node-type: high-memory``

**tolerations** (*[]Toleration*, optional)
   Tolerations for maintenance pods.
   Allows scheduling on nodes with matching taints.

   Supports standard Kubernetes toleration fields:
   ``key``, ``operator``, ``value``, ``effect``, ``tolerationSeconds``

**cacheCapacity** (*Quantity*, optional)
   Size of the Kopia metadata cache volume.
   If specified without cachePVC, a new PVC will be created.

**cacheStorageClassName** (*string*, optional)
   StorageClass for the Kopia metadata cache volume.
   Only used when creating a new cache PVC.

**cacheAccessModes** (*[]PersistentVolumeAccessMode*, optional)
   Access modes for the Kopia metadata cache volume.
   Default: ``[ReadWriteOnce]``

**cachePVC** (*string*, optional)
   Name of an existing PVC to use for Kopia cache.
   If specified, other cache configuration fields are ignored.

Status Fields
^^^^^^^^^^^^^

The KopiaMaintenance controller updates these status fields:

**activeCronJob** (*string*)
   Name of the currently active CronJob managing maintenance.
   Empty if no CronJob is active.

**lastReconcileTime** (*Time*)
   Timestamp of the last successful reconciliation.

**lastMaintenanceTime** (*Time*)
   Timestamp of the last successful maintenance operation.

**nextScheduledMaintenance** (*Time*)
   Next scheduled maintenance execution time.

**maintenanceFailures** (*integer*)
   Count of consecutive maintenance failures.

**lastManualSync** (*string*)
   Set to the last spec.trigger.manual value when manual maintenance completes.
   Used to track completion of manual triggers.

**conditions** (*[]Condition*)
   Current state observations of the maintenance configuration.
   Common conditions: Ready, Reconciling, Error.

Configuration Examples
======================

Trigger Configuration
--------------------

Scheduled Trigger (Recommended)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: scheduled-maintenance
     namespace: my-app
   spec:
     repository:
       repository: kopia-repository-secret
     trigger:
       schedule: "0 3 * * *"  # 3 AM daily
     enabled: true

Manual Trigger for On-Demand Maintenance
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: manual-maintenance
     namespace: my-app
   spec:
     repository:
       repository: kopia-repository-secret
     trigger:
       manual: "run-maintenance-2024-01-15"  # Change this value to trigger
     enabled: true

   # To trigger maintenance:
   # 1. Update spec.trigger.manual to a new value
   # 2. Wait for status.lastManualSync to match the new value
   # 3. Maintenance has completed when values match

Basic Daily Maintenance (Legacy)
--------------------------------

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: daily-maintenance
     namespace: my-app
   spec:
     repository:
       repository: kopia-repository-secret
     schedule: "0 3 * * *"  # 3 AM daily (deprecated field)
     enabled: true
     successfulJobsHistoryLimit: 3  # Keep last 3 successful jobs
     failedJobsHistoryLimit: 1       # Keep last failed job

Weekly Maintenance with Resource Limits
----------------------------------------

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: weekly-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     schedule: "0 2 * * 0"  # 2 AM on Sundays
     resources:
       requests:
         memory: "512Mi"
         cpu: "200m"
       limits:
         memory: "2Gi"
         cpu: "1"
     successfulJobsHistoryLimit: 5
     failedJobsHistoryLimit: 2

Maintenance with Custom CA
--------------------------

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: secure-maintenance
     namespace: secure-backups
   spec:
     repository:
       repository: private-s3-config
       customCA:
         configMapName: company-ca-bundle
         key: ca-bundle.crt
     schedule: "0 1 * * 1,4"  # 1 AM on Mondays and Thursdays
     moverPodLabels:
       environment: production
       team: platform

High-Performance Maintenance with Cache
----------------------------------------

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: large-repo-maintenance
     namespace: data-warehouse
   spec:
     repository:
       repository: warehouse-backup-config
     trigger:
       schedule: "0 0 * * 6"  # Midnight on Saturdays
     resources:
       requests:
         memory: "2Gi"
         cpu: "1"
       limits:
         memory: "8Gi"
         cpu: "4"
     # Cache configuration for better performance
     cacheCapacity: 20Gi
     cacheStorageClassName: fast-ssd
     cacheAccessModes:
       - ReadWriteOnce
     affinity:
       nodeAffinity:
         requiredDuringSchedulingIgnoredDuringExecution:
           nodeSelectorTerms:
           - matchExpressions:
             - key: node-type
               operator: In
               values: ["high-memory"]

Scheduling with NodeSelector and Tolerations
--------------------------------------------

Schedule maintenance on specific nodes with taints:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: dedicated-node-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       schedule: "0 2 * * *"
     # Schedule on nodes with specific labels
     nodeSelector:
       node-role: backup-worker
       disk-type: ssd
     # Tolerate taints on dedicated backup nodes
     tolerations:
       - key: "dedicated"
         operator: "Equal"
         value: "backup"
         effect: "NoSchedule"
       - key: "backup-only"
         operator: "Exists"
         effect: "NoExecute"
         tolerationSeconds: 3600
     resources:
       requests:
         memory: "1Gi"
       limits:
         memory: "4Gi"

Long-Running Maintenance with Extended Timeout
----------------------------------------------

For very large repositories that need more than the default 3-hour timeout:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: large-repo-extended-timeout
     namespace: archive
   spec:
     repository:
       repository: archive-backup-config
     trigger:
       schedule: "0 0 * * 6"  # Weekly on Saturday midnight
     activeDeadlineSeconds: 43200  # 12 hours for very large repos
     resources:
       requests:
         memory: "4Gi"
         cpu: "2"
       limits:
         memory: "16Gi"
         cpu: "8"
     cacheCapacity: 50Gi
     cacheStorageClassName: fast-ssd

Cache Configuration Examples
----------------------------

Using Existing Cache PVC
^^^^^^^^^^^^^^^^^^^^^^^^

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: maintenance-with-existing-cache
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       schedule: "0 2 * * *"
     cachePVC: shared-kopia-cache  # Use existing PVC

Creating New Cache PVC
^^^^^^^^^^^^^^^^^^^^^^

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: maintenance-with-new-cache
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       schedule: "0 2 * * *"
     cacheCapacity: 15Gi            # Create new PVC with this size
     cacheStorageClassName: fast    # Use this storage class
     cacheAccessModes:
       - ReadWriteOnce

No Cache (EmptyDir Fallback)
^^^^^^^^^^^^^^^^^^^^^^^^^^^^

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: maintenance-no-cache
     namespace: testing
   spec:
     repository:
       repository: test-backup-config
     trigger:
       schedule: "0 4 * * *"
     # No cache configuration - will use EmptyDir

Temporarily Suspended Maintenance
----------------------------------

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: suspended-maintenance
     namespace: testing
   spec:
     repository:
       repository: test-backup-config
     trigger:
       schedule: "0 4 * * *"
     enabled: true
     suspend: true  # Temporarily suspended
     successfulJobsHistoryLimit: 10  # Keep more history during suspension

Pod Security Configuration
==========================

Overview
--------

The ``podSecurityContext`` field allows you to customize pod-level security settings for maintenance jobs. This is particularly useful when repository directories have specific ownership requirements or when you need to comply with security policies.

When to Use Pod Security Context
---------------------------------

You should configure ``podSecurityContext`` when:

- **Repository ownership differs from defaults**: Your repository directory is owned by a user other than UID 1000
- **Permission errors occur**: You see "permission denied" errors when accessing repository files
- **Security compliance**: Your organization requires specific security context settings
- **Storage system requirements**: Your storage backend requires specific user/group IDs

Common Use Case: Permission Denied Errors
------------------------------------------

**Problem**: Maintenance jobs fail with permission errors when accessing the repository.

**Error Example**:

.. code-block:: text

   ERROR error connecting to repository: unable to read format blob:
   error determining sharded path: error getting sharding parameters for storage:
   unable to complete GetBlobFromPath:/repository/.shards despite 10 retries:
   open /repository/.shards: permission denied

**Cause**: The repository directory is owned by a user (e.g., UID 2000) that differs from the default maintenance job user (UID 1000).

**Solution**: Configure ``podSecurityContext`` to match the repository ownership:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: my-maintenance
     namespace: backup-ns
   spec:
     repository:
       repository: my-repo-secret
     podSecurityContext:
       runAsUser: 2000      # Match repository directory owner
       fsGroup: 2000        # Match repository directory group
       runAsNonRoot: true   # Security best practice

Configuration Examples
----------------------

Matching Repository File Ownership
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

When your repository files are owned by a specific user:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: custom-user-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-secret
     podSecurityContext:
       runAsUser: 2000
       fsGroup: 2000
       runAsNonRoot: true
     trigger:
       schedule: "0 2 * * *"

Additional Security Settings
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

For enhanced security compliance:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: secure-maintenance
     namespace: production
   spec:
     repository:
       repository: secure-repo-secret
     podSecurityContext:
       runAsUser: 3000
       runAsGroup: 3000
       fsGroup: 3000
       runAsNonRoot: true
       seccompProfile:
         type: RuntimeDefault
       supplementalGroups:
         - 4000
     trigger:
       schedule: "0 3 * * 0"

Default Security Context
^^^^^^^^^^^^^^^^^^^^^^^^^

When ``podSecurityContext`` is not specified, the following defaults are used:

.. code-block:: yaml

   podSecurityContext:
     runAsUser: 1000
     fsGroup: 1000
     runAsNonRoot: true

This default configuration works for most scenarios where repository directories are created by VolSync with standard ownership.

Determining Required User/Group IDs
------------------------------------

To identify the correct user and group IDs for your repository:

**For filesystem-based repositories (moverVolumes)**:

.. code-block:: bash

   # Create a temporary pod to check ownership
   kubectl run -it --rm debug --image=busybox --restart=Never \
     --overrides='
     {
       "spec": {
         "containers": [{
           "name": "debug",
           "image": "busybox",
           "command": ["sh"],
           "volumeMounts": [{
             "name": "repo",
             "mountPath": "/repository"
           }]
         }],
         "volumes": [{
           "name": "repo",
           "persistentVolumeClaim": {
             "claimName": "your-repository-pvc"
           }
         }]
       }
     }' \
     -- sh -c "ls -ln /repository"

   # Look for the numeric user and group IDs in the output
   # Example output: drwxr-xr-x 2 2000 2000 4096 Jan 20 10:00 repository

**For object storage repositories (S3, Azure, GCS)**:

Object storage typically doesn't require specific UIDs, but you may need to match the user that created the repository if filesystem caching is used.

Available Security Context Fields
----------------------------------

The ``podSecurityContext`` field supports all standard Kubernetes PodSecurityContext options:

.. list-table::
   :header-rows: 1
   :widths: 30 70

   * - Field
     - Description
   * - ``runAsUser``
     - UID to run the pod processes
   * - ``runAsGroup``
     - Primary GID for pod processes
   * - ``fsGroup``
     - Special supplemental group for volume ownership
   * - ``runAsNonRoot``
     - Ensures containers run as non-root (recommended: true)
   * - ``supplementalGroups``
     - Additional groups for the first process
   * - ``fsGroupChangePolicy``
     - How volume ownership is changed (OnRootMismatch, Always)
   * - ``seccompProfile``
     - Seccomp profile (e.g., RuntimeDefault)
   * - ``seLinuxOptions``
     - SELinux options for containers
   * - ``windowsOptions``
     - Windows-specific security settings

Container-Level Security
-------------------------

KopiaMaintenance supports both pod-level and container-level security context configuration.
This provides flexibility for advanced use cases while keeping simple scenarios straightforward.

Security Context Inheritance
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

**How it works:**

1. **Pod-level settings** (``podSecurityContext``) apply to all containers and control volume permissions
2. **Container-level settings** (``containerSecurityContext``) provide fine-grained container controls
3. **The container inherits ``runAsUser`` from the pod-level context** - no need to set it twice

**Default behavior** (when containerSecurityContext is not specified):

.. code-block:: yaml

   # Container security context (applied automatically)
   securityContext:
     allowPrivilegeEscalation: false
     capabilities:
       drop:
         - ALL
     privileged: false
     readOnlyRootFilesystem: true
     runAsNonRoot: true
     # runAsUser: <inherited from pod-level>

These defaults provide defense-in-depth security by:

- Preventing privilege escalation
- Dropping all Linux capabilities
- Making the root filesystem read-only
- Ensuring non-root execution

**Simple configuration** (recommended for most users):

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   spec:
     podSecurityContext:
       runAsUser: 2000      # Container inherits this
       fsGroup: 2000
       runAsNonRoot: true

**Advanced configuration** (for custom capabilities, seLinux, etc.):

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   spec:
     podSecurityContext:
       runAsUser: 2000      # Still set user here
       fsGroup: 2000
     containerSecurityContext:
       allowPrivilegeEscalation: false
       capabilities:
         drop: ["ALL"]
         add: ["NET_BIND_SERVICE"]  # Advanced: add specific capability
       readOnlyRootFilesystem: true
       runAsNonRoot: true
       # Don't set runAsUser here - it's inherited from pod level

Backward Compatibility
----------------------

Existing KopiaMaintenance resources continue to work without changes:

- If ``podSecurityContext`` is not specified, the default values are applied
- No migration is required for existing configurations
- You can add ``podSecurityContext`` to existing resources at any time

Troubleshooting Pod Security Issues
------------------------------------

**Maintenance Jobs Fail with Permission Errors**

.. code-block:: bash

   # Check the maintenance job logs
   kubectl logs -n <namespace> job/<maintenance-job-name>

   # Verify pod security context
   kubectl get pod <maintenance-pod> -o jsonpath='{.spec.securityContext}'

   # Check repository directory permissions (for filesystem repos)
   kubectl exec <maintenance-pod> -- ls -ln /repository

**Solution**: Configure ``podSecurityContext`` to match repository ownership.

**Jobs Won't Start Due to Security Policy Violations**

.. code-block:: bash

   # Check pod security admission warnings
   kubectl describe pod <maintenance-pod>

**Solution**: Adjust ``podSecurityContext`` to comply with cluster security policies (Pod Security Standards, OPA policies, etc.).

**SELinux Context Errors**

.. code-block:: yaml

   podSecurityContext:
     seLinuxOptions:
       level: "s0:c123,c456"
       role: "system_r"
       type: "container_t"
       user: "system_u"

Best Practices
==============

Trigger Selection
----------------

**Scheduled Triggers**

Use scheduled triggers for:

- Regular, predictable maintenance windows
- Production environments with consistent backup patterns
- Repositories that grow at a steady rate

Example schedules:

- ``"0 2 * * *"`` - Daily at 2 AM
- ``"0 3 * * 0"`` - Weekly on Sunday at 3 AM
- ``"0 4 1 * *"`` - Monthly on the 1st at 4 AM
- ``"@daily"`` - Once per day at midnight
- ``"@weekly"`` - Once per week on Sunday at midnight

**Manual Triggers**

Use manual triggers for:

- On-demand maintenance after large data changes
- Testing and troubleshooting
- Maintenance coordination with other operations
- CI/CD pipeline integration

To use manual triggers:

1. Set ``spec.trigger.manual`` to a unique value
2. Apply the resource
3. Monitor ``status.lastManualSync``
4. When ``lastManualSync`` matches your trigger value, maintenance is complete
5. Update ``spec.trigger.manual`` to a new value for next trigger

Job History Management
----------------------

KopiaMaintenance allows you to control how many completed Job records are retained for successful and failed maintenance operations. This helps balance between having debugging history and reducing cluster resource usage.

Configuration Fields
^^^^^^^^^^^^^^^^^^^^

**successfulJobsHistoryLimit** (*integer*, default: 3)
   Controls how many successful maintenance Job records to keep. These records are useful for:

   - Tracking maintenance execution patterns
   - Verifying maintenance is running on schedule
   - Reviewing historical performance and duration
   - Troubleshooting intermittent issues

   Set to 0 to delete successful jobs immediately after completion.

**failedJobsHistoryLimit** (*integer*, default: 1)
   Controls how many failed maintenance Job records to keep. Failed jobs are crucial for:

   - Diagnosing what went wrong during maintenance
   - Identifying patterns in failures
   - Providing logs for troubleshooting
   - Understanding error conditions

   Set to 0 to delete failed jobs immediately (not recommended).

When to Customize
^^^^^^^^^^^^^^^^^

**Increase history limits when:**

- Debugging maintenance issues and need more historical context
- Running maintenance infrequently (weekly/monthly) and want long-term history
- Tracking performance trends over time
- Working in development/testing environments

**Decrease history limits when:**

- Running maintenance very frequently (hourly) and don't need extensive history
- Cluster has limited resources and job records consume too much memory
- Using external monitoring and don't need Kubernetes job history
- Operating in resource-constrained environments

Example Configurations
^^^^^^^^^^^^^^^^^^^^^^

Minimal History (Resource Constrained):

.. code-block:: yaml

   spec:
     successfulJobsHistoryLimit: 1   # Keep only last success
     failedJobsHistoryLimit: 0       # Delete failures immediately

Extended History (Debugging):

.. code-block:: yaml

   spec:
     successfulJobsHistoryLimit: 10  # Keep 10 successful runs
     failedJobsHistoryLimit: 5       # Keep 5 failed runs for analysis

Balanced Default (Recommended):

.. code-block:: yaml

   spec:
     successfulJobsHistoryLimit: 3   # Default: last 3 successful runs
     failedJobsHistoryLimit: 1       # Default: last failed run

Cache Configuration
-------------------

Kopia uses a metadata cache to improve performance. KopiaMaintenance supports four cache scenarios:

**1. Existing PVC (Recommended for Production)**

Best when you want full control over the cache PVC:

.. code-block:: yaml

   spec:
     cachePVC: my-cache-pvc  # Must exist in same namespace

**2. Auto-Created PVC**

Best for automatic cache management:

.. code-block:: yaml

   spec:
     cacheCapacity: 10Gi
     cacheStorageClassName: fast-ssd
     cacheAccessModes:
       - ReadWriteOnce

**3. EmptyDir (Default)**

When no cache configuration is provided, uses ephemeral storage.
Suitable for:

- Small repositories
- Testing environments
- When persistence isn't critical

**4. No Cache**

Kopia will operate without cache if explicitly disabled in repository configuration.

**Cache Sizing Guidelines:**

- Small repos (<100GB): 1-2Gi cache
- Medium repos (100GB-1TB): 5-10Gi cache
- Large repos (>1TB): 15-30Gi cache
- Very large repos: 50Gi+ cache

Repository Secret Management
----------------------------

1. **Keep secrets in the same namespace**: The repository secret must exist in the same namespace as the KopiaMaintenance resource
2. **Use descriptive secret names**: Choose names that clearly identify the repository purpose (e.g., ``prod-s3-backup-config``, ``dev-gcs-repo``)
3. **Secure sensitive data**: Ensure repository secrets are properly protected with RBAC

Scheduling Considerations
-------------------------

1. **Avoid peak hours**: Schedule maintenance during low-activity periods
2. **Stagger multiple maintenances**: If managing multiple repositories, use different schedules to avoid resource contention
3. **Consider repository size**: Large repositories may need weekly rather than daily maintenance
4. **Account for time zones**: Schedules are interpreted in the controller's timezone

Resource Allocation
-------------------

1. **Start conservative**: Begin with default resources and adjust based on observed usage
2. **Monitor maintenance jobs**: Check job completion times and resource consumption
3. **Scale for repository size**: Larger repositories require more memory and CPU
4. **Use node affinity**: Direct maintenance to appropriate nodes for large-scale operations

**Resource Recommendations by Repository Size:**

.. list-table::
   :header-rows: 1
   :widths: 30 35 35

   * - Repository Size
     - Memory (Request/Limit)
     - CPU (Request/Limit)
   * - Small (<100GB)
     - 256Mi / 1Gi
     - 100m / 500m
   * - Medium (100GB-1TB)
     - 512Mi / 2Gi
     - 200m / 1
   * - Large (1TB-10TB)
     - 1Gi / 4Gi
     - 500m / 2
   * - Very Large (>10TB)
     - 2Gi / 8Gi
     - 1 / 4

Maintenance Ownership
---------------------

Kopia requires a single user to own maintenance operations. KopiaMaintenance automatically:

1. **Sets identity**: Uses ``maintenance@volsync`` as the maintenance identity
2. **Claims ownership**: Automatically claims or reclaims maintenance ownership
3. **Handles conflicts**: Retries if another user currently owns maintenance
4. **Ensures reliability**: Prevents maintenance failures due to ownership issues

Naming Conventions
------------------

1. **Use descriptive names**: ``prod-daily-maintenance``, ``staging-weekly-cleanup``
2. **Include frequency**: Indicate maintenance schedule in the name when relevant
3. **Match repository purpose**: Align maintenance names with repository naming

Migration Guide
===============

Migrating from maintenanceIntervalDays
---------------------------------------

The ``maintenanceIntervalDays`` field has been removed from ReplicationSource. All maintenance
operations must now be configured through the KopiaMaintenance CRD.

**Old Configuration (No Longer Supported):**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: my-backup
   spec:
     sourcePVC: my-data
     kopia:
       repository: kopia-config
       maintenanceIntervalDays: 7  # REMOVED - NO LONGER SUPPORTED

**New Configuration (Required):**

Create a separate KopiaMaintenance resource:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: my-maintenance
     namespace: same-as-replicationsource
   spec:
     repository:
       repository: kopia-config  # Same secret as ReplicationSource
     trigger:
       schedule: "0 2 * * 0"      # Weekly on Sunday at 2 AM
     # Optional: Add cache for better performance
     cacheCapacity: 10Gi
     cacheStorageClassName: fast-ssd
     cacheAccessModes:
       - ReadWriteOnce

**Migration Benefits:**

- **Independent scheduling**: Maintenance no longer tied to backup frequency
- **Better performance**: Dedicated cache configuration for maintenance
- **Resource control**: Specify CPU/memory limits for maintenance jobs
- **Flexible triggers**: Support for both scheduled and manual maintenance

Migrating from Deprecated schedule Field
----------------------------------------

The ``schedule`` field is deprecated in favor of ``trigger.schedule``. Here's how to migrate:

**Old Configuration:**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: my-maintenance
   spec:
     repository:
       repository: backup-config
     schedule: "0 2 * * *"  # Deprecated field

**New Configuration:**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: my-maintenance
   spec:
     repository:
       repository: backup-config
     trigger:
       schedule: "0 2 * * *"  # New field location

**Backward Compatibility:**

- The deprecated ``schedule`` field continues to work
- If both fields are set, ``trigger.schedule`` takes precedence
- The controller will log warnings when using the deprecated field
- Plan to migrate before the field is removed in a future version

From Embedded maintenanceCronJob
---------------------------------

If you're currently using embedded maintenance configuration in ReplicationSource:

**Before (Embedded Configuration):**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: production
   spec:
     sourcePVC: app-data
     kopia:
       repository: prod-backup-config
       maintenanceCronJob:
         enabled: true
         schedule: "0 2 * * *"
         resources:
           requests:
             memory: "256Mi"

**After (Separate KopiaMaintenance):**

.. code-block:: yaml

   # Step 1: Create KopiaMaintenance resource
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: prod-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     schedule: "0 2 * * *"
     resources:
       requests:
         memory: "256Mi"
       limits:
         memory: "1Gi"

   ---
   # Step 2: Remove maintenanceCronJob from ReplicationSource
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: production
   spec:
     sourcePVC: app-data
     kopia:
       repository: prod-backup-config
       # maintenanceCronJob section removed

Migration Steps
----------------

1. **Create KopiaMaintenance resources** before modifying ReplicationSources
2. **Verify CronJob creation** using ``kubectl get cronjobs -n <namespace>``
3. **Remove embedded configuration** from ReplicationSources
4. **Monitor maintenance execution** to ensure continuity

Adding Cache to Existing Maintenance
------------------------------------

To add cache support to existing maintenance configurations:

**Step 1: Create a cache PVC (if not using auto-creation)**

.. code-block:: yaml

   apiVersion: v1
   kind: PersistentVolumeClaim
   metadata:
     name: kopia-cache
     namespace: production
   spec:
     accessModes:
       - ReadWriteOnce
     storageClassName: fast-ssd
     resources:
       requests:
         storage: 10Gi

**Step 2: Update KopiaMaintenance to use cache**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: prod-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       schedule: "0 2 * * *"
     cachePVC: kopia-cache  # Add this line

**Step 3: Monitor performance improvement**

.. code-block:: bash

   # Check maintenance job duration before and after cache
   kubectl get jobs -n production -l volsync.backube/kopia-maintenance=true \
     -o custom-columns=NAME:.metadata.name,DURATION:.status.completionTime

Advanced Usage
==============

Combining Manual and Scheduled Triggers
----------------------------------------

While you cannot use both triggers simultaneously in a single resource, you can create separate resources for different trigger types:

.. code-block:: yaml

   # Regular scheduled maintenance
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: scheduled-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       schedule: "0 2 * * *"
   ---
   # On-demand maintenance for the same repository
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: manual-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       manual: "on-demand-1"
     enabled: false  # Enable only when needed

Automating Manual Triggers
---------------------------

You can automate manual triggers using kubectl or CI/CD pipelines:

.. code-block:: bash

   #!/bin/bash
   # Script to trigger manual maintenance

   NAMESPACE="production"
   MAINTENANCE_NAME="manual-maintenance"
   TRIGGER_VALUE="manual-$(date +%Y%m%d-%H%M%S)"

   # Update the trigger
   kubectl patch kopiamaintenance $MAINTENANCE_NAME -n $NAMESPACE \
     --type merge -p '{"spec":{"trigger":{"manual":"'$TRIGGER_VALUE'"}}}'

   # Wait for completion
   while true; do
     LAST_SYNC=$(kubectl get kopiamaintenance $MAINTENANCE_NAME -n $NAMESPACE \
       -o jsonpath='{.status.lastManualSync}')
     if [ "$LAST_SYNC" == "$TRIGGER_VALUE" ]; then
       echo "Maintenance completed"
       break
     fi
     echo "Waiting for maintenance to complete..."
     sleep 30
   done

Performance Tuning with Cache
------------------------------

**Cache Warming Strategy:**

For optimal performance, pre-warm the cache before heavy maintenance:

.. code-block:: yaml

   apiVersion: batch/v1
   kind: Job
   metadata:
     name: cache-warmer
     namespace: production
   spec:
     template:
       spec:
         containers:
         - name: kopia
           image: kopia/kopia:latest
           command:
           - kopia
           - repository
           - status
           - --config-file=/tmp/repository/config
           volumeMounts:
           - name: cache
             mountPath: /cache
           - name: repository-config
             mountPath: /tmp/repository
         volumes:
         - name: cache
           persistentVolumeClaim:
             claimName: kopia-cache
         - name: repository-config
           secret:
             secretName: prod-backup-config

Troubleshooting
===============

Common Issues
-------------

Trigger-Related Issues
^^^^^^^^^^^^^^^^^^^^^^

**Manual Trigger Not Working:**

*Symptoms:*

- ``status.lastManualSync`` doesn't update
- No maintenance job created

*Solutions:*

1. Verify trigger value changed:

   .. code-block:: bash

      kubectl get kopiamaintenance <name> -n <namespace> \
        -o jsonpath='{.spec.trigger.manual}'

2. Check for conflicting triggers:

   .. code-block:: bash

      kubectl get kopiamaintenance <name> -n <namespace> \
        -o jsonpath='{.spec.trigger}'

3. Ensure not using both manual and schedule triggers

**Schedule Trigger Using Deprecated Field:**

*Symptoms:*

- Controller warnings about deprecated field usage
- Unexpected scheduling behavior

*Solutions:*

1. Migrate to new trigger format:

   .. code-block:: bash

      kubectl patch kopiamaintenance <name> -n <namespace> --type=json \
        -p='[{"op": "remove", "path": "/spec/schedule"},
             {"op": "add", "path": "/spec/trigger",
              "value": {"schedule": "0 2 * * *"}}]'

Cache-Related Issues
^^^^^^^^^^^^^^^^^^^^

**Cache PVC Not Found:**

*Symptoms:*

- Maintenance jobs fail with volume mount errors
- Events show PVC binding failures

*Solutions:*

1. Verify PVC exists:

   .. code-block:: bash

      kubectl get pvc <cache-pvc-name> -n <namespace>

2. Check PVC is bound:

   .. code-block:: bash

      kubectl get pvc <cache-pvc-name> -n <namespace> -o jsonpath='{.status.phase}'

3. Ensure PVC access modes match job requirements

**Cache Performance Issues:**

*Symptoms:*

- Slow maintenance despite cache
- Cache PVC filling up

*Solutions:*

1. Check cache usage:

   .. code-block:: bash

      kubectl exec -n <namespace> <maintenance-pod> -- df -h /cache

2. Increase cache size if needed
3. Use faster storage class
4. Clear cache if corrupted:

   .. code-block:: bash

      kubectl delete pvc <cache-pvc> -n <namespace>
      # Recreate with larger size

Maintenance Not Running
^^^^^^^^^^^^^^^^^^^^^^^

**Symptoms:**

- No CronJob created in namespace
- ``status.activeCronJob`` is empty

**Solutions:**

1. Verify repository secret exists:

   .. code-block:: bash

      kubectl get secret <repository-secret> -n <namespace>

2. Check KopiaMaintenance status:

   .. code-block:: bash

      kubectl describe kopiamaintenance <name> -n <namespace>

3. Review controller logs for errors:

   .. code-block:: bash

      kubectl logs -n volsync-system deployment/volsync | grep -i kopiamaintenance

Authentication Failures
^^^^^^^^^^^^^^^^^^^^^^^

**Symptoms:**

- Maintenance jobs fail with authentication errors
- Repository access denied messages

**Solutions:**

1. Verify secret contains required fields:

   .. code-block:: bash

      kubectl get secret <repository-secret> -n <namespace> -o jsonpath='{.data}' | jq 'keys'

2. Check secret data is valid and not corrupted
3. Ensure custom CA is properly configured if using self-signed certificates

Resource Exhaustion
^^^^^^^^^^^^^^^^^^^

**Symptoms:**

- Maintenance jobs killed or evicted
- Out of memory errors

**Solutions:**

1. Increase resource limits:

   .. code-block:: yaml

      resources:
        requests:
          memory: "1Gi"
        limits:
          memory: "4Gi"

2. Monitor actual usage:

   .. code-block:: bash

      kubectl top pod -n <namespace> -l job-name=<maintenance-job>

Schedule Not Working
^^^^^^^^^^^^^^^^^^^^

**Symptoms:**

- Jobs not running at expected times
- Incorrect execution frequency

**Solutions:**

1. Validate cron expression using online validators or tools
2. Check controller timezone configuration
3. Verify ``suspend`` is not set to ``true``

Job History for Debugging
^^^^^^^^^^^^^^^^^^^^^^^^^

The job history limits control how much historical data you have available for troubleshooting:

.. code-block:: bash

   # View recent successful maintenance jobs
   kubectl get jobs -n <namespace> -l volsync.backube/kopia-maintenance=true \
     --sort-by=.metadata.creationTimestamp

   # Check job history count
   kubectl get jobs -n <namespace> -l volsync.backube/kopia-maintenance=true \
     -o custom-columns=NAME:.metadata.name,STATUS:.status.succeeded,FAILED:.status.failed,START:.status.startTime

   # View logs from a specific job
   kubectl logs -n <namespace> job/<maintenance-job-name>

   # If you need more history, increase the limits:
   kubectl patch kopiamaintenance <name> -n <namespace> --type merge \
     -p '{"spec":{"successfulJobsHistoryLimit":10,"failedJobsHistoryLimit":5}}'

.. tip::
   If you're troubleshooting maintenance issues and the job history has been
   cleaned up, consider temporarily increasing ``successfulJobsHistoryLimit``
   and ``failedJobsHistoryLimit`` to capture more execution history.

Debugging Commands
------------------

.. code-block:: bash

   # Check KopiaMaintenance resources
   kubectl get kopiamaintenance -A

   # View detailed status with trigger info
   kubectl get kopiamaintenance <name> -n <namespace> -o yaml | grep -A5 trigger

   # Check trigger status
   kubectl get kopiamaintenance <name> -n <namespace> \
     -o jsonpath='{.spec.trigger.manual} -> {.status.lastManualSync}\n'

   # View cache configuration
   kubectl get kopiamaintenance <name> -n <namespace> \
     -o jsonpath='{.spec.cache*}'

   # Check created CronJobs (for scheduled triggers)
   kubectl get cronjobs -n <namespace> -l volsync.backube/kopia-maintenance=true

   # Check Jobs (for manual triggers)
   kubectl get jobs -n <namespace> -l volsync.backube/kopia-maintenance=true

   # View maintenance job logs
   kubectl logs -n <namespace> job/<maintenance-job-name>

   # Check events for errors
   kubectl get events -n <namespace> --field-selector involvedObject.name=<maintenance-name>

   # Monitor cache PVC usage
   kubectl exec -n <namespace> <pod-name> -- df -h /cache

Limitations
===========

Current Limitations
-------------------

1. **Namespace Isolation**: Repository secret must exist in the same namespace as KopiaMaintenance
2. **No Cross-Namespace Management**: Cannot manage repositories in different namespaces
3. **Single Repository**: Each KopiaMaintenance manages exactly one repository
4. **No Repository Discovery**: No automatic detection of repositories or ReplicationSources

Design Rationale
----------------

The simplified design provides:

- **Clear ownership**: Namespace-scoped resources have clear ownership boundaries
- **Better security**: No cross-namespace secret access reduces attack surface
- **Simpler RBAC**: Namespace-level permissions are easier to manage
- **Predictable behavior**: Direct configuration eliminates matching complexity

Performance Considerations
==========================

Cache Impact on Performance
---------------------------

The Kopia cache significantly improves maintenance performance:

**Performance Comparison:**

.. list-table::
   :header-rows: 1
   :widths: 30 35 35

   * - Repository Size
     - Without Cache
     - With Cache
   * - 100GB
     - 15-20 minutes
     - 5-8 minutes
   * - 1TB
     - 2-3 hours
     - 30-45 minutes
   * - 10TB
     - 8-12 hours
     - 2-3 hours

**Cache Optimization Tips:**

1. **Use SSD storage** for cache PVCs when possible
2. **Size appropriately**: 1-2% of repository size is usually sufficient
3. **Monitor cache hit rates** through Kopia logs
4. **Persistent cache** is crucial for large repositories
5. **Share cache** between maintenance and backup operations when possible

Scheduling Optimization
-----------------------

**Best Practices for Scheduling:**

1. **Avoid backup windows**: Don't run maintenance during active backups
2. **Stagger maintenance**: Spread maintenance across different times for multiple repositories
3. **Consider time zones**: Schedule based on application usage patterns
4. **Frequency guidelines**:

   - Daily: Small, frequently changing repositories
   - Weekly: Medium-sized, moderate change rate
   - Monthly: Large, slow-changing archives

**Example Staggered Schedule:**

.. code-block:: yaml

   # Repository 1: 2 AM
   trigger:
     schedule: "0 2 * * *"

   # Repository 2: 3 AM
   trigger:
     schedule: "0 3 * * *"

   # Repository 3: 4 AM
   trigger:
     schedule: "0 4 * * *"

Monitoring and Observability
============================

Key Metrics to Monitor
-----------------------

**Maintenance Health Metrics:**

- ``volsync_kopia_maintenance_last_run_timestamp_seconds``: Last successful maintenance
- ``volsync_kopia_maintenance_duration_seconds``: Maintenance duration
- ``volsync_kopia_maintenance_cronjob_failures_total``: Failed maintenance count

**Repository Health Metrics:**

- Repository size growth rate
- Deduplication ratio
- Number of snapshots
- Orphaned blocks count

Prometheus Queries
------------------

**Alert on Missing Maintenance:**

.. code-block:: promql

   time() - volsync_kopia_maintenance_last_run_timestamp_seconds > 259200

**Track Maintenance Duration Trends:**

.. code-block:: promql

   rate(volsync_kopia_maintenance_duration_seconds[1d])

**Monitor Cache Effectiveness:**

.. code-block:: bash

   # Check cache hit ratio in maintenance logs
   kubectl logs -n <namespace> job/<maintenance-job> | grep -i "cache hit"

Integration with CI/CD
-----------------------

**GitOps Integration Example:**

.. code-block:: yaml

   # In your GitOps repository
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: post-deployment-maintenance
     namespace: production
   spec:
     repository:
       repository: prod-backup-config
     trigger:
       manual: "deployment-${CI_COMMIT_SHA}"  # Trigger after deployment
     cacheCapacity: 20Gi
     resources:
       requests:
         memory: "2Gi"
       limits:
         memory: "4Gi"

**Jenkins Pipeline Example:**

.. code-block:: groovy

   stage('Trigger Maintenance') {
     steps {
       script {
         def triggerValue = "jenkins-${env.BUILD_NUMBER}"
         sh """
           kubectl patch kopiamaintenance manual-maintenance \
             -n production \
             --type merge \
             -p '{"spec":{"trigger":{"manual":"${triggerValue}"}}}'
         """

         // Wait for completion
         timeout(time: 30, unit: 'MINUTES') {
           waitUntil {
             def status = sh(
               script: "kubectl get kopiamaintenance manual-maintenance -n production -o jsonpath='{.status.lastManualSync}'",
               returnStdout: true
             ).trim()
             return status == triggerValue
           }
         }
       }
     }
   }

Next Steps
==========

- Review :doc:`backup-configuration` for repository setup
- Explore :doc:`troubleshooting` for detailed debugging
- Set up monitoring with the :doc:`/examples/kopia/maintenance-alerts`
- Learn about `Kopia's maintenance operations <https://kopia.io/docs/maintenance/>`_ in detail
- Understand cache architecture in `Kopia's performance guide <https://kopia.io/docs/advanced/performance/>`_

Support
=======

For issues or questions:

- GitHub Issues: https://github.com/backube/volsync/issues
- GitHub Discussions: https://github.com/backube/volsync/discussions
- Documentation: https://volsync.readthedocs.io/