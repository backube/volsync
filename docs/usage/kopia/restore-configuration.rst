======================
Restore Configuration
======================

.. contents:: Restore Operations and Configuration
   :local:

Data from a backup can be restored using the ReplicationDestination CR. In most
cases, it is desirable to perform a single restore into an empty
PersistentVolume.

Restoring from a Specific ReplicationSource (sourceIdentity)
------------------------------------------------------------

When restoring data from a Kopia repository that contains backups from multiple sources, 
you need to specify which source's snapshots to restore. The ``sourceIdentity`` field 
provides a simplified way to do this.

Understanding the Multi-Tenancy Challenge
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Kopia repositories support multiple clients backing up to the same repository. Each client 
is identified by a unique combination of username and hostname. When you want to restore 
data, you need to specify the exact identity to restore from.

Without ``sourceIdentity``, you would need to:

1. Determine the exact username and hostname used by the ReplicationSource
2. Manually configure these values in your ReplicationDestination
3. Handle the complexity of VolSync's automatic identity generation rules

The ``sourceIdentity`` field simplifies this by allowing you to specify just the 
ReplicationSource's name and namespace, and VolSync automatically generates the 
matching username and hostname.

Using sourceIdentity
~~~~~~~~~~~~~~~~~~~~

The ``sourceIdentity`` field accepts two parameters:

- ``sourceName``: The name of the ReplicationSource that created the backups
- ``sourceNamespace``: The namespace where the ReplicationSource exists

**Example: Basic sourceIdentity usage**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-webapp
     namespace: production
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       # Specify which ReplicationSource's snapshots to restore
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production

In this example, VolSync will automatically:

1. Generate the username based on the source name (``webapp-backup``)
2. Generate the hostname based on the namespace and PVC name from the source
3. Connect to the repository using these generated credentials
4. Restore from the snapshots created by that specific ReplicationSource

**Example: Cross-namespace restore**

You can restore snapshots created in a different namespace:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-from-staging
     namespace: production
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       # Restore staging data to production
       sourceIdentity:
         sourceName: app-backup
         sourceNamespace: staging

**Example: Combining sourceIdentity with previous parameter**

You can use ``sourceIdentity`` together with the ``previous`` parameter to restore 
older snapshots from a specific source:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-previous-version
     namespace: production
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       # Identify the source
       sourceIdentity:
         sourceName: database-backup
         sourceNamespace: production
       # Skip the latest snapshot, use the previous one
       previous: 1

Fallback to Manual Username/Hostname
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

If you need more control, you can still manually specify ``username`` and ``hostname`` 
fields. When these are explicitly set, they take precedence over ``sourceIdentity``:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-manual-identity
   spec:
     kopia:
       repository: kopia-config
       # Manual identity configuration (overrides sourceIdentity if both are present)
       username: "custom-user"
       hostname: "custom-host"
       # sourceIdentity would be ignored if present

Priority order for identity resolution:

1. **Explicit username/hostname**: If specified, these are used exactly as provided
2. **sourceIdentity**: If specified and no explicit username/hostname, generates them based on source
3. **Default generation**: If none specified, generates based on ReplicationDestination name and namespace

Enhanced Error Reporting and Discovery
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

VolSync provides comprehensive error reporting and snapshot discovery features to help 
you quickly identify and resolve restore issues. When a restore operation encounters 
problems, the system automatically provides detailed diagnostic information.

Status Fields for Debugging
^^^^^^^^^^^^^^^^^^^^^^^^^^^^

VolSync exposes several status fields that provide critical information about the 
restore operation and available snapshots:

**RequestedIdentity**

Shows the exact username@hostname that VolSync is attempting to restore from. This 
helps verify that identity resolution is working as expected:

.. code-block:: bash

   kubectl get replicationdestination restore-webapp -o jsonpath='{.status.kopia.requestedIdentity}'
   # Output: webapp-backup@production-webapp-data

**SnapshotsFound**

Indicates how many snapshots were found for the requested identity. A value of 0 
indicates that no matching snapshots were found:

.. code-block:: bash

   kubectl get replicationdestination restore-webapp -o jsonpath='{.status.kopia.snapshotsFound}'
   # Output: 15

**AvailableIdentities**

Lists all identities available in the repository with detailed information about each. 
This is particularly helpful when snapshots aren't found for the requested identity:

.. code-block:: bash

   # View available identities in formatted output
   kubectl get replicationdestination restore-webapp -o json | jq '.status.kopia.availableIdentities'
   
   # Alternative: using grep
   kubectl get replicationdestination restore-webapp -o yaml | grep -A 50 availableIdentities

Error Reporting Examples
^^^^^^^^^^^^^^^^^^^^^^^^^

When snapshots are not found, the enhanced error reporting provides clear feedback:

**Example: No snapshots found**

.. code-block:: yaml

   status:
     conditions:
     - type: Synchronizing
       status: "False"
       reason: SnapshotsNotFound
       message: "No snapshots found for identity 'webapp-backup@production-webapp-data'. 
                Available identities in repository: 
                database-backup@production-postgres-data (30 snapshots, latest: 2024-01-20T11:00:00Z), 
                app-backup@staging-app-data (7 snapshots, latest: 2024-01-19T22:00:00Z)"
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

**Example: Successful discovery**

.. code-block:: yaml

   status:
     conditions:
     - type: Synchronizing
       status: "True"
       reason: SynchronizingData
       message: "Restoring from snapshot 2024-01-20T10:30:00Z"
     kopia:
       requestedIdentity: "webapp-backup@production-webapp-data"
       snapshotsFound: 15
       availableIdentities:
       - identity: "webapp-backup@production-webapp-data"
         snapshotCount: 15
         latestSnapshot: "2024-01-20T10:30:00Z"
       - identity: "database-backup@production-postgres-data"
         snapshotCount: 30
         latestSnapshot: "2024-01-20T11:00:00Z"
       - identity: "app-backup@staging-app-data"
         snapshotCount: 7
         latestSnapshot: "2024-01-19T22:00:00Z"

Using Discovery Information for Debugging
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

The discovery features help you:

1. **Identify available backup sources**: See all identities in the repository
2. **Verify snapshot availability**: Check snapshot counts and latest timestamps
3. **Debug configuration issues**: Compare requested vs. available identities
4. **Choose the correct source**: Select from available identities when multiple exist

Error Reporting and Troubleshooting
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

VolSync's enhanced error reporting provides detailed information when restore operations 
encounter issues. This section covers common error scenarios and their solutions.

Common Error Scenarios
^^^^^^^^^^^^^^^^^^^^^^^

**Scenario 1: No snapshots found**

*Error Message*: "No snapshots found for identity '<username>@<hostname>'"

*Symptoms*: 
- ``snapshotsFound`` shows 0
- Restore operation fails
- Error message lists available identities

*Resolution Steps*:

1. **Review available identities**
   
   The error message and ``availableIdentities`` field show what's in the repository:
   
   .. code-block:: bash
   
      kubectl get replicationdestination restore-webapp -o json | jq '.status.kopia.availableIdentities'
   
2. **Verify source configuration**
   
   Check the ReplicationSource that created the backups:
   
   .. code-block:: bash
   
      # Find the source
      kubectl get replicationsource -A | grep webapp-backup
      
      # Check its configuration
      kubectl get replicationsource webapp-backup -n production -o yaml | grep -A 10 "kopia:"
   
3. **Common fixes**:
   
   - **Incorrect sourceIdentity**: Verify sourceName and sourceNamespace match exactly
   - **Custom identity**: If source uses custom username/hostname, use those instead
   - **No backups created**: Check if the ReplicationSource has run successfully

**Scenario 2: Identity mismatch**

*Symptoms*: 
- Data restored from wrong source
- ``requestedIdentity`` doesn't match expectations

*Debugging Process*:

1. Check the requested identity:
   
   .. code-block:: bash
   
      kubectl get replicationdestination restore-webapp -o jsonpath='{.status.kopia.requestedIdentity}'
   
2. Compare with available identities to find the correct one:
   
   .. code-block:: bash
   
      kubectl get replicationdestination restore-webapp -o yaml | grep -A 50 availableIdentities
   
3. Update configuration to use the correct identity

**Scenario 3: Insufficient snapshots for previous parameter**

*Error Message*: "Requested snapshot index X but only Y snapshots found"

*Symptoms*:
- Using ``previous`` parameter
- ``snapshotsFound`` is less than or equal to ``previous`` value

*Solution*:

.. code-block:: yaml

   # Check available snapshot count
   status:
     kopia:
       snapshotsFound: 2  # Only 2 snapshots available
   
   # Adjust previous parameter
   spec:
     kopia:
       previous: 1  # Maximum value is snapshotsFound - 1

Step-by-Step Debugging Process
^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^^

When encountering restore issues, follow this systematic approach:

1. **Check the error message**
   
   .. code-block:: bash
   
      kubectl describe replicationdestination <name> | grep -A 10 "Conditions:"
   
2. **Review discovery information**
   
   .. code-block:: bash
   
      # Check requested identity
      kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
      
      # Check snapshot count
      kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.snapshotsFound}'
      
      # List available identities
      kubectl get replicationdestination <name> -o json | jq '.status.kopia.availableIdentities'
   
3. **Verify source configuration**
   
   .. code-block:: bash
   
      # Find the ReplicationSource
      kubectl get replicationsource -A
      
      # Check its configuration
      kubectl get replicationsource <name> -n <namespace> -o yaml
   
4. **Check mover pod logs**
   
   .. code-block:: bash
   
      # Find the mover pod
      kubectl get pods -l "volsync.backube/mover-job" -n <namespace>
      
      # View logs
      kubectl logs <pod-name> -n <namespace>
   
5. **Adjust configuration based on findings**
   
   Based on the discovery information, update your ReplicationDestination:
   
   - Use correct sourceIdentity values
   - Or switch to explicit username/hostname
   - Adjust previous parameter if needed

Best Practices for Error Prevention
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

1. **Use discovery features proactively**
   
   Before configuring a restore, create a temporary ReplicationDestination to discover 
   available identities:
   
   .. code-block:: yaml
   
      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: identity-discovery
      spec:
        trigger:
          manual: discover
        kopia:
          repository: kopia-config
          destinationPVC: temp-pvc
          copyMethod: Direct
   
   Then check the status for available identities before configuring your actual restore.

2. **Document identity configuration**
   
   Maintain documentation of:
   
   - Custom username/hostname configurations in ReplicationSources
   - Identity mappings between sources and their generated identities
   - Repository organization for multi-tenant setups

3. **Monitor backup success**
   
   Regularly verify that backups are being created successfully:
   
   .. code-block:: bash
   
      # Check last successful backup
      kubectl get replicationsource <name> -o jsonpath='{.status.lastSyncTime}'
   
4. **Test restore procedures**
   
   Regularly test restore operations in non-production environments:
   
   - Verify identity configuration works correctly
   - Test point-in-time recovery with ``previous`` parameter
   - Validate restored data integrity

5. **Use consistent naming conventions**
   
   Maintain consistent ReplicationSource names across environments to simplify:
   
   - Disaster recovery procedures
   - Cross-environment restores
   - Identity management

6. **Leverage error messages**
   
   The enhanced error reporting provides actionable information:
   
   - Lists available identities when snapshots aren't found
   - Shows exact identity being requested
   - Provides snapshot counts and timestamps
   
   Use this information to quickly identify and fix configuration issues.

Basic Restore Example
---------------------

For example, create a PVC to hold the restored data:

.. code-block:: yaml

   ---
   kind: PersistentVolumeClaim
   apiVersion: v1
   metadata:
     name: datavol
   spec:
     accessModes:
       - ReadWriteOnce
     resources:
       requests:
         storage: 3Gi

Restore the data into ``datavol``:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: datavol-dest
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       # Use an existing PVC, don't provision a new one
       destinationPVC: datavol
       copyMethod: Direct

In the above example, the data will be written directly into the new PVC since
it is specified via ``destinationPVC``, and no snapshot will be created since a
``copyMethod`` of ``Direct`` is used.

The restore operation only needs to be performed once, so instead of using a
cronspec-based schedule, a :doc:`manual trigger<../triggers>` is used. After the
restore completes, the ReplicationDestination object can be deleted.

Practical Debugging Example
---------------------------

This example demonstrates how to use the enhanced error reporting to debug a failed restore:

**Scenario**: Attempting to restore data but getting "No snapshots found" error.

**Step 1: Check the error details**

.. code-block:: bash

   kubectl describe replicationdestination webapp-restore

Output shows:

.. code-block:: text

   Status:
     Conditions:
       Type:     Synchronizing
       Status:   False
       Reason:   SnapshotsNotFound
       Message:  No snapshots found for identity 'webapp-backup@production-webapp-data'. 
                Available identities in repository: 
                webapp-backup@staging-webapp-data (15 snapshots), 
                database-backup@production-postgres-data (30 snapshots)

**Step 2: Review available identities**

.. code-block:: bash

   kubectl get replicationdestination webapp-restore -o json | jq '.status.kopia.availableIdentities'

Output:

.. code-block:: json

   [
     {
       "identity": "webapp-backup@staging-webapp-data",
       "snapshotCount": 15,
       "latestSnapshot": "2024-01-20T10:30:00Z"
     },
     {
       "identity": "database-backup@production-postgres-data",
       "snapshotCount": 30,
       "latestSnapshot": "2024-01-20T11:00:00Z"
     }
   ]

**Step 3: Identify the issue**

The error shows that:
- We're looking for ``webapp-backup@production-webapp-data``
- But the repository has ``webapp-backup@staging-webapp-data``
- The namespace is different (staging vs production)

**Step 4: Fix the configuration**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: webapp-restore
     namespace: production
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       # Fix: Use the correct namespace
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: staging  # Changed from production to staging

**Step 5: Verify the fix**

After applying the corrected configuration:

.. code-block:: bash

   kubectl get replicationdestination webapp-restore -o jsonpath='{.status.kopia.snapshotsFound}'
   # Output: 15
   
   kubectl get replicationdestination webapp-restore -o jsonpath='{.status.kopia.requestedIdentity}'
   # Output: webapp-backup@staging-webapp-data

The restore now proceeds successfully with 15 snapshots available.

Point-in-Time and Previous Snapshot Restores
--------------------------------------------

The example, shown above, will restore the data from the most recent backup. To
restore an older version of the data, the ``previous``, ``shallow`` and ``restoreAsOf``
fields can be used. See below for more information on their meaning.

**Example: Restoring from a previous snapshot**

To restore from the second-most-recent snapshot (skipping the latest backup):

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: datavol-dest-previous
   spec:
     trigger:
       manual: restore-previous
     kopia:
       repository: kopia-config
       destinationPVC: datavol
       copyMethod: Direct
       # Skip 1 snapshot to get the previous one
       previous: 1

**Example: Combining previous with restoreAsOf**

To restore from 2 snapshots before a specific point in time:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: datavol-dest-timepoint
   spec:
     trigger:
       manual: restore-timepoint
     kopia:
       repository: kopia-config
       destinationPVC: datavol
       copyMethod: Direct
       # Find snapshots before this time, then skip 2 to get the 3rd oldest
       restoreAsOf: "2024-01-15T14:30:00Z"
       previous: 2

Restore options
---------------

There are a number of additional configuration options not shown in the above
example.

.. include:: ../inc_dst_opts.rst

cacheCapacity
   This determines the size of the Kopia metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the repository metadata. The default is ``1 Gi``.

   **Cache Volume Behavior:**

   - **When specified**: A PersistentVolumeClaim is created with the specified capacity
   - **When not specified**: An EmptyDir volume is used as fallback, providing temporary cache storage

   .. important::
      With EmptyDir fallback, cache contents are lost when the pod restarts, which may
      impact performance for subsequent restore operations as the cache needs to be rebuilt.

cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.

   .. note::
      This setting only applies when ``cacheCapacity`` is specified. EmptyDir volumes
      do not use StorageClasses.

cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.

   .. note::
      This setting only applies when ``cacheCapacity`` is specified. EmptyDir volumes
      do not use access modes.

cleanupCachePVC
   This optional boolean determines if the cache PVC should be cleaned up at
   the end of the restore. Cache PVCs will always be deleted if the owning
   ReplicationDestination is removed, even if this setting is false.
   Defaults to ``false``.

customCA
   This option allows a custom certificate authority to be used when making TLS
   (https) connections to the remote repository.

   key
      This is the name of the field within the Secret that holds the CA
      certificate
   secretName
      This is the name of a Secret containing the CA certificate
   configMapName
      This is the name of a ConfigMap containing the CA certificate

repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository.

sourceIdentity
   This optional field provides a simplified way to specify which ReplicationSource's 
   snapshots to restore. It automatically generates the correct username and hostname 
   based on the source configuration.
   
   sourceName
      The name of the ReplicationSource that created the snapshots to restore
   
   sourceNamespace  
      The namespace where the ReplicationSource exists
   
   When ``sourceIdentity`` is specified, it takes precedence over default identity 
   generation but is overridden by explicit ``username`` and ``hostname`` fields.

previous
   Non-negative integer that specifies how many snapshots to skip before 
   selecting one to restore from. When set to ``0`` (the default), the latest 
   snapshot is used. When set to ``1``, the second newest snapshot is used 
   (skipping 1 snapshot), and so on. This parameter can be combined with 
   ``restoreAsOf`` to skip snapshots from a specific point in time.
   
   Examples:
   
   - ``previous: 0`` or omitted: Use the latest snapshot
   - ``previous: 1``: Skip the latest snapshot, use the previous one
   - ``previous: 2``: Skip 2 snapshots, use the 3rd newest
   
   When used with ``restoreAsOf``, the behavior is the same, but counting 
   starts from the first snapshot taken before the ``restoreAsOf`` timestamp.

restoreAsOf
   An RFC-3339 timestamp which specifies an upper-limit on the snapshots that we
   should be looking through when preparing to restore. Snapshots made after
   this timestamp will not be considered. Note: though this is an RFC-3339
   timestamp, Kubernetes will only accept ones with the day and hour fields
   separated by a ``T``. E.g, ``2022-08-10T20:01:03-04:00`` will work but
   ``2022-08-10 20:01:03-04:00`` will fail.

shallow
   Non-negative integer which specifies how many recent snapshots to consider
   for restore. When ``restoreAsOf`` is provided, the behavior is the same,
   however the starting snapshot considered will be the first one taken
   before ``restoreAsOf``. This is similar to Restic's ``previous`` option
   but uses Kopia's shallow clone concept.