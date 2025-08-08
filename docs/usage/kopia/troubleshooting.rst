=======================
Troubleshooting Guide
=======================

.. contents:: Troubleshooting Kopia Operations
   :local:

This guide provides comprehensive troubleshooting information for Kopia-based backup
and restore operations in VolSync, with a focus on the enhanced error reporting and
snapshot discovery features.

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

   **Incorrect sourceIdentity**:
   
   .. code-block:: yaml
   
      # Wrong namespace or name
      sourceIdentity:
        sourceName: webapp-backup     # Verify this matches exactly
        sourceNamespace: production    # Verify this matches exactly
        # sourcePVCName: optional - auto-discovered if not provided
   
   **Source uses custom username/hostname**:
   
   If the ReplicationSource has custom identity fields, you must use them directly:
   
   .. code-block:: yaml
   
      # Instead of sourceIdentity, use:
      username: "custom-user"
      hostname: "custom-host"
   
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
based on specific rules:

**Default Generation (no custom fields)**:

- Username: ReplicationSource name
- Hostname: ``<namespace>-<pvc-name>``

**With sourceIdentity**:

- Username: ``sourceName`` from sourceIdentity
- Hostname: ``<sourceNamespace>-<sourcePVCName>``
  - If ``sourcePVCName`` is provided: uses that value
  - If not provided: auto-discovers from ReplicationSource's ``spec.sourcePVC``

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
   
      # Default pattern
      myapp-backup@production-myapp-data
      
      # With custom username
      custom-user@production-myapp-data
      
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

Getting Help
============

If you continue to experience issues after following this troubleshooting guide:

1. Check the VolSync documentation for updates
2. Review the GitHub issues for similar problems
3. Enable debug logging for more detailed information
4. Contact support with the output from the diagnostic commands above