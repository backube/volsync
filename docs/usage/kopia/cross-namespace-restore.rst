===============================================
Cross-Namespace Restore Guide
===============================================

.. contents:: Cross-Namespace Restore Operations
   :local:

This guide provides comprehensive documentation for restoring Kopia backups from one namespace into a different namespace in VolSync. Cross-namespace restore is a critical capability for disaster recovery, environment cloning, and namespace migration scenarios.

Overview
========

Cross-namespace restore allows you to restore data backed up from one Kubernetes namespace into a different namespace. This powerful feature enables several important use cases while maintaining proper isolation and security boundaries.

Why Cross-Namespace Restore?
----------------------------

Cross-namespace restore operations are essential for:

**Disaster Recovery**
   When a namespace is accidentally deleted or corrupted, you can restore the data to a new namespace to resume operations quickly.

**Environment Cloning**
   Copy production data to staging or development environments for testing, debugging, or training purposes.

**Namespace Migration**
   Move applications and their data between namespaces as part of reorganization or multi-tenancy changes.

**Testing and Validation**
   Verify backup integrity by restoring to an isolated namespace without affecting the production environment.

**Blue-Green Deployments**
   Create identical environments in separate namespaces for zero-downtime deployments.

How Cross-Namespace Restore Works
----------------------------------

VolSync's Kopia mover uses a combination of username and hostname to identify backup sources in the repository. When performing cross-namespace restores:

1. **Identity Resolution**: VolSync generates or uses the source identity (username@hostname) to locate the correct snapshots
2. **Repository Access**: The destination namespace accesses the same Kopia repository as the source
3. **Snapshot Selection**: The system finds and restores snapshots created by the specified source
4. **Data Restoration**: Data is restored to a new or existing PVC in the destination namespace

The key to successful cross-namespace restore is correctly identifying the source backups using the ``sourceIdentity`` field or explicit username/hostname configuration.

Prerequisites
=============

Before performing a cross-namespace restore, ensure you have:

Required Components
-------------------

1. **VolSync Installed**: VolSync operator must be installed and running in your cluster
2. **Source Backups**: Existing Kopia backups created by a ReplicationSource
3. **Repository Access**: Access to the Kopia repository containing the backups
4. **Target Namespace**: The destination namespace must exist or be created

Required Permissions
--------------------

The VolSync operator requires the following RBAC permissions:

- Read access to ReplicationSource resources (for auto-discovery)
- Create/read/update access to ReplicationDestination resources
- Access to Secrets containing repository credentials
- Create/manage PersistentVolumeClaims in the target namespace

Repository Configuration
------------------------

The repository configuration Secret must be accessible in the destination namespace. You have several options:

1. **Copy the Secret**: Duplicate the repository Secret to the destination namespace
2. **Use a Shared Secret**: Reference a cluster-wide Secret (if supported by your setup)
3. **Create a New Secret**: Create a new Secret with the same repository credentials

Step-by-Step Guide
==================

This section provides detailed instructions for performing cross-namespace restores in various scenarios.

Basic Cross-Namespace Restore
------------------------------

This example demonstrates restoring data from a production namespace to a staging namespace.

**Step 1: Identify the Source Backup**

First, identify the ReplicationSource that created the backups:

.. code-block:: bash

   # List all ReplicationSources across namespaces
   kubectl get replicationsource -A
   
   # Example output:
   # NAMESPACE    NAME            AGE   LAST-SYNC
   # production   webapp-backup   30d   2024-01-20T10:30:00Z
   # production   db-backup       30d   2024-01-20T10:45:00Z

**Step 2: Prepare Repository Access in Target Namespace**

Copy or create the repository configuration Secret in the destination namespace:

.. code-block:: bash

   # Option 1: Copy the secret from source namespace
   kubectl get secret kopia-config -n production -o yaml | \
     sed 's/namespace: production/namespace: staging/' | \
     kubectl apply -f -
   
   # Option 2: Create a new secret with the same credentials
   kubectl create secret generic kopia-config \
     --namespace=staging \
     --from-literal=KOPIA_REPOSITORY=s3://backup-bucket/kopia \
     --from-literal=KOPIA_PASSWORD=your-repository-password \
     --from-literal=AWS_ACCESS_KEY_ID=AKIAIOSFODNN7EXAMPLE \
     --from-literal=AWS_SECRET_ACCESS_KEY=wJalrXUtnFEMI/K7MDENG

**Step 3: Create the ReplicationDestination**

Create a ReplicationDestination in the target namespace using ``sourceIdentity``:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: webapp-restore
     namespace: staging  # Target namespace
   spec:
     trigger:
       manual: restore-once
     kopia:
       # Repository configuration in staging namespace
       repository: kopia-config
       
       # Create or use existing PVC in staging
       destinationPVC: webapp-data-staging
       copyMethod: Direct
       
       # Specify the source backup to restore from
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production  # Source namespace
         # sourcePVCName is auto-discovered from the ReplicationSource

**Step 4: Apply and Monitor**

Apply the configuration and monitor the restore progress:

.. code-block:: bash

   # Apply the ReplicationDestination
   kubectl apply -f webapp-restore.yaml
   
   # Check restore status
   kubectl get replicationdestination webapp-restore -n staging
   
   # Monitor detailed status
   kubectl describe replicationdestination webapp-restore -n staging
   
   # Check if snapshots were found
   kubectl get replicationdestination webapp-restore -n staging \
     -o jsonpath='{.status.kopia.snapshotsFound}'

Advanced Configuration Examples
--------------------------------

**Example 1: Disaster Recovery - Namespace Deleted**

When the source namespace no longer exists, you must provide explicit configuration:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: disaster-recovery
     namespace: production-recovery  # New namespace for recovery
   spec:
     trigger:
       manual: restore-emergency
     kopia:
       repository: kopia-config
       destinationPVC: recovered-data
       copyMethod: Direct
       
       # Source namespace is gone, use explicit identity
       username: webapp-backup-production  # Generated username format
       hostname: production                 # ALWAYS just the namespace name (intentional design)

**Example 2: Environment Cloning with Point-in-Time Recovery**

Clone production data to staging from a specific point in time:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: clone-prod-to-staging
     namespace: staging
   spec:
     trigger:
       manual: clone-once
     kopia:
       repository: kopia-config
       destinationPVC: cloned-app-data
       copyMethod: Direct
       
       # Source identification
       sourceIdentity:
         sourceName: app-backup
         sourceNamespace: production
       
       # Restore from before a specific date/time
       restoreAsOf: "2024-01-15T00:00:00Z"
       
       # Optional: Skip the latest backup at that time
       previous: 1  # Use second-to-last backup before the timestamp

**Example 3: Multi-Tenant Repository Restore**

Restore from a shared repository with multiple tenants:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: tenant-restore
     namespace: tenant-b-prod
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: shared-kopia-repository
       destinationPVC: tenant-b-data
       copyMethod: Direct
       
       # Restore from tenant-a's backup to tenant-b
       sourceIdentity:
         sourceName: database-backup
         sourceNamespace: tenant-a-prod
         # Repository is auto-discovered from source

**Example 4: Testing Restore with Explicit PVC Name**

Test restore procedures with explicit PVC specification:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: test-restore
     namespace: test-environment
   spec:
     trigger:
       manual: test-restore
     kopia:
       repository: kopia-config
       destinationPVC: test-data
       copyMethod: Direct
       
       sourceIdentity:
         sourceName: critical-app-backup
         sourceNamespace: production
         # Explicitly specify PVC name (bypasses auto-discovery)
         sourcePVCName: critical-app-storage
         # Useful when ReplicationSource is not accessible

Common Scenarios
================

This section covers specific use cases with detailed implementation guidance.

Disaster Recovery
-----------------

**Scenario**: Production namespace accidentally deleted, need to restore to a new namespace.

**Challenge**: Original ReplicationSource no longer exists for auto-discovery.

**Solution**:

1. **Create Recovery Namespace**:

   .. code-block:: bash

      kubectl create namespace production-recovery

2. **Restore Repository Access**:

   Create the repository Secret using backed-up credentials or from documentation:

   .. code-block:: yaml

      apiVersion: v1
      kind: Secret
      metadata:
        name: kopia-config
        namespace: production-recovery
      type: Opaque
      stringData:
        KOPIA_REPOSITORY: s3://disaster-recovery/kopia
        KOPIA_PASSWORD: ${BACKUP_PASSWORD}
        AWS_ACCESS_KEY_ID: ${AWS_KEY}
        AWS_SECRET_ACCESS_KEY: ${AWS_SECRET}

3. **Discover Available Backups**:

   Create a temporary ReplicationDestination to list available identities:

   .. code-block:: yaml

      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: identity-discovery
        namespace: production-recovery
      spec:
        trigger:
          manual: discover
        kopia:
          repository: kopia-config
          destinationPVC: temp-pvc
          copyMethod: Direct

   Check available identities:

   .. code-block:: bash

      kubectl get replicationdestination identity-discovery \
        -n production-recovery -o json | jq '.status.kopia.availableIdentities'

4. **Restore Critical Data**:

   Based on discovered identities, restore each application:

   .. code-block:: yaml

      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: restore-webapp
        namespace: production-recovery
      spec:
        trigger:
          manual: restore-now
        kopia:
          repository: kopia-config
          destinationPVC: webapp-data
          copyMethod: Direct
          # Use explicit identity from discovery
          username: webapp-backup-production
          hostname: production

Environment Cloning
-------------------

**Scenario**: Clone production data to staging for testing a major upgrade.

**Implementation**:

1. **Prepare Staging Environment**:

   .. code-block:: bash

      # Ensure staging namespace exists
      kubectl create namespace staging --dry-run=client -o yaml | kubectl apply -f -
      
      # Copy repository credentials
      kubectl get secret kopia-config -n production -o yaml | \
        kubectl create -f - -n staging --dry-run=client -o yaml | \
        kubectl apply -f -

2. **Clone Multiple Applications**:

   Create a batch restore configuration:

   .. code-block:: yaml

      # Clone database
      ---
      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: clone-database
        namespace: staging
      spec:
        trigger:
          manual: clone-once
        kopia:
          destinationPVC: staging-database
          copyMethod: Direct
          sourceIdentity:
            sourceName: database-backup
            sourceNamespace: production
      ---
      # Clone application data
      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: clone-appdata
        namespace: staging
      spec:
        trigger:
          manual: clone-once
        kopia:
          destinationPVC: staging-appdata
          copyMethod: Direct
          sourceIdentity:
            sourceName: app-backup
            sourceNamespace: production

3. **Verify Cloned Data**:

   .. code-block:: bash

      # Check restore completion
      kubectl get replicationdestination -n staging
      
      # Verify PVCs created
      kubectl get pvc -n staging
      
      # Check application readiness
      kubectl get pods -n staging

Testing Restore Procedures
--------------------------

**Scenario**: Regularly test backup integrity without affecting production.

**Automated Test Process**:

1. **Create Test Namespace**:

   .. code-block:: yaml

      apiVersion: v1
      kind: Namespace
      metadata:
        name: backup-test
        labels:
          purpose: backup-validation
          temporary: "true"

2. **Automated Test Restore**:

   .. code-block:: yaml

      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: test-restore
        namespace: backup-test
        annotations:
          test-date: "2024-01-20"
          test-id: "weekly-validation"
      spec:
        trigger:
          manual: test-now
        kopia:
          repository: kopia-config
          destinationPVC: test-data
          copyMethod: Direct
          sourceIdentity:
            sourceName: critical-backup
            sourceNamespace: production
          # Test with an older snapshot
          previous: 2

3. **Validation Script**:

   .. code-block:: bash

      #!/bin/bash
      # Automated restore test script
      
      TEST_NS="backup-test-$(date +%Y%m%d)"
      
      # Create test namespace
      kubectl create namespace $TEST_NS
      
      # Copy repository secret
      kubectl get secret kopia-config -n production -o yaml | \
        sed "s/namespace: production/namespace: $TEST_NS/" | \
        kubectl apply -f -
      
      # Apply test restore
      cat <<EOF | kubectl apply -f -
      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: test-restore
        namespace: $TEST_NS
      spec:
        trigger:
          manual: test-$(date +%s)
        kopia:
          repository: kopia-config
          destinationPVC: test-data
          copyMethod: Direct
          sourceIdentity:
            sourceName: app-backup
            sourceNamespace: production
      EOF
      
      # Wait for completion
      kubectl wait --for=condition=Synchronizing=false \
        replicationdestination/test-restore -n $TEST_NS \
        --timeout=300s
      
      # Validate data (custom validation logic here)
      # ...
      
      # Cleanup
      kubectl delete namespace $TEST_NS

Namespace Migration
-------------------

**Scenario**: Migrate application from old namespace to new namespace structure.

**Migration Process**:

1. **Final Backup in Old Namespace**:

   .. code-block:: bash

      # Trigger final backup
      kubectl patch replicationsource app-backup -n old-namespace \
        --type merge -p '{"spec":{"trigger":{"manual":"final-backup"}}}'
      
      # Wait for completion
      kubectl wait --for=condition=Synchronizing=false \
        replicationsource/app-backup -n old-namespace

2. **Restore to New Namespace**:

   .. code-block:: yaml

      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: migrate-app
        namespace: new-namespace
      spec:
        trigger:
          manual: migrate-once
        kopia:
          repository: kopia-config
          destinationPVC: migrated-app-data
          copyMethod: Direct
          sourceIdentity:
            sourceName: app-backup
            sourceNamespace: old-namespace
          # Ensure we get the latest backup
          restoreAsOf: "2024-01-20T15:00:00Z"

Important Considerations
========================

Security Implications
---------------------

**Repository Access Control**
   Cross-namespace restore requires careful management of repository credentials:
   
   - Use separate repository passwords for different security zones
   - Implement RBAC policies to control who can create ReplicationDestinations
   - Audit repository access and restore operations
   - Consider using separate repositories for sensitive data

**Secret Management**
   Repository credentials must be properly secured:
   
   - Use Kubernetes Secrets encryption at rest
   - Implement Secret rotation policies
   - Use tools like Sealed Secrets or External Secrets Operator
   - Limit Secret access to specific ServiceAccounts

**Data Classification**
   Consider data sensitivity when performing cross-namespace restores:
   
   - Production data should not be restored to less secure environments
   - Implement data masking for non-production restores
   - Document data flow between namespaces
   - Comply with data residency requirements

RBAC Requirements
-----------------

Configure appropriate RBAC for cross-namespace operations:

.. code-block:: yaml

   # Role for reading ReplicationSources (for auto-discovery)
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRole
   metadata:
     name: volsync-source-reader
   rules:
   - apiGroups: ["volsync.backube"]
     resources: ["replicationsources"]
     verbs: ["get", "list"]
   
   ---
   # Binding for VolSync operator
   apiVersion: rbac.authorization.k8s.io/v1
   kind: ClusterRoleBinding
   metadata:
     name: volsync-source-reader-binding
   roleRef:
     apiGroup: rbac.authorization.k8s.io
     kind: ClusterRole
     name: volsync-source-reader
   subjects:
   - kind: ServiceAccount
     name: volsync
     namespace: volsync-system

Secret Management
-----------------

**Best Practices for Repository Secrets**:

1. **Namespace Isolation**: Each namespace should have its own copy of repository Secrets
2. **Credential Rotation**: Regularly rotate repository passwords
3. **Access Logging**: Monitor Secret access through Kubernetes audit logs
4. **Encryption**: Enable encryption at rest for etcd

**Example: Automated Secret Replication**:

.. code-block:: yaml

   # Using Kyverno to replicate secrets
   apiVersion: kyverno.io/v1
   kind: ClusterPolicy
   metadata:
     name: replicate-kopia-secrets
   spec:
     generateExistingOnPolicyUpdate: true
     rules:
     - name: replicate-repository-secret
       match:
         any:
         - resources:
             kinds:
             - Namespace
             selector:
               matchLabels:
                 needs-kopia-repository: "true"
       generate:
         synchronize: true
         apiVersion: v1
         kind: Secret
         name: kopia-config
         namespace: "{{request.object.metadata.name}}"
         clone:
           namespace: production
           name: kopia-config

StorageClass Compatibility
--------------------------

Ensure StorageClass compatibility between namespaces:

**Considerations**:

- Destination namespace must have access to compatible StorageClasses
- Volume expansion capabilities should match if needed
- Performance characteristics (SSD vs HDD) should be considered
- Regional availability for cloud storage

**Example: Specifying StorageClass**:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-with-storageclass
     namespace: target-namespace
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       storageClassName: fast-ssd  # Explicitly specify StorageClass
       accessModes:
       - ReadWriteOnce
       capacity: 10Gi
       sourceIdentity:
         sourceName: app-backup
         sourceNamespace: source-namespace

PVC Access Modes
----------------

Match access modes appropriately:

**Access Mode Compatibility Matrix**:

- **ReadWriteOnce (RWO)**: Standard for single-node access
- **ReadWriteMany (RWX)**: Required for multi-pod access
- **ReadOnlyMany (ROX)**: For read-only shared access

**Example: Multi-Pod Access Configuration**:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-shared-storage
     namespace: target-namespace
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: shared-data
       copyMethod: Direct
       accessModes:
       - ReadWriteMany  # Enable multi-pod access
       capacity: 20Gi
       sourceIdentity:
         sourceName: shared-backup
         sourceNamespace: source-namespace

Troubleshooting
===============

This section addresses common issues and their solutions.

Common Errors and Solutions
---------------------------

**Error: No snapshots found**

*Symptom*: ReplicationDestination shows ``snapshotsFound: 0``

*Diagnosis*:

.. code-block:: bash

   # Check the requested identity
   kubectl get replicationdestination <name> -n <namespace> \
     -o jsonpath='{.status.kopia.requestedIdentity}'
   
   # List available identities
   kubectl get replicationdestination <name> -n <namespace> \
     -o json | jq '.status.kopia.availableIdentities'

*Solutions*:

1. Verify source namespace and name are correct
2. Check if backups exist for the source
3. Ensure repository Secret is correctly configured
4. Verify the ReplicationSource has run successfully

**Error: Repository access denied**

*Symptom*: Authentication or authorization errors

*Solutions*:

1. Verify repository credentials in Secret:

   .. code-block:: bash

      kubectl get secret kopia-config -n <namespace> -o yaml

2. Check repository connectivity:

   .. code-block:: bash

      # Test repository access from a debug pod
      kubectl run -n <namespace> debug --rm -i --tty \
        --image=kopia/kopia:latest \
        --env-from=secret/kopia-config \
        -- kopia repository status

**Error: PVC already exists**

*Symptom*: Cannot create destination PVC

*Solutions*:

1. Use existing PVC:

   .. code-block:: yaml

      spec:
        kopia:
          destinationPVC: existing-pvc  # Reference existing PVC

2. Or specify a different name:

   .. code-block:: yaml

      spec:
        kopia:
          destinationPVC: new-unique-pvc  # Use different name

Verifying Restore Success
-------------------------

**Step 1: Check ReplicationDestination Status**

.. code-block:: bash

   # Overall status
   kubectl get replicationdestination <name> -n <namespace>
   
   # Detailed conditions
   kubectl describe replicationdestination <name> -n <namespace>
   
   # Check if synchronization completed
   kubectl get replicationdestination <name> -n <namespace> \
     -o jsonpath='{.status.conditions[?(@.type=="Synchronizing")].status}'

**Step 2: Verify Data Integrity**

.. code-block:: bash

   # Check PVC creation
   kubectl get pvc -n <namespace>
   
   # Mount and verify data
   kubectl run -n <namespace> verify --rm -i --tty \
     --image=busybox \
     --overrides='{"spec":{"containers":[{
       "name":"verify",
       "image":"busybox",
       "command":["sh"],
       "volumeMounts":[{"name":"data","mountPath":"/data"}]
     }],"volumes":[{
       "name":"data",
       "persistentVolumeClaim":{"claimName":"restored-data"}
     }]}}' \
     -- ls -la /data

**Step 3: Application Validation**

Deploy your application using the restored data and verify functionality:

.. code-block:: yaml

   apiVersion: apps/v1
   kind: Deployment
   metadata:
     name: app-with-restored-data
     namespace: target-namespace
   spec:
     replicas: 1
     selector:
       matchLabels:
         app: restored-app
     template:
       metadata:
         labels:
           app: restored-app
       spec:
         containers:
         - name: app
           image: myapp:latest
           volumeMounts:
           - name: data
             mountPath: /app/data
         volumes:
         - name: data
           persistentVolumeClaim:
             claimName: restored-data

Debugging Identity Mismatches
-----------------------------

When identity mismatches occur, follow this systematic approach:

**Step 1: Discovery Phase**

Create a discovery ReplicationDestination:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: debug-discovery
     namespace: target-namespace
   spec:
     trigger:
       manual: discover
     kopia:
       repository: kopia-config
       destinationPVC: temp-discovery
       copyMethod: Direct
       # Don't specify identity to see all available

**Step 2: Analyze Available Identities**

.. code-block:: bash

   # Get full list of identities
   kubectl get replicationdestination debug-discovery \
     -n target-namespace -o json | jq '.status.kopia.availableIdentities'
   
   # Filter for specific namespace
   kubectl get replicationdestination debug-discovery \
     -n target-namespace -o json | \
     jq '.status.kopia.availableIdentities[] | select(.identity | contains("production"))'

**Step 3: Match Identity Format**

Understanding identity format:
- Username: ``{source-name}-{namespace}`` or custom
- Hostname: ``{namespace}`` (always just namespace unless custom)
- Full identity: ``{username}@{hostname}``

**Step 4: Test with Correct Identity**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: test-correct-identity
     namespace: target-namespace
   spec:
     trigger:
       manual: test-identity
     kopia:
       repository: kopia-config
       destinationPVC: test-restore
       copyMethod: Direct
       # Use discovered identity
       username: "webapp-backup-production"
       hostname: "production"

Best Practices
==============

To ensure successful cross-namespace restore operations, follow these best practices.

Naming Conventions
------------------

**Consistent Naming Across Environments**

Maintain consistent naming for ReplicationSources across environments:

.. code-block:: yaml

   # Production
   metadata:
     name: app-backup  # Same name
     namespace: production
   
   # Staging
   metadata:
     name: app-backup  # Same name
     namespace: staging
   
   # Development
   metadata:
     name: app-backup  # Same name
     namespace: development

This simplifies restore procedures and automation.

**Descriptive Resource Names**

Use clear, descriptive names that indicate purpose:

.. code-block:: yaml

   # Good naming examples
   name: mysql-daily-backup
   name: webapp-hourly-backup
   name: restore-prod-to-staging
   name: disaster-recovery-webapp

Documentation Requirements
--------------------------

**Maintain Restore Runbooks**

Document critical information for each backup:

.. code-block:: yaml

   # restore-runbook.yaml
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: restore-runbook
     namespace: operations
   data:
     webapp-backup: |
       Source: production/webapp-backup
       Repository: kopia-config
       Schedule: Every 6 hours
       Retention: 30 days
       Critical: Yes
       Contact: platform-team@company.com
       Restore Procedure:
         1. Create namespace if needed
         2. Copy kopia-config secret
         3. Apply restore-webapp.yaml
         4. Verify with test-webapp.sh

**Track Source Locations**

Maintain a registry of backup sources:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: backup-registry
     namespace: volsync-system
   data:
     sources.yaml: |
       production:
         - name: webapp-backup
           pvc: webapp-data
           schedule: "0 */6 * * *"
         - name: database-backup
           pvc: postgres-data
           schedule: "0 2 * * *"
       staging:
         - name: webapp-backup
           pvc: webapp-staging-data
           schedule: "0 0 * * *"

Testing Procedures
------------------

**Regular Restore Tests**

Implement automated testing of restore procedures:

.. code-block:: bash

   #!/bin/bash
   # test-restore.sh - Weekly restore test
   
   set -e
   
   TEST_NS="restore-test-$(date +%Y%m%d)"
   SOURCE_NS="production"
   SOURCE_NAME="critical-backup"
   
   echo "Starting restore test to $TEST_NS"
   
   # Create test namespace
   kubectl create namespace $TEST_NS
   
   # Copy repository secret
   kubectl get secret kopia-config -n $SOURCE_NS -o yaml | \
     sed "s/namespace: $SOURCE_NS/namespace: $TEST_NS/" | \
     kubectl apply -f -
   
   # Create restore
   cat <<EOF | kubectl apply -f -
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: test-restore
     namespace: $TEST_NS
   spec:
     trigger:
       manual: test-$(date +%s)
     kopia:
       repository: kopia-config
       destinationPVC: test-data
       copyMethod: Direct
       sourceIdentity:
         sourceName: $SOURCE_NAME
         sourceNamespace: $SOURCE_NS
   EOF
   
   # Wait for completion
   timeout 600 kubectl wait --for=condition=Synchronizing=false \
     replicationdestination/test-restore -n $TEST_NS
   
   # Verify data exists
   kubectl run -n $TEST_NS verify --rm -i --image=busybox \
     --restart=Never \
     --overrides='{"spec":{"containers":[{
       "name":"verify",
       "image":"busybox",
       "command":["sh","-c","ls -la /data && exit 0"],
       "volumeMounts":[{"name":"data","mountPath":"/data"}]
     }],"volumes":[{
       "name":"data",
       "persistentVolumeClaim":{"claimName":"test-data"}
     }]}}'
   
   # Cleanup
   kubectl delete namespace $TEST_NS
   
   echo "Restore test completed successfully"

**Monitoring and Alerting**

Set up monitoring for restore operations:

.. code-block:: yaml

   # PrometheusRule for restore monitoring
   apiVersion: monitoring.coreos.com/v1
   kind: PrometheusRule
   metadata:
     name: volsync-restore-alerts
     namespace: monitoring
   spec:
     groups:
     - name: volsync-restore
       interval: 5m
       rules:
       - alert: RestoreOperationFailed
         expr: |
           kube_replicationdestination_status_condition{condition="Synchronizing",status="false"}
           * on(namespace,name) 
           kube_replicationdestination_info{reason!="Completed"}
         for: 10m
         annotations:
           summary: "Restore operation failed in {{ $labels.namespace }}/{{ $labels.name }}"
           description: "ReplicationDestination {{ $labels.name }} in namespace {{ $labels.namespace }} has failed"

Summary
=======

Cross-namespace restore is a powerful feature of VolSync's Kopia integration that enables critical operational scenarios including disaster recovery, environment cloning, and namespace migration. Success depends on:

1. **Correct Identity Configuration**: Use ``sourceIdentity`` for simplified configuration or explicit username/hostname for full control
2. **Repository Access Management**: Ensure proper Secret configuration in target namespaces
3. **Security Considerations**: Implement appropriate RBAC and Secret management practices
4. **Regular Testing**: Validate restore procedures before they're needed in emergencies
5. **Documentation**: Maintain clear records of backup sources and restore procedures

By following this guide and implementing the best practices, you can confidently perform cross-namespace restore operations while maintaining security and operational excellence.

Additional Resources
====================

- :doc:`restore-configuration` - General restore configuration guide
- :doc:`multi-tenancy` - Understanding identity management in multi-tenant setups
- :doc:`troubleshooting` - Comprehensive troubleshooting guide
- :doc:`backup-configuration` - Setting up ReplicationSources for backups

For more information about VolSync and Kopia integration, refer to the main documentation index.