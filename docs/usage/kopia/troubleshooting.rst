=======================
Troubleshooting Guide
=======================

.. contents:: Troubleshooting Kopia Operations
   :local:

This guide provides comprehensive troubleshooting information for Kopia-based backup
and restore operations in VolSync, with a focus on the enhanced error reporting and
snapshot discovery features.

Quick Reference: Common Issues
===============================

This section provides quick solutions to the most common Kopia issues:

.. list-table:: Common Issues Quick Reference
   :header-rows: 1
   :widths: 30 70

   * - Issue
     - Quick Solution
   * - Compression not working
     - Known issue: Use KOPIA_MANUAL_CONFIG in repository secret instead of compression field
   * - No snapshots found
     - Check requestedIdentity matches source; use availableIdentities to see what's in repository
   * - repositoryPVC in ReplicationDestination
     - Not supported - repositoryPVC only works with ReplicationSource
   * - External policy files not loading
     - Not implemented - use inline configuration (retain, actions) instead
   * - enableFileDeletion vs enable_file_deletion
     - Use camelCase: ``enableFileDeletion`` (not snake_case)
   * - Partial identity error
     - Provide both username AND hostname, or use sourceIdentity, or omit both
   * - S3 endpoint not working
     - Both AWS_S3_ENDPOINT and KOPIA_S3_ENDPOINT are supported - check which you're using
   * - Read-only filesystem error
     - Update VolSync - fix mounts data at /restore/data for destinations
   * - Retention not working
     - Check maintenance is running; policies only apply during maintenance
   * - Wrong data restored
     - Verify requestedIdentity; check if source used custom username/hostname
   * - Cache PVC filling up with logs
     - Configure logging via KOPIA_FILE_LOG_LEVEL, KOPIA_LOG_DIR_MAX_FILES, KOPIA_LOG_DIR_MAX_AGE in repository secret

Understanding Enhanced Error Reporting
======================================

VolSync provides detailed error reporting when restore operations encounter issues.
The enhanced error reporting system automatically provides diagnostic information to
help you quickly identify and resolve problems.

Key Status Fields
-----------------

When troubleshooting restore operations, these status fields provide critical information:

**requestedIdentity**
   Shows the exact username@hostname that VolSync is attempting to restore from.
   This helps verify that the identity resolution is working as expected.

**snapshotsFound**
   Indicates the number of snapshots found for the requested identity.
   A value of 0 indicates no matching snapshots were found.

**availableIdentities**
   Lists all identities available in the repository with their snapshot counts
   and latest snapshot timestamps. This is particularly helpful when snapshots
   aren't found for the requested identity.

Checking Status Information
----------------------------

To view the complete status of a ReplicationDestination:

.. code-block:: bash

   # View full status
   kubectl get replicationdestination <name> -o yaml

   # Check specific status fields
   kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
   kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.snapshotsFound}'
   
   # View available identities
   kubectl get replicationdestination <name> -o json | jq '.status.kopia.availableIdentities'

Example Status Output
---------------------

When a restore operation cannot find snapshots, the status provides comprehensive information:

.. code-block:: yaml

   status:
     conditions:
     - type: Synchronizing
       status: "False"
       reason: SnapshotsNotFound
       message: "No snapshots found for identity 'webapp-backup@production-webapp-data'. Available identities in repository: database-backup@production-postgres-data (30 snapshots, latest: 2024-01-20T11:00:00Z), app-backup@staging-app-data (7 snapshots, latest: 2024-01-19T22:00:00Z)"
     kopia:
       requestedIdentity: "webapp-backup@production-webapp-data"
       snapshotsFound: 0
       availableIdentities:
       - identity: "database-backup@production-postgres-data"
         snapshotCount: 30
         latestSnapshot: "2024-01-20T11:00:00Z"
       - identity: "app-backup@staging-app-data"
         snapshotCount: 7
         latestSnapshot: "2024-01-19T22:00:00Z"

Common Error Scenarios and Solutions
=====================================

Partial Identity Configuration Error
-------------------------------------

**Error Message**: "missing 'hostname' - either provide both 'username' and 'hostname', or omit both"

**Cause**: You've provided only username without hostname (or vice versa). When using explicit 
identity, both fields must be provided together.

**Resolution**:

1. **Use automatic identity (simplest)** - Remove partial configuration:

   .. code-block:: yaml

      spec:
        kopia:
          destinationPVC: restored-data
          # No identity fields - uses automatic identity:
          # username: <destination-name>
          # hostname: <namespace>

2. **Use sourceIdentity (only needed for cross-namespace or different names)**:

   .. code-block:: yaml

      spec:
        kopia:
          # ⚠️ sourceIdentity only REQUIRED when:
          # - Cross-namespace restore (different namespaces)
          # - Destination name ≠ source ReplicationSource name
          sourceIdentity:
            sourceName: my-backup        # Name of the ReplicationSource
            sourceNamespace: production  # Namespace of the source
            # sourcePVCName is auto-discovered if not provided

3. **Provide both username AND hostname**:

   .. code-block:: yaml

      spec:
        kopia:
          username: "my-backup-production"
          hostname: "production"
          # Both fields are required together

**Common Mistakes**:

- Providing only ``username`` without ``hostname`` (or vice versa)
- Mixing sourceIdentity with explicit username/hostname fields

**Verification**:

Check that identity is properly configured:

.. code-block:: bash

   # Check the requested identity
   kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
   
   # Verify available identities in repository
   kubectl get replicationdestination <name> -o json | jq '.status.kopia.availableIdentities'

Filesystem Repository Issues
-----------------------------

**PVC Not Found**

**Error Message**: "PersistentVolumeClaim '<name>' not found"

**Resolution**:

1. Verify the PVC specified in ``repositoryPVC`` exists in the correct namespace:

   .. code-block:: bash

      kubectl get pvc -n <namespace>

2. Create the PVC if missing:

   .. code-block:: bash

      kubectl apply -f backup-pvc.yaml -n <namespace>

**PVC Not Bound**

**Error Message**: "PVC <name> is not bound"

**Resolution**:

1. Check PVC status:

   .. code-block:: bash

      kubectl describe pvc <name> -n <namespace>

2. Verify available PersistentVolumes:

   .. code-block:: bash

      kubectl get pv

3. Check for StorageClass issues if using dynamic provisioning

**Repository Initialization Failed**

**Error Message**: "unable to initialize repository at /kopia/repository"

**Resolution**:

1. Verify the PVC has sufficient space:

   .. code-block:: bash

      kubectl exec -it <kopia-pod> -n <namespace> -- df -h /kopia

2. Check the repository password is properly configured:

   .. code-block:: bash

      kubectl get secret <secret-name> -n <namespace> -o jsonpath='{.data.KOPIA_PASSWORD}' | base64 -d

3. Ensure the PVC supports write operations

**Filesystem URL Configuration**

**Note**: When using ``repositoryPVC``, VolSync automatically sets ``KOPIA_REPOSITORY=filesystem:///kopia/repository``. You don't need to configure this manually in the secret.
3. Check for directory traversal attempts (../)

**Permission Denied**

**Error Message**: "unable to create repository: permission denied"

**Resolution**:

1. Verify PVC is mounted with write permissions:

   .. code-block:: yaml

      filesystemDestination:
        claimName: backup-pvc
        readOnly: false  # Must be false for write access

2. Check pod security context if using privileged movers
3. Verify storage supports required operations

**Insufficient Storage**

**Error Message**: "no space left on device"

**Resolution**:

1. Check PVC usage:

   .. code-block:: bash

      kubectl exec -it <kopia-pod> -n <namespace> -- df -h /kopia

2. Expand PVC if supported:

   .. code-block:: bash

      kubectl patch pvc <name> -n <namespace> -p '{"spec":{"resources":{"requests":{"storage":"200Gi"}}}}'

3. Clean up old snapshots using retention policies

No Snapshots Found
------------------

**Error Message**: "No snapshots found for identity '<username>@<hostname>'"

**Symptoms**:

- ``snapshotsFound`` shows 0
- Restore operation fails
- ``availableIdentities`` shows other identities but not the requested one

**Resolution Steps**:

1. **Check available identities**
   
   Review what's actually in the repository:
   
   .. code-block:: bash
   
      kubectl get replicationdestination <name> -o yaml | grep -A 50 availableIdentities
   
2. **Verify source configuration**
   
   Check the ReplicationSource that created the backups:
   
   .. code-block:: bash
   
      # Find the source
      kubectl get replicationsource -A | grep <source-name>
      
      # Check its configuration
      kubectl get replicationsource <source-name> -n <namespace> -o yaml | grep -A 10 "kopia:"
   
3. **Common causes and fixes**:

   **Incorrect sourceIdentity (only needed for cross-namespace or different names)**:
   
   .. code-block:: yaml
   
      # ⚠️ Only use sourceIdentity when necessary:
      # - Cross-namespace restore: target namespace ≠ source namespace  
      # - Different names: destination name ≠ source ReplicationSource name
      sourceIdentity:
        sourceName: webapp-backup     # Verify this matches exactly
        sourceNamespace: production    # Verify this matches exactly
        # sourcePVCName: optional - auto-discovered if not provided
   
   **Source uses custom username/hostname**:
   
   If the ReplicationSource has custom identity fields, you must use them directly 
   (sourceIdentity won't work with custom source identity):
   
   .. code-block:: yaml
   
      # ⚠️ When source used custom identity, must use explicit identity:
      username: "custom-user"    # Must match source's custom username exactly
      hostname: "custom-host"    # Must match source's custom hostname exactly
   
   **No backups have been created yet**:
   
   Check if the ReplicationSource has successfully created any snapshots:
   
   .. code-block:: bash
   
      kubectl get replicationsource <name> -o jsonpath='{.status.lastManualSync}'

sourceIdentity Auto-Discovery Issues
-------------------------------------

**Error**: "Failed to fetch ReplicationSource for auto-discovery"

**Symptoms**:

- sourceIdentity specified without sourcePVCName or sourcePathOverride
- Auto-discovery fails to fetch the ReplicationSource

**Common Causes**:

1. **ReplicationSource doesn't exist**:
   
   Verify the source exists:
   
   .. code-block:: bash
   
      kubectl get replicationsource <sourceName> -n <sourceNamespace>
   
2. **Incorrect sourceName or sourceNamespace**:
   
   Double-check the spelling and namespace:
   
   .. code-block:: yaml
   
      sourceIdentity:
        sourceName: webapp-backup  # Must match exactly
        sourceNamespace: production  # Must match exactly
   
3. **Permission issues**:
   
   The operator may not have permission to read ReplicationSources in the target namespace.
   
4. **ReplicationSource has no sourcePVC**:
   
   Check if the source has a PVC defined:
   
   .. code-block:: bash
   
      kubectl get replicationsource <name> -n <namespace> -o jsonpath='{.spec.sourcePVC}'

**Resolution**:

Either fix the underlying issue or specify the values explicitly:

.. code-block:: yaml

   sourceIdentity:
     sourceName: webapp-backup
     sourceNamespace: production
     sourcePVCName: webapp-data        # Bypass PVC auto-discovery
     sourcePathOverride: "/app/data"   # Bypass path override auto-discovery

Identity Mismatch Issues
------------------------

**Error**: Restored data is from the wrong source

**Symptoms**:

- Data restored successfully but from unexpected source
- ``requestedIdentity`` doesn't match expectations

**Debugging Process**:

1. **Verify the requested identity**:
   
   .. code-block:: bash
   
      kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
   
2. **Compare with source identity**:
   
   Check what identity the ReplicationSource is using:
   
   .. code-block:: bash
   
      # Check source status
      kubectl get replicationsource <source-name> -o yaml | grep -A 5 "status:"
   
3. **Resolution**:
   
   Ensure identity configuration matches between source and destination:
   
   .. code-block:: yaml
   
      # Option 1: Use sourceIdentity for automatic matching
      spec:
        kopia:
          sourceIdentity:
            sourceName: <exact-source-name>
            sourceNamespace: <exact-source-namespace>
            # sourcePVCName: <optional - auto-discovered if omitted>
      
      # Option 2: Use explicit identity if source has custom values
      spec:
        kopia:
          username: <exact-username-from-source>
          hostname: <exact-hostname-from-source>

sourcePathOverride Issues
--------------------------

**Error**: "No snapshots found" with correct identity but path override mismatch

**Symptoms**:

- Identity (username@hostname) matches between source and destination
- ``snapshotsFound`` shows 0 despite having backups  
- ``requestedIdentity`` appears correct

**Common Causes**:

1. **Source used sourcePathOverride but destination doesn't**:

   The ReplicationSource created snapshots with a path override, but the restore 
   operation isn't using the same path override.

   **Debugging**:

   Check if the source used a path override:

   .. code-block:: bash

      kubectl get replicationsource <source-name> -n <namespace> -o jsonpath='{.spec.kopia.sourcePathOverride}'

   **Resolution**:

   If the source used a path override, ensure the destination uses the same value:

   .. code-block:: yaml

      # Option 1: Use sourceIdentity auto-discovery (recommended)
      sourceIdentity:
        sourceName: <source-name>
        sourceNamespace: <source-namespace>
        # sourcePathOverride will be auto-discovered

      # Option 2: Specify explicitly  
      sourceIdentity:
        sourceName: <source-name>
        sourceNamespace: <source-namespace>
        sourcePathOverride: "/path/from/source"

2. **Incorrect sourcePathOverride value**:

   The destination specifies a different path override than the source used.

   **Resolution**:

   .. code-block:: yaml

      sourceIdentity:
        sourceName: webapp-backup
        sourceNamespace: production
        # Remove explicit sourcePathOverride to use auto-discovery
        # sourcePathOverride: "/wrong/path"  # Remove this line

3. **Auto-discovery failed to find sourcePathOverride**:

   The ReplicationSource exists but auto-discovery couldn't fetch the path override.

   **Debugging**:

   Check the ReplicationDestination status for discovery information:

   .. code-block:: bash

      kubectl get replicationdestination <name> -o yaml | grep -A 10 "status:"

   **Resolution**:

   Specify the path override explicitly:

   .. code-block:: yaml

      sourceIdentity:
        sourceName: webapp-backup
        sourceNamespace: production
        sourcePathOverride: "/var/lib/myapp/data"  # Specify explicitly

**Error**: "Data restored to wrong path" or "Application can't find data"

**Symptoms**:

- Restore completes successfully
- Data exists in the destination PVC but at unexpected location
- Application can't access the restored data

**Common Causes**:

1. **Missing sourcePathOverride during restore**:

   The source used a path override, but the restore didn't apply the same override.

   **Resolution**:

   Ensure the restore uses the same path override:

   .. code-block:: yaml

      sourceIdentity:
        sourceName: database-backup
        sourceNamespace: production
        # This will auto-discover the correct sourcePathOverride

2. **Incorrect path override during restore**:

   The restore used a different path override than the source.

   **Verification**:

   Compare the source and destination configurations:

   .. code-block:: bash

      # Check source path override
      kubectl get replicationsource <source> -o jsonpath='{.spec.kopia.sourcePathOverride}'

      # Check what the destination used (from logs)
      kubectl logs -l volsync.backube/mover-job -n <namespace> | grep "source path override"

**Error**: "Auto-discovery found unexpected sourcePathOverride"

**Symptoms**:

- Restore uses a different path than expected
- Logs show auto-discovered path override that doesn't match expectations

**Resolution**:

Override auto-discovery by specifying the path explicitly:

.. code-block:: yaml

   sourceIdentity:
     sourceName: webapp-backup
     sourceNamespace: production
     # Override auto-discovery with the desired path
     sourcePathOverride: "/custom/restore/path"

**Best Practices for sourcePathOverride**

1. **Use auto-discovery when possible**:

   .. code-block:: yaml

      # Recommended: Let VolSync auto-discover the path override
      sourceIdentity:
        sourceName: webapp-backup
        sourceNamespace: production
        # No sourcePathOverride - will be auto-discovered

2. **Document path overrides**:

   Maintain documentation of which ReplicationSources use path overrides and why.

3. **Verify path overrides match**:

   Before creating restores, check the source configuration:

   .. code-block:: bash

      # Check if source uses path override
      kubectl get replicationsource <source> -o yaml | grep sourcePathOverride

4. **Test restore paths**:

   Verify that restored data appears at the expected location:

   .. code-block:: bash

      # After restore, check data location
      kubectl exec -it <test-pod> -- ls -la /expected/path/

Repository Connection Issues
----------------------------

**Error**: "Failed to connect to repository"

**Common Causes**:

1. **Incorrect repository secret**:
   
   Verify the secret exists and contains correct values:
   
   .. code-block:: bash
   
      kubectl get secret kopia-config -o yaml
   
2. **Network connectivity**:
   
   Check if the repository endpoint is reachable from the cluster.
   
3. **Authentication failures**:
   
   Verify credentials in the repository secret are valid.

**Resolution**:

.. code-block:: yaml

   # Ensure repository secret is correctly configured
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   stringData:
     KOPIA_REPOSITORY: <correct-repository-url>
     KOPIA_PASSWORD: <correct-password>
     # Additional credentials as needed

Multi-Tenant Repository Troubleshooting
========================================

Listing All Available Identities
---------------------------------

When working with multi-tenant repositories, use the ``availableIdentities`` status
field to understand what's in the repository:

.. code-block:: bash

   # Create a temporary ReplicationDestination to discover identities
   cat <<EOF | kubectl apply -f -
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: identity-discovery
     namespace: default
   spec:
     trigger:
       manual: discover
     kopia:
       repository: kopia-config
       destinationPVC: temp-pvc
       copyMethod: Direct
   EOF
   
   # Wait for status to populate
   sleep 10
   
   # List all identities
   kubectl get replicationdestination identity-discovery -o json | jq '.status.kopia.availableIdentities'
   
   # Clean up
   kubectl delete replicationdestination identity-discovery

Understanding Identity Format
-----------------------------

Identities in Kopia follow the format ``username@hostname``. VolSync generates these
based on specific, intentional design rules:

**Default Generation (no custom fields)**:

- Username: ReplicationSource/ReplicationDestination name (guaranteed unique within namespace)
- Hostname: ``<namespace>`` (ALWAYS just the namespace, never includes PVC name)

**With sourceIdentity**:

- Username: Derived from ``sourceName`` (the ReplicationSource object name)
- Hostname: ``<sourceNamespace>`` (ALWAYS just the namespace)
  - The ``sourcePVCName`` field (if provided) is used for reference but does NOT affect hostname
  - This is intentional - hostname is always namespace-only for consistency

**With explicit username/hostname**:

- Uses the exact values provided

Debugging Identity Generation
-----------------------------

To understand how identities are being generated:

1. **Check ReplicationSource configuration**:
   
   .. code-block:: bash
   
      kubectl get replicationsource <name> -o yaml | grep -E "(username|hostname|sourcePVC)"
   
2. **Verify ReplicationDestination resolution**:
   
   .. code-block:: bash
   
      kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
   
3. **Common identity patterns**:
   
   .. code-block:: text
   
      # Default pattern (namespace-only hostname)
      myapp-backup@production
      database-backup@production
      webapp-backup@staging
      
      # Multiple sources in same namespace (multi-tenancy)
      app1-backup@production  # Same hostname
      app2-backup@production  # Same hostname
      db-backup@production    # Same hostname - all unique identities
      
      # With custom username
      custom-user@production
      
      # With custom hostname
      myapp-backup@custom-host
      
      # Fully custom
      custom-user@custom-host

Advanced Debugging Techniques
==============================

Examining Pod Logs
------------------

When errors occur, check the mover pod logs for detailed information:

.. code-block:: bash

   # Find the mover pod
   kubectl get pods -l "volsync.backube/mover-job" -n <namespace>
   
   # View logs
   kubectl logs <pod-name> -n <namespace>
   
   # Follow logs in real-time
   kubectl logs -f <pod-name> -n <namespace>

Common Log Messages
-------------------

**"No snapshots found matching criteria"**:

Indicates the identity exists but no snapshots match the restore criteria
(e.g., restoreAsOf timestamp).

**"Unable to find snapshot source"**:

The specified username@hostname doesn't exist in the repository.

Troubleshooting enableFileDeletion
===================================

The ``enableFileDeletion`` feature cleans the destination directory before restore to ensure 
exact snapshot matching. Here's how to troubleshoot common issues:

Verifying File Deletion is Enabled
-----------------------------------

Check if the feature is properly configured:

.. code-block:: bash

   # Check the spec configuration
   kubectl get replicationdestination <name> -o jsonpath='{.spec.kopia.enableFileDeletion}'
   
   # Verify the environment variable is set in the mover pod
   kubectl describe pod <mover-pod> | grep KOPIA_ENABLE_FILE_DELETION
   
   # Check mover logs for cleaning activity
   kubectl logs <mover-pod> | grep -E "(File deletion|Cleaning destination)"

Expected log output when enabled:

.. code-block:: text

   File deletion enabled - cleaning destination directory before restore
   Cleaning destination directory: /data
   Destination directory cleaned (preserved lost+found if present)

Files Not Being Deleted
------------------------

**Symptoms**: Extra files remain after restore despite ``enableFileDeletion: true``

**Possible Causes**:

1. **Configuration not applied**: Check YAML indentation
   
   .. code-block:: yaml
   
      # Correct indentation
      spec:
        kopia:
          enableFileDeletion: true
   
2. **Old VolSync version**: Ensure you're using a version that supports this feature
   
   .. code-block:: bash
   
      kubectl get deployment volsync -n volsync-system -o jsonpath='{.spec.template.spec.containers[0].image}'
   
3. **Permission issues**: Mover pod lacks permissions to delete files
   
   .. code-block:: bash
   
      # Check file permissions in the destination
      kubectl exec <pod-using-pvc> -- ls -la /mount/point
      
      # Check security context of mover pod
      kubectl get pod <mover-pod> -o jsonpath='{.spec.securityContext}'

Restore Fails During Cleaning
------------------------------

**Error**: "Permission denied" or "Operation not permitted" during cleaning

**Solutions**:

1. Check for immutable files:
   
   .. code-block:: bash
   
      kubectl exec <pod-using-pvc> -- lsattr /mount/point 2>/dev/null || echo "lsattr not available"
   
2. Verify volume mount permissions:
   
   .. code-block:: bash
   
      kubectl get pvc <pvc-name> -o yaml | grep -A5 "accessModes"
   
3. Check if volume is read-only:
   
   .. code-block:: bash
   
      kubectl describe pod <mover-pod> | grep -A5 "Mounts:"

Performance Impact
------------------

Large directories with many files may take time to clean. Monitor the cleaning phase:

.. code-block:: bash

   # Watch mover pod logs in real-time
   kubectl logs -f <mover-pod>
   
   # Check how many files are being deleted
   kubectl exec <pod-using-pvc> -- find /mount/point -type f | wc -l

Best Practices for Debugging
-----------------------------

1. **Test in non-production first**: Always verify behavior in a test environment
   
2. **Create a backup before enabling**: If unsure about existing data
   
   .. code-block:: bash
   
      # Create a snapshot of the PVC before enabling file deletion
      kubectl apply -f - <<EOF
      apiVersion: snapshot.storage.k8s.io/v1
      kind: VolumeSnapshot
      metadata:
        name: backup-before-deletion
      spec:
        source:
          persistentVolumeClaimName: <destination-pvc>
      EOF
   
3. **Monitor the first restore carefully**: Check logs and verify results
   
4. **Document what's being deleted**: List files before enabling for production
   
   .. code-block:: bash
   
      # List files that would be deleted (excluding lost+found)
      kubectl exec <pod-using-pvc> -- find /mount/point -mindepth 1 -maxdepth 1 ! -name 'lost+found'

**"Repository not initialized"**:

The repository hasn't been created yet or connection details are incorrect.

Using Previous Parameter with Discovery
----------------------------------------

When using the ``previous`` parameter, the discovery features help verify
snapshot availability:

.. code-block:: yaml

   spec:
     kopia:
       sourceIdentity:
         sourceName: myapp-backup
         sourceNamespace: production
         # sourcePVCName: auto-discovered from ReplicationSource
       previous: 2  # Skip 2 snapshots
   
   status:
     kopia:
       requestedIdentity: "myapp-backup@production-myapp-data"
       snapshotsFound: 5  # Total snapshots available
       # With previous: 2, will use the 3rd newest snapshot

If ``snapshotsFound`` is less than or equal to ``previous``, the restore will fail:

.. code-block:: yaml

   status:
     conditions:
     - type: Synchronizing
       status: "False"
       reason: InsufficientSnapshots
       message: "Requested snapshot index 2 but only 1 snapshots found for identity 'myapp-backup@production-myapp-data'"

Best Practices for Troubleshooting
===================================

Preventive Measures
--------------------

1. **Document identity configuration**:
   
   Maintain documentation of custom username/hostname configurations used in
   ReplicationSources.
   
2. **Test restore procedures regularly**:
   
   Periodically test restore operations in non-production environments.
   
3. **Monitor backup success**:
   
   Set up alerts for failed backup operations to ensure snapshots are being created.
   
4. **Use consistent naming**:
   
   Maintain consistent ReplicationSource names across environments.

Systematic Debugging Approach
------------------------------

When encountering issues, follow this systematic approach:

1. **Check status fields**:
   
   Start with ``requestedIdentity``, ``snapshotsFound``, and ``availableIdentities``.
   
2. **Verify configuration**:
   
   Ensure ReplicationSource and ReplicationDestination configurations match.
   
3. **Review logs**:
   
   Check mover pod logs for detailed error messages.
   
4. **Test connectivity**:
   
   Verify repository is accessible and credentials are valid.
   
5. **Validate data**:
   
   Ensure backups have been successfully created before attempting restore.

Quick Reference Commands
========================

.. code-block:: bash

   # List all ReplicationSources
   kubectl get replicationsource -A
   
   # Check ReplicationDestination status
   kubectl describe replicationdestination <name>
   
   # View available identities
   kubectl get replicationdestination <name> -o json | jq '.status.kopia.availableIdentities'
   
   # Check requested identity
   kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
   
   # View snapshot count
   kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.snapshotsFound}'
   
   # Find mover pods
   kubectl get pods -l "volsync.backube/mover-job"
   
   # View mover logs
   kubectl logs -l "volsync.backube/mover-job" --tail=100

Read-Only Root Filesystem Error
================================

**Error**: "unlinkat //data.kopia-entry: read-only file system"

**Symptoms**:
- Restore operations fail when using ``readOnlyRootFilesystem: true`` security setting
- Error occurs during ``kopia snapshot restore`` command execution
- Affects pods with restricted security contexts

**Cause**:

Kopia uses atomic file operations that create temporary files (`.kopia-entry`) during restore operations. When the root filesystem is read-only and data is mounted at `/data`, Kopia attempts to create these temporary files at `/data.kopia-entry`, which fails because the root directory (`/`) is read-only.

**Resolution**:

This issue has been fixed in recent versions of VolSync. The fix involves:

1. **For destination (restore) operations**: Data is now mounted at `/restore/data` instead of `/data`
2. **Additional volume**: An emptyDir volume is mounted at `/restore` to provide a writable directory for Kopia's temporary files
3. **Result**: Kopia can now create its temporary `.kopia-entry` files at `/restore/data.kopia-entry` within the writable `/restore` directory

**Note**: This change only affects destination (restore) operations. Source (backup) operations continue to use the `/data` mount path and are not affected by this issue.

**Verification**:

To verify you have the fix:

1. Check your VolSync version - ensure you're using a version that includes this fix
2. During restore operations, the mover pod should have:
   - Data volume mounted at `/restore/data`
   - An emptyDir volume mounted at `/restore`

If you're still experiencing this issue, ensure your VolSync deployment is up to date.

Repository Policy Troubleshooting
==================================

Troubleshooting issues related to repository policies, retention, compression, and actions.

Retention Policy Not Working
-----------------------------

**Symptoms**:

- Old snapshots are not being removed
- Repository size keeps growing
- Retention settings seem to be ignored

**Common Causes and Solutions**:

1. **Maintenance Not Running**
   
   Retention policies are enforced during maintenance operations.
   
   .. code-block:: bash
   
      # Check when maintenance last ran
      kubectl get replicationsource <name> -o jsonpath='{.status.kopia.lastMaintenance}'
   
   **Solution**: Ensure ``maintenanceIntervalDays`` is set appropriately:
   
   .. code-block:: yaml
   
      spec:
        kopia:
          maintenanceIntervalDays: 7  # Run weekly

2. **Policy Not Applied**
   
   Check if the policy was successfully set:
   
   .. code-block:: bash
   
      # Check mover pod logs for policy application
      kubectl logs <mover-pod> | grep -i "policy\|retention"
   
   **Solution**: Verify retention configuration syntax:
   
   .. code-block:: yaml
   
      spec:
        kopia:
          retain:
            hourly: 24    # Must be integer
            daily: 7      # Not string
            weekly: 4
            monthly: 12
            yearly: 5

3. **Conflicting Policies**
   
   External policy files may override inline settings.
   
   .. code-block:: bash
   
      # Check if external policies are configured
      kubectl get replicationsource <name> -o jsonpath='{.spec.kopia.policyConfig}'
   
   **Solution**: Either use inline OR external policies, not both.

Compression Issues
------------------

**Problem**: Compression not reducing backup size as expected

**Known Implementation Issue**:

.. warning::
   The ``compression`` field in the ReplicationSource spec has a known implementation issue.
   While the KOPIA_COMPRESSION environment variable is set based on this field, it is not
   actually used by the Kopia shell script during repository creation or operations.
   This is a limitation in the current implementation.

**Diagnosis**:

.. code-block:: bash

   # Check if compression is configured
   kubectl get replicationsource <name> -o jsonpath='{.spec.kopia.compression}'
   
   # Check mover logs for compression settings
   kubectl logs <mover-pod> | grep -i compression
   
   # Check if KOPIA_COMPRESSION is set (it will be, but not used)
   kubectl describe pod <mover-pod> | grep KOPIA_COMPRESSION

**Important Notes**:

- The ``compression`` field sets the KOPIA_COMPRESSION environment variable
- However, this environment variable is **not used** by the shell script
- Compression is set at **repository creation time only** and cannot be changed
- To use different compression, you must create a new repository
- Not all data compresses well (already compressed files, encrypted data)

**Current Workarounds**:

1. **Use KOPIA_MANUAL_CONFIG for compression** (Most Reliable):
   
   Add a KOPIA_MANUAL_CONFIG entry to your repository secret with compression settings:
   
   .. code-block:: yaml
   
      apiVersion: v1
      kind: Secret
      metadata:
        name: kopia-config
      stringData:
        KOPIA_REPOSITORY: s3://my-bucket/backups
        KOPIA_PASSWORD: my-password
        # Use manual config to set compression
        KOPIA_MANUAL_CONFIG: |
          {
            "compression": {
              "compressor": "zstd"
            }
          }

2. **Wait for fix**: This is a known issue that may be addressed in future releases

3. **For existing repositories**: You cannot change compression after creation:
   - Create a new repository with desired compression settings
   - Migrate data to the new repository

Actions Not Executing
---------------------

**Problem**: Before/after snapshot actions are not running

**Diagnosis**:

.. code-block:: bash

   # Check if actions are configured
   kubectl get replicationsource <name> -o yaml | grep -A5 actions
   
   # Check mover pod logs for action execution
   kubectl logs <mover-pod> | grep -i "action\|hook\|before\|after"

**Common Issues**:

1. **Actions Not Enabled in Repository**
   
   When using external policy files, ensure actions are enabled:
   
   .. code-block:: yaml
   
      # In repository.config
      {
        "enableActions": true,
        "permittedActions": [
          "beforeSnapshotRoot",
          "afterSnapshotRoot"
        ]
      }

2. **Command Not Found**
   
   Actions run in the mover container context:
   
   .. code-block:: yaml
   
      actions:
        # Bad: assumes mysql client in mover container
        beforeSnapshot: "mysql -e 'FLUSH TABLES'"
        
        # Good: uses commands available in container
        beforeSnapshot: "sync"  # Flush filesystem buffers

3. **Permission Issues**
   
   Actions run with mover pod permissions:
   
   .. code-block:: bash
   
      # Check mover pod security context
      kubectl get pod <mover-pod> -o jsonpath='{.spec.securityContext}'

Policy Configuration Not Loading
---------------------------------

**Problem**: External policy files not being applied

**Diagnosis**:

.. code-block:: bash

   # Check if policy configuration is specified
   kubectl get replicationsource <name> -o jsonpath='{.spec.kopia.policyConfig}'
   
   # Verify ConfigMap/Secret exists
   kubectl get configmap <policy-config-name> -n <namespace>
   kubectl get secret <policy-secret-name> -n <namespace>
   
   # Check mover pod logs for policy application
   kubectl logs <mover-pod> | grep -i "policy.*config"

**Common Solutions**:

1. **Use inline configuration for simple policies**:

   .. code-block:: yaml

      spec:
        kopia:
          retain:
            daily: 7
            weekly: 4
          compression: "zstd"  # Now works reliably
          actions:
            beforeSnapshot: "sync"

2. **For complex policies, use external policy files**:

   .. code-block:: yaml

      spec:
        kopia:
          policyConfig:
            configMapName: kopia-policies
            # Ensure JSON files are valid and properly formatted

**Note on Policy Configuration**:

Both inline and external policy configuration methods are supported:

**Inline configuration** (for simple policies):
- ``retain``: Retention policies (applied during maintenance)
- ``compression``: Compression algorithm (works reliably)
- ``actions``: Before/after snapshot commands
- ``parallelism``: Number of parallel upload streams

**External policy files** (for complex policies):
- Global policy files via ConfigMap/Secret
- Repository configuration files
- JSON validation and 1MB size limits
- Support for advanced Kopia features

Verifying Policy Application
-----------------------------

To verify policies are correctly applied:

1. **Check Mover Pod Logs**:
   
   .. code-block:: bash
   
      # Look for policy-related messages
      kubectl logs <mover-pod> | grep -E "policy|retention|compression|action"

2. **Direct Repository Inspection** (if accessible):
   
   .. code-block:: bash
   
      # Connect to repository and check policies
      kopia repository connect <repository-params>
      kopia policy show --global
      kopia policy show <path>

3. **Monitor Maintenance Operations**:
   
   .. code-block:: bash
   
      # Watch for maintenance runs
      kubectl get replicationsource <name> -w -o jsonpath='{.status.kopia.lastMaintenance}'

Best Practices for Policy Configuration
----------------------------------------

1. **Start Simple**: Begin with inline configuration, move to external files only when needed
2. **Test Policies**: Verify policies work in test environment before production
3. **Monitor Results**: Check that retention is working as expected
4. **Document Changes**: Keep track of policy modifications and reasons
5. **Regular Audits**: Periodically verify policies are still appropriate

Debugging with KOPIA_MANUAL_CONFIG
-----------------------------------

When features aren't working as expected through the standard configuration fields,
check if KOPIA_MANUAL_CONFIG can be used as a workaround:

**Checking Current Configuration:**

.. code-block:: bash

   # Check if KOPIA_MANUAL_CONFIG is set in the repository secret
   kubectl get secret kopia-config -o jsonpath='{.data.KOPIA_MANUAL_CONFIG}' | base64 -d
   
   # Check environment variables in the mover pod
   kubectl describe pod <mover-pod> | grep -A20 "Environment:"
   
   # Check mover logs for manual config usage
   kubectl logs <mover-pod> | grep -i "manual\|config"

**Using KOPIA_MANUAL_CONFIG for Workarounds:**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   stringData:
     KOPIA_REPOSITORY: s3://my-bucket/backups
     KOPIA_PASSWORD: my-password
     # Use manual config for features with implementation issues
     KOPIA_MANUAL_CONFIG: |
       {
         "compression": {
           "compressor": "zstd",
           "min-size": 1000
         },
         "splitter": {
           "algorithm": "DYNAMIC-4M-BUZHASH",
           "min-size": "1MB",
           "max-size": "4MB"
         },
         "actions": {
           "before-snapshot-root": "/scripts/pre-backup.sh",
           "after-snapshot-root": "/scripts/post-backup.sh"
         }
       }

**Common KOPIA_MANUAL_CONFIG Use Cases:**

1. **Setting compression** (workaround for compression field issue)
2. **Advanced splitter configuration** (not exposed in VolSync)
3. **Custom encryption settings** (beyond basic password)
4. **Advanced caching parameters** (fine-tuning performance)
5. **Repository-specific overrides** (special requirements)

.. warning::
   KOPIA_MANUAL_CONFIG is a low-level configuration option. Use with caution and
   test thoroughly before applying to production. Some settings may conflict with
   VolSync's automatic configuration.

Kopia Logging Configuration
============================

VolSync provides environment variables to control Kopia's file logging behavior, preventing the cache PVC from filling up with excessive logs. This is particularly important in Kubernetes environments where users typically rely on external logging solutions (Loki, ElasticSearch, Splunk, etc.) rather than file-based logs.

The Problem: Cache PVC Filling Up
----------------------------------

**Issue**: Kopia's default logging configuration can generate large amounts of log files that accumulate in the cache PVC, eventually filling it up and causing backup failures.

**Root Cause**: 

- Kopia creates detailed file logs by default at debug level
- Logs are stored in the cache directory (typically ``/kopia/cache/logs``)
- Default retention keeps logs indefinitely or for long periods
- In Kubernetes, these logs duplicate what's already captured by pod logs

**Impact**:

- Cache PVCs fill up over time, especially with frequent backups
- Backup and restore operations fail when the PVC is full
- Manual intervention required to clean up logs
- Wasted storage on redundant logging

Logging Configuration Environment Variables
--------------------------------------------

VolSync exposes Kopia's native logging controls through environment variables that can be set in your repository secret. The defaults are optimized for Kubernetes environments:

.. list-table:: Kopia Logging Environment Variables
   :header-rows: 1
   :widths: 30 20 50

   * - Variable
     - Default
     - Description
   * - ``KOPIA_FILE_LOG_LEVEL``
     - ``info``
     - Log level for file logs (debug, info, warn, error). Provides good operational visibility without excessive verbosity
   * - ``KOPIA_LOG_DIR_MAX_FILES``
     - ``3``
     - Maximum number of CLI log files to retain. Optimized for Kubernetes where logs are externally collected
   * - ``KOPIA_LOG_DIR_MAX_AGE``
     - ``4h``
     - Maximum age of CLI log files. Short retention since Kubernetes typically has external logging
   * - ``KOPIA_CONTENT_LOG_DIR_MAX_FILES``
     - ``3``
     - Maximum number of content log files to retain. Minimal retention for immediate debugging only
   * - ``KOPIA_CONTENT_LOG_DIR_MAX_AGE``
     - ``4h``
     - Maximum age of content log files. Short retention optimized for Kubernetes environments

**Default Configuration Rationale**:

The defaults are conservative to prevent cache PVC issues:

- **Info log level**: Balances useful information with manageable log size
- **10 files max**: Limits total log storage to a predictable amount
- **24 hour retention**: Provides recent history while ensuring regular cleanup
- **Optimized for Kubernetes**: Assumes pod logs are the primary logging mechanism

Configuring Logging in Your Repository Secret
----------------------------------------------

Override the default logging configuration by adding environment variables to your Kopia repository secret:

**Example: Production Configuration with Minimal Logging**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     # Repository configuration
     KOPIA_REPOSITORY: s3://my-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     
     # Minimal logging for production
     KOPIA_FILE_LOG_LEVEL: "error"      # Only log errors to files
     KOPIA_LOG_DIR_MAX_FILES: "5"       # Keep only 5 log files
     KOPIA_LOG_DIR_MAX_AGE: "6h"        # Retain for 6 hours only

**Example: Development Configuration with Verbose Logging**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config-dev
   type: Opaque
   stringData:
     # Repository configuration
     KOPIA_REPOSITORY: s3://dev-bucket/backups
     KOPIA_PASSWORD: dev-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     
     # Verbose logging for debugging
     KOPIA_FILE_LOG_LEVEL: "debug"      # Maximum verbosity
     KOPIA_LOG_DIR_MAX_FILES: "20"      # Keep more files for analysis
     KOPIA_LOG_DIR_MAX_AGE: "7d"        # Keep logs for a week

**Example: Disable File Logging Entirely**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config-no-logs
   type: Opaque
   stringData:
     # Repository configuration
     KOPIA_REPOSITORY: s3://my-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     
     # Effectively disable file logging
     KOPIA_FILE_LOG_LEVEL: "error"      # Only critical errors
     KOPIA_LOG_DIR_MAX_FILES: "1"       # Minimum possible
     KOPIA_LOG_DIR_MAX_AGE: "1h"        # Very short retention

Troubleshooting Logging Issues
-------------------------------

**Checking Current Log Usage**

To see how much space logs are using in your cache PVC:

.. code-block:: bash

   # Find the mover pod
   kubectl get pods -l "volsync.backube/mover-job" -n <namespace>
   
   # Check log directory size
   kubectl exec <mover-pod> -n <namespace> -- du -sh /kopia/cache/logs
   
   # List log files
   kubectl exec <mover-pod> -n <namespace> -- ls -lh /kopia/cache/logs

**Monitoring Log Rotation**

Verify that log rotation is working:

.. code-block:: bash

   # Check mover pod environment variables
   kubectl describe pod <mover-pod> -n <namespace> | grep -E "KOPIA_(FILE_)?LOG"
   
   # Watch log directory over time
   kubectl exec <mover-pod> -n <namespace> -- ls -lt /kopia/cache/logs | head -10

**Cleaning Up Existing Logs**

If your cache PVC is already full of old logs:

.. code-block:: bash

   # Option 1: Delete old logs manually
   kubectl exec <mover-pod> -n <namespace> -- find /kopia/cache/logs -type f -mtime +1 -delete
   
   # Option 2: Clear all logs (safe - they'll be recreated)
   kubectl exec <mover-pod> -n <namespace> -- rm -rf /kopia/cache/logs/*

**Debugging with Increased Logging**

When troubleshooting issues, temporarily increase logging:

.. code-block:: yaml

   # Temporarily update your secret for debugging
   stringData:
     KOPIA_FILE_LOG_LEVEL: "debug"      # Increase verbosity
     KOPIA_LOG_DIR_MAX_FILES: "20"      # Keep more files
     KOPIA_LOG_DIR_MAX_AGE: "48h"       # Keep for 2 days

.. warning::
   Remember to revert to production settings after debugging. Debug level logging
   can generate very large files (100MB+ per backup operation).

Best Practices for Logging in Kubernetes
-----------------------------------------

1. **Use External Logging Systems**: Rely on Kubernetes pod logs and external aggregation (Loki, ElasticSearch, Splunk) rather than file logs.

2. **Conservative Defaults**: The VolSync defaults (info level, 3 files, 4h retention) are optimized for Kubernetes environments where external logging is typically used.

3. **Monitor Cache PVC Usage**: Set up alerts for cache PVC usage to catch issues early:

   .. code-block:: yaml

      # Example Prometheus alert
      alert: KopiaCachePVCFull
      expr: |
        (kubelet_volume_stats_used_bytes / kubelet_volume_stats_capacity_bytes) 
        * on(persistentvolumeclaim) group_left()
        kube_persistentvolumeclaim_labels{label_app="volsync"} > 0.8
      annotations:
        summary: "Kopia cache PVC is >80% full"

4. **Size Cache PVCs Appropriately**: Account for both cache data and logs when sizing:
   
   - Minimum: 2Gi for light usage
   - Recommended: 5-10Gi for regular backups
   - Large datasets: 20Gi+ (scales with data size and change rate)

5. **Regular Maintenance**: Run Kopia maintenance to clean up cache and logs:

   .. code-block:: yaml

      spec:
        kopia:
          maintenanceIntervalDays: 7  # Weekly maintenance

Common Scenarios and Recommendations
-------------------------------------

**High-Frequency Backups (Hourly or more)**

.. code-block:: yaml

   stringData:
     KOPIA_FILE_LOG_LEVEL: "error"      # Minimize logging
     KOPIA_LOG_DIR_MAX_FILES: "5"       # Small rotation
     KOPIA_LOG_DIR_MAX_AGE: "6h"        # Short retention

**Large Datasets (100GB+)**

.. code-block:: yaml

   stringData:
     KOPIA_FILE_LOG_LEVEL: "warn"       # Balanced logging
     KOPIA_LOG_DIR_MAX_FILES: "10"      # Moderate rotation
     KOPIA_LOG_DIR_MAX_AGE: "12h"       # Half-day retention

**Development/Testing**

.. code-block:: yaml

   stringData:
     KOPIA_FILE_LOG_LEVEL: "info"       # Informative logging
     KOPIA_LOG_DIR_MAX_FILES: "20"      # Keep more history
     KOPIA_LOG_DIR_MAX_AGE: "3d"        # Several days retention

**Air-Gapped/Disconnected Environments**

.. code-block:: yaml

   stringData:
     KOPIA_FILE_LOG_LEVEL: "info"       # More logging since no external collection
     KOPIA_LOG_DIR_MAX_FILES: "30"      # Extended history
     KOPIA_LOG_DIR_MAX_AGE: "7d"        # Week of logs for troubleshooting

Migration Guide for Existing Deployments
-----------------------------------------

If you're experiencing cache PVC issues with existing deployments:

1. **Immediate Relief**: Clear existing logs

   .. code-block:: bash

      # Clean up old logs in running pods
      kubectl exec -it <mover-pod> -- rm -rf /kopia/cache/logs/*.log

2. **Apply New Configuration**: Update your repository secret

   .. code-block:: bash

      # Edit the secret
      kubectl edit secret kopia-config -n <namespace>
      
      # Add the logging configuration
      # KOPIA_FILE_LOG_LEVEL: "info"
      # KOPIA_LOG_DIR_MAX_FILES: "3"
      # KOPIA_LOG_DIR_MAX_AGE: "4h"

3. **Trigger New Backup**: Force a new backup to apply settings

   .. code-block:: bash

      # Trigger manual sync
      kubectl patch replicationsource <name> -n <namespace> \
        --type merge -p '{"spec":{"trigger":{"manual":"backup-now"}}}'

4. **Verify New Settings**: Check that rotation is working

   .. code-block:: bash

      # After backup completes, verify settings
      kubectl logs <new-mover-pod> | grep "Log Configuration"

Technical Details
-----------------

**Log Types in Kopia**:

1. **CLI Logs** (``KOPIA_LOG_DIR_*``): General operations, may contain file names and paths
2. **Content Logs** (``KOPIA_CONTENT_LOG_DIR_*``): Low-level storage operations, no sensitive data

**Log File Naming**:

- CLI logs: ``kopia-<timestamp>-<pid>.log``
- Content logs: ``kopia-content-<timestamp>-<pid>.log``

**Rotation Mechanism**:

- Kopia checks file count and age at startup
- Oldest files are deleted when limits are exceeded
- Rotation happens per-execution, not continuously

**Performance Impact**:

- ``debug`` level: Can slow operations by 10-20% due to I/O
- ``info`` level: Minimal impact (<5%)
- ``warn``/``error`` level: Negligible impact

Getting Help
============

If you continue to experience issues after following this troubleshooting guide:

1. Check the VolSync documentation for updates
2. Review the GitHub issues for similar problems
3. Enable debug logging for more detailed information
4. Contact support with the output from the diagnostic commands above