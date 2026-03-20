===============================
KopiaMaintenance CRD Reference
===============================

.. sidebar:: Contents

   .. contents:: KopiaMaintenance CRD
      :local:

Overview and Introduction
=========================

What is KopiaMaintenance?
-------------------------

The KopiaMaintenance Custom Resource Definition (CRD) is a cluster-scoped resource that provides centralized, flexible management of Kopia repository maintenance operations in VolSync.

**Key Benefits:**

- **Centralized Management**: Single point of control for maintenance across all namespaces
- **Advanced Matching**: Sophisticated repository and namespace selection capabilities
- **Priority-Based Resolution**: Automatic conflict resolution when multiple configurations target the same repository
- **Cross-Namespace Operations**: Seamless secret management across namespace boundaries
- **Enhanced Flexibility**: Support for both repository selector and direct repository modes
- **Better Resource Management**: Optimized CronJob creation and lifecycle management

Why KopiaMaintenance was Created
--------------------------------

Traditional maintenance configuration had several limitations:

1. **Limited Flexibility**: Each ReplicationSource required its own maintenance configuration
2. **No Cross-Namespace Management**: Maintenance was confined to individual namespaces
3. **Conflict Resolution**: No systematic way to handle conflicting maintenance schedules
4. **Resource Duplication**: Multiple ReplicationSources using the same repository created duplicate maintenance CronJobs
5. **Complex Administration**: Difficult to manage maintenance policies at scale

The KopiaMaintenance CRD addresses these limitations by providing:

- Cluster-wide maintenance policy management
- Intelligent repository matching and deduplication
- Priority-based conflict resolution
- Cross-namespace secret management
- Centralized monitoring and observability

When to Use KopiaMaintenance
----------------------------

**Use KopiaMaintenance when:**

- Managing maintenance for multiple repositories across namespaces
- Implementing organization-wide maintenance policies
- Requiring advanced matching criteria (wildcards, labels, namespace selectors)
- Setting up maintenance for repositories without ReplicationSources
- Need priority-based conflict resolution for shared repositories
- Centralizing maintenance operations for better observability

CRD Specification
=================

Complete Field Reference
------------------------

KopiaMaintenanceSpec Fields
^^^^^^^^^^^^^^^^^^^^^^^^^^^

**repositorySelector** (*KopiaRepositorySelector*, optional)
   Repository matching configuration for finding existing ReplicationSources.
   This approach matches ReplicationSources by their repository configuration.
   Either ``repositorySelector`` OR ``repository`` must be specified, but not both.

**repository** (*KopiaRepositorySpec*, optional)
   Repository defines a direct repository configuration for maintenance.
   This approach allows KopiaMaintenance to work independently of ReplicationSources.
   Either ``repositorySelector`` OR ``repository`` must be specified, but not both.

**schedule** (*string*, optional, default: "0 2 * * *")
   Cron schedule for when maintenance should run. The schedule is interpreted
   in the controller's timezone. Must match the pattern for valid cron expressions.

**enabled** (*bool*, optional, default: true)
   Determines if maintenance should be performed. When false, no maintenance
   will be scheduled.

**priority** (*int32*, optional, default: 0)
   Priority of this maintenance configuration. When multiple KopiaMaintenance
   resources match the same repository, the one with the highest priority wins.
   Range: -100 to 100.

**suspend** (*bool*, optional)
   Can be used to temporarily stop maintenance. When true, the
   CronJob will not create new Jobs, but existing Jobs will be allowed
   to complete.

**successfulJobsHistoryLimit** (*int32*, optional, default: 3)
   Specifies how many successful maintenance Jobs should be kept.
   Minimum: 0.

**failedJobsHistoryLimit** (*int32*, optional, default: 1)
   Specifies how many failed maintenance Jobs should be kept.
   Minimum: 0.

**resources** (*ResourceRequirements*, optional)
   Compute resources required by the maintenance container.
   If not specified, defaults to 256Mi memory request and 1Gi memory limit.

**activeDeadlineSeconds** (*int64*, optional, default: 10800)
   Specifies the duration in seconds relative to the startTime that the job
   may be active before the system tries to terminate it. If not specified,
   defaults to 10800 (3 hours). This prevents maintenance jobs from running
   indefinitely. For repositories requiring longer maintenance windows (e.g.,
   very large repositories that take 8+ hours), increase this value.
   Minimum: 600 (10 minutes).

**podSecurityContext** (*PodSecurityContext*, optional)
   Security context for the entire maintenance pod.

**securityContext** (*SecurityContext*, optional)
   Security context for the maintenance container.

**nodeSelector** (*map[string]string*, optional)
   NodeSelector for scheduling maintenance pods on specific nodes.

**tolerations** (*[]Toleration*, optional)
   Tolerations for scheduling maintenance pods.

**affinity** (*Affinity*, optional)
   Affinity settings for pod scheduling.

KopiaRepositorySelector Fields
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

**repository** (*string*, required)
   Repository name or pattern to match. Supports wildcards (* and ?).
   Use "*" to match all repositories.

**namespaceSelector** (*NamespaceSelector*, optional)
   Criteria for selecting namespaces. If empty, all namespaces are considered.

NamespaceSelector Fields
^^^^^^^^^^^^^^^^^^^^^^^

**matchNames** (*[]string*, optional)
   List of namespace names to include. Empty means all namespaces.

**matchLabels** (*map[string]string*, optional)
   Label selector for namespaces. Only namespaces matching all labels
   will be considered.

KopiaRepositorySpec Fields
^^^^^^^^^^^^^^^^^^^^^^^^^

**repository** (*string*, required)
   The name of a Secret containing repository configuration.

**namespace** (*string*, required)
   The namespace containing the repository Secret.

**customCA** (*CustomCASpec*, optional)
   Custom CA configuration for secure connections.

Operating Modes
===============

Repository Selector Mode
------------------------

This mode matches existing ReplicationSources by repository configuration patterns.

**Key Features:**

- Pattern matching with wildcards (* and ?)
- Namespace filtering via names or labels
- Automatic discovery of ReplicationSources
- Dynamic adaptation to changes

**Example:**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: all-production-repos
   spec:
     repositorySelector:
       repository: "prod-*"  # Match all repositories starting with "prod-"
       namespaceSelector:
         matchLabels:
           environment: production
     schedule: "0 2 * * *"
     priority: 10

Direct Repository Mode
----------------------

This mode allows direct repository configuration without requiring ReplicationSources.

**Key Features:**

- Independent of ReplicationSource resources
- Direct secret reference
- Useful for maintenance-only scenarios
- Simpler configuration for known repositories

**Example:**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: central-backup-maintenance
   spec:
     repository:
       name: central-backup-config
       namespace: backup-system
     schedule: "0 3 * * 0"  # Weekly on Sunday at 3 AM
     resources:
       requests:
         memory: "512Mi"
       limits:
         memory: "2Gi"

Advanced Matching Criteria
==========================

Wildcard Patterns
-----------------

Supported wildcard patterns for repository matching:

.. list-table::
   :header-rows: 1
   :widths: 20 40 40

   * - Pattern
     - Description
     - Example Matches
   * - ``*``
     - Match all repositories
     - Any repository
   * - ``prod-*``
     - Match prefix
     - prod-db, prod-app, prod-cache
   * - ``*-backup``
     - Match suffix
     - mysql-backup, redis-backup
   * - ``app-?-data``
     - Single character wildcard
     - app-1-data, app-2-data, app-a-data
   * - ``*-prod-*``
     - Multiple wildcards
     - us-prod-db, eu-prod-app

Namespace Selection
-------------------

**By Name:**

.. code-block:: yaml

   namespaceSelector:
     matchNames:
       - production
       - staging
       - qa

**By Labels:**

.. code-block:: yaml

   namespaceSelector:
     matchLabels:
       environment: production
       team: platform

**Combined Selection:**

.. code-block:: yaml

   namespaceSelector:
     matchNames:
       - critical-apps
     matchLabels:
       backup-required: "true"

Priority-Based Conflict Resolution
==================================

When multiple KopiaMaintenance resources match the same repository, priority determines which one is used:

1. Higher priority values win (range: -100 to 100)
2. If priorities are equal, the first created resource wins
3. Default priority is 0

**Example Priority Hierarchy:**

.. code-block:: yaml

   # Highest priority - specific override
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: critical-app-override
   spec:
     repositorySelector:
       repository: "critical-app-backup"
     priority: 50  # Highest priority
     schedule: "*/30 * * * *"  # Every 30 minutes

   ---
   # Medium priority - department policy
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: finance-dept-maintenance
   spec:
     repositorySelector:
       repository: "finance-*"
     priority: 10
     schedule: "0 1 * * *"  # Daily at 1 AM

   ---
   # Default priority - organization-wide
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: global-maintenance
   spec:
     repositorySelector:
       repository: "*"
     priority: 0  # Default
     schedule: "0 2 * * *"  # Daily at 2 AM

Best Practices
==============

Organization Strategy
---------------------

**1. Hierarchical Configuration**

Start with broad, low-priority configurations and add specific overrides:

- Global default (priority: 0)
- Environment-specific (priority: 10)
- Department/team-specific (priority: 20)
- Application-specific overrides (priority: 30+)

**2. Naming Conventions**

Use descriptive names that indicate scope and purpose:

- ``global-maintenance-default``
- ``prod-env-maintenance``
- ``team-finance-maintenance``
- ``app-critical-db-override``

Resource Management
-------------------

**Memory Configuration:**

.. code-block:: yaml

   resources:
     requests:
       memory: "256Mi"  # Minimum for small repos
     limits:
       memory: "2Gi"    # Cap for large operations

**Scheduling Best Practices:**

- Schedule maintenance during low-traffic periods
- Stagger schedules for different repositories
- Consider timezone implications
- Use suspend feature for maintenance windows

Monitoring and Observability
----------------------------

**Status Monitoring:**

.. code-block:: bash

   # Watch all KopiaMaintenance resources
   kubectl get kopiamaintenance -w

   # Check specific maintenance status
   kubectl describe kopiamaintenance production-maintenance

**CronJob Monitoring:**

.. code-block:: bash

   # View all maintenance CronJobs
   kubectl get cronjobs -n volsync-system \
     -l volsync.backube/kopia-maintenance=true

   # Check recent job executions
   kubectl get jobs -n volsync-system \
     -l volsync.backube/kopia-maintenance=true \
     --sort-by=.metadata.creationTimestamp

Security Considerations
=======================

Cross-Namespace Secret Access
-----------------------------

KopiaMaintenance can access secrets across namespaces. This requires proper RBAC configuration:

**Required Permissions:**

- Read access to repository secrets in specified namespaces
- Create/update permissions for CronJobs in volsync-system
- List/watch permissions for ReplicationSources

**Security Best Practices:**

1. Limit secret access to necessary namespaces only
2. Use namespace selectors to restrict scope
3. Regularly audit KopiaMaintenance resources
4. Monitor cross-namespace access patterns

Pod Security
------------

**Recommended Security Context:**

.. code-block:: yaml

   podSecurityContext:
     runAsNonRoot: true
     runAsUser: 1000
     fsGroup: 1000
     seccompProfile:
       type: RuntimeDefault

   securityContext:
     allowPrivilegeEscalation: false
     capabilities:
       drop:
         - ALL
     readOnlyRootFilesystem: true

Troubleshooting
===============

Common Issues
-------------

**No Matching ReplicationSources**

*Symptoms:* ``status.matchedSources`` is empty

*Checks:*

1. Verify repository pattern matches actual names
2. Check namespace selector criteria
3. Ensure ReplicationSources exist in selected namespaces
4. Review controller logs for matching details

**CronJob Not Created**

*Symptoms:* No CronJob in volsync-system namespace

*Checks:*

1. Verify ``enabled: true`` in spec
2. Check if higher priority maintenance exists
3. Ensure repository secret exists and is accessible
4. Review controller logs for errors

**Maintenance Jobs Failing**

*Symptoms:* Jobs in Failed state

*Checks:*

1. Check job logs: ``kubectl logs -n volsync-system job/<job-name>``
2. Verify repository credentials are valid
3. Check resource limits aren't too restrictive
4. Ensure network connectivity to repository

Debugging Commands
------------------

.. code-block:: bash

   # List all maintenances with their matched sources
   kubectl get kopiamaintenance -o custom-columns=\
     NAME:.metadata.name,\
     PRIORITY:.spec.priority,\
     MATCHED:.status.matchedSources[*].name

   # Check controller logs for maintenance processing
   kubectl logs -n volsync-system deployment/volsync | \
     grep -i kopiamaintenance

   # View maintenance job history
   kubectl get jobs -n volsync-system \
     -l volsync.backube/kopia-maintenance=true \
     -o custom-columns=\
     NAME:.metadata.name,\
     STATUS:.status.conditions[0].type,\
     START:.status.startTime,\
     COMPLETE:.status.completionTime

Special Cases
=============

Filesystem Repositories with moverVolumes
------------------------------------------

ReplicationSources using ``moverVolumes`` for filesystem repositories work with KopiaMaintenance. The repository
secret should be configured with the appropriate filesystem URL (e.g., ``filesystem:///mnt/<mountPath>``).

Multiple Repositories Same Namespace
------------------------------------

When multiple repositories exist in the same namespace, use specific repository patterns:

.. code-block:: yaml

   # Target specific repository
   repositorySelector:
     repository: "app-backup-config"  # Exact match
     namespaceSelector:
       matchNames: ["production"]

Complete Examples
=================

Example 1: Global Default with Overrides
-----------------------------------------

.. code-block:: yaml

   # Global default maintenance
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: global-default
   spec:
     repositorySelector:
       repository: "*"
     schedule: "0 2 * * *"
     priority: 0
     resources:
       requests:
         memory: "256Mi"
       limits:
         memory: "1Gi"

   ---
   # Production override with higher frequency
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: production-frequent
   spec:
     repositorySelector:
       repository: "*"
       namespaceSelector:
         matchLabels:
           environment: production
     schedule: "0 */6 * * *"  # Every 6 hours
     priority: 20
     resources:
       requests:
         memory: "512Mi"
       limits:
         memory: "2Gi"

Example 2: Team-Based Maintenance
----------------------------------

.. code-block:: yaml

   # Platform team repositories
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: platform-team-maintenance
   spec:
     repositorySelector:
       repository: "platform-*"
       namespaceSelector:
         matchLabels:
           team: platform
     schedule: "0 3 * * *"
     priority: 15
     nodeSelector:
       workload: maintenance
     tolerations:
       - key: maintenance
         operator: Equal
         value: "true"
         effect: NoSchedule

Example 3: Direct Repository Configuration
-------------------------------------------

.. code-block:: yaml

   # Direct maintenance for specific repository
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: central-backup-maintenance
   spec:
     repository:
       name: central-backup-secret
       namespace: backup-system
       customCA:
         configMapName: custom-ca-bundle
         key: ca.crt
     schedule: "0 4 * * 0"  # Weekly on Sunday
     activeDeadlineSeconds: 28800  # 8 hours for large repository
     resources:
       requests:
         memory: "1Gi"
         cpu: "500m"
       limits:
         memory: "4Gi"
         cpu: "2"
     affinity:
       nodeAffinity:
         preferredDuringSchedulingIgnoredDuringExecution:
           - weight: 100
             preference:
               matchExpressions:
                 - key: node-type
                   operator: In
                   values:
                     - maintenance