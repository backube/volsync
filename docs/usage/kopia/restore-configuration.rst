======================
Restore Configuration
======================

.. contents:: Restore Operations and Configuration
   :local:

Data from a backup can be restored using the ReplicationDestination CR. In most
cases, it is desirable to perform a single restore into an empty
PersistentVolume.

.. note::
   **Simplified Identity Configuration for Kopia ReplicationDestination**
   
   Identity configuration is now **OPTIONAL**! When not provided, VolSync automatically 
   determines the appropriate identity, making simple same-namespace restores easier.
   
   **Automatic Identity** (when no identity fields are provided):
   
   - Username: ``<destination-name>``
   - Hostname: ``<namespace>``
   
   This works perfectly when the ReplicationDestination has the same name as the 
   ReplicationSource and both are in the same namespace.
   
   For more complex scenarios, you can still use:
   
   1. **sourceIdentity**: For cross-namespace restores or different names
   2. **Explicit identity**: Provide both ``username`` AND ``hostname`` for custom control

Simple Same-Namespace Restore (No Configuration Needed)
--------------------------------------------------------

The simplest way to restore data is to create a ReplicationDestination with the same 
name as your ReplicationSource. No identity configuration is needed!

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: my-backup  # Same name as the ReplicationSource
     namespace: production
   spec:
     kopia:
       destinationPVC: restored-data
       repository: kopia-config
       # No identity configuration needed!
       # Automatically uses:
       # - username: my-backup-production
       # - hostname: production

Advanced: Restoring from a Specific ReplicationSource (sourceIdentity)
----------------------------------------------------------------------

For cross-namespace restores or when the destination has a different name than the source, 
use the ``sourceIdentity`` field. This provides automatic discovery of the source configuration.

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

How Identity Generation Works with sourceIdentity
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When you use ``sourceIdentity``, VolSync generates the identity as follows:

**Username Generation**:

The username is generated from the ``sourceName`` field using the same logic as the 
ReplicationSource would use:

- If the source has a custom username, that would need to be specified explicitly
- Otherwise, uses the sanitized source name

**Hostname Generation**:

The hostname is generated simply from the namespace:

1. **Standard behavior**:
   
   .. code-block:: yaml
   
      sourceIdentity:
        sourceName: webapp-backup
        sourceNamespace: production
        sourcePVCName: webapp-data  # Optional - doesn't affect hostname
   
   Generates hostname: ``production`` (always just the namespace)

2. **With auto-discovery (sourcePVCName not provided)**:
   
   .. code-block:: yaml
   
      sourceIdentity:
        sourceName: webapp-backup
        sourceNamespace: production
        # sourcePVCName will be fetched from the ReplicationSource for reference
   
   VolSync will:
   - Query the ReplicationSource ``webapp-backup`` in namespace ``production``
   - Read its ``spec.sourcePVC`` field for reference (e.g., ``webapp-data``)
   - Generate hostname: ``production`` (always just namespace, PVC name not used)

3. **Key point**:
   
   The hostname is ALWAYS just the namespace name. All PVCs in a namespace share the same hostname.

**Complete Identity Example**:

For a ReplicationSource with:
- Name: ``webapp-backup``
- Namespace: ``production``
- sourcePVC: ``webapp-data``

The generated identity would be: ``webapp-backup-production@production``
(username includes namespace suffix, hostname is just namespace)

Using sourceIdentity
~~~~~~~~~~~~~~~~~~~~

The ``sourceIdentity`` field accepts three parameters:

- ``sourceName`` (required): The name of the ReplicationSource that created the backups
- ``sourceNamespace`` (required): The namespace where the ReplicationSource exists
- ``sourcePVCName`` (optional): The name of the source PVC - auto-discovered if not provided

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

1. Fetch the ReplicationSource configuration from ``webapp-backup`` in the ``production`` namespace
2. Auto-discover the ``sourcePVC`` name from the ReplicationSource (if ``sourcePVCName`` not provided)
3. Generate the username based on the source name (``webapp-backup``)
4. Generate the hostname using just the namespace (PVC name not included)
5. Connect to the repository using these generated credentials
6. Restore from the snapshots created by that specific ReplicationSource

**Example: Cross-namespace restore with auto-discovery**

You can restore snapshots created in a different namespace. For comprehensive guidance on
cross-namespace restore scenarios including disaster recovery, environment cloning, and 
namespace migration, see :doc:`cross-namespace-restore`.

Basic cross-namespace restore example:

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
         # sourcePVCName automatically discovered from staging/app-backup ReplicationSource

**Example: Explicit PVC name (bypassing auto-discovery)**

You can explicitly specify the source PVC name if needed:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-specific-pvc
     namespace: production
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       sourceIdentity:
         sourceName: multi-pvc-backup
         sourceNamespace: production
         sourcePVCName: specific-data-pvc  # Explicitly specify which PVC's snapshots to restore

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
         # Auto-discovery will fetch the PVC name from the source
       # Skip the latest snapshot, use the previous one
       previous: 1

Auto-Discovery Feature Details
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The auto-discovery feature simplifies configuration by automatically fetching the source 
PVC name from the ReplicationSource. This is particularly useful when:

1. **You don't remember the exact PVC name**: No need to look up the source configuration
2. **PVC names might change**: Auto-discovery always gets the current value
3. **Reducing configuration errors**: Eliminates typos in PVC names
4. **Simplifying documentation**: Restore procedures don't need to specify PVC names

**How Auto-Discovery Works**

When ``sourcePVCName`` is not provided in ``sourceIdentity``:

1. VolSync queries the Kubernetes API for the specified ReplicationSource
2. It reads the ``spec.sourcePVC`` field from the ReplicationSource
3. This value is used to generate the hostname for identity matching
4. If the ReplicationSource cannot be found or accessed, an error is reported

**Auto-Discovery Requirements**

- The ReplicationSource must exist in the specified namespace
- The operator must have permission to read ReplicationSource resources
- The ReplicationSource must have a ``spec.sourcePVC`` field defined

**When to Use Explicit sourcePVCName**

While auto-discovery is convenient, you might want to specify ``sourcePVCName`` explicitly when:

1. **Cross-cluster restores**: The source cluster/ReplicationSource is not accessible
2. **Performance optimization**: Avoid the API call to fetch the ReplicationSource
3. **Historical restores**: Restoring from a source that no longer exists
4. **Custom scenarios**: When you need to override the discovered value

Repository Auto-Discovery
~~~~~~~~~~~~~~~~~~~~~~~~~

VolSync can automatically discover and use the repository configuration from the ReplicationSource 
when using ``sourceIdentity``. This feature reduces manual configuration overhead by automatically 
fetching the repository secret name from the source backup configuration.

**How Repository Auto-Discovery Works**

When using ``sourceIdentity``, VolSync can automatically discover the repository configuration:

1. **Query the ReplicationSource**: VolSync fetches the ReplicationSource specified in ``sourceIdentity``
2. **Extract Repository**: If the ReplicationSource has ``spec.kopia.repository`` configured, 
   VolSync automatically uses this value for the restore operation
3. **Apply to Restore**: The discovered repository is used to connect to the backup repository

**Priority Order for Repository Configuration**

VolSync resolves the repository with the following priority:

1. **Explicit repository in ReplicationDestination**: Highest priority (manual override)
2. **Auto-discovered from ReplicationSource**: Automatic discovery when destination repository is empty
3. **Empty repository**: If neither explicit nor discovered repository is available, results in configuration error

**Example: Repository Auto-Discovery in Action**

Consider this ReplicationSource with a repository configuration:

.. code-block:: yaml

   # Original ReplicationSource that created the backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-backup
     namespace: production
   spec:
     sourcePVC: webapp-data
     kopia:
       repository: production-kopia-config  # Repository secret name

When restoring with ``sourceIdentity``, the repository is automatically discovered:

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
       # No repository specified - will be auto-discovered!
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production

VolSync will automatically:

1. Fetch the ``webapp-backup`` ReplicationSource from the ``production`` namespace
2. Discover that it uses ``repository: production-kopia-config``
3. Use this repository configuration for the restore operation
4. Connect to the same backup repository that the source used

**Manual Override of Auto-Discovery**

You can override the auto-discovered repository by specifying it explicitly:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: webapp-restore-custom
   spec:
     kopia:
       # Explicit repository overrides auto-discovery
       repository: different-repo-config
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production

**Cross-Environment Restore Example**

Repository auto-discovery simplifies cross-environment restores when the same repository is shared:

.. code-block:: yaml

   # Restore production data to staging using auto-discovered repository
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-prod-to-staging
     namespace: staging
   spec:
     trigger:
       manual: restore-once
     kopia:
       # Repository will be auto-discovered from production source
       destinationPVC: staging-data
       copyMethod: Direct
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production  # Cross-namespace restore

This restore operation will:

1. Discover the repository configuration from ``production/webapp-backup``
2. Use the same repository that production uses for its backups
3. Restore the production snapshots to the staging environment

**Before and After: Configuration Simplification**

Repository auto-discovery significantly reduces the configuration overhead when setting up restore operations.

**Before (Manual Configuration)**:

.. code-block:: yaml

   # You had to manually specify all details
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: webapp-restore
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: production-kopia-config  # Must manually specify
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       # Manual identity configuration (old format for compatibility)
       username: webapp-backup-production
       hostname: production  # Now always just namespace

**After (Auto-Discovery)**:

.. code-block:: yaml

   # VolSync handles all the discovery automatically
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: webapp-restore
   spec:
     trigger:
       manual: restore-once
     kopia:
       # Repository auto-discovered - no manual configuration needed!
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       sourceIdentity:
         sourceName: webapp-backup      # Just specify the source
         sourceNamespace: production    # And its namespace

With auto-discovery, VolSync automatically:

- Discovers ``repository: production-kopia-config`` from the ReplicationSource
- Generates ``username: webapp-backup`` from the source name
- Generates ``hostname: production`` (always just the namespace, PVC name not included)
- Discovers any ``sourcePathOverride`` settings from the source

sourcePathOverride Auto-Discovery
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

In addition to auto-discovering the source PVC name and repository, VolSync can also automatically discover 
and use the ``sourcePathOverride`` setting from the ReplicationSource. This ensures that 
restore operations use the correct snapshot path when the source used a path override.

**How sourcePathOverride Auto-Discovery Works**

When using ``sourceIdentity``, VolSync can automatically discover the ``sourcePathOverride`` 
from the ReplicationSource configuration:

1. **Query the ReplicationSource**: VolSync fetches the ReplicationSource specified in ``sourceIdentity``
2. **Extract sourcePathOverride**: If the ReplicationSource has ``spec.kopia.sourcePathOverride`` configured, 
   VolSync automatically uses this value for the restore operation
3. **Apply to restore**: The discovered ``sourcePathOverride`` is used to restore from the correct snapshot path

**Priority Order for sourcePathOverride**

VolSync resolves ``sourcePathOverride`` with the following priority:

1. **Explicit sourcePathOverride in sourceIdentity**: Highest priority (manual override)
2. **Auto-discovered from ReplicationSource**: Automatic discovery when not explicitly provided
3. **No sourcePathOverride**: Standard restore without path override

**Example: Auto-Discovery in Action**

Consider this ReplicationSource with a path override:

.. code-block:: yaml

   # Original ReplicationSource that created the backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-backup
     namespace: production
   spec:
     sourcePVC: webapp-data
     kopia:
       repository: kopia-config
       # This path override was used during backup
       sourcePathOverride: "/var/lib/webapp/data"

When restoring with ``sourceIdentity``, the ``sourcePathOverride`` is automatically discovered:

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
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production
         # No need to specify sourcePathOverride - it's auto-discovered!

VolSync will automatically:

1. Fetch the ``webapp-backup`` ReplicationSource from the ``production`` namespace
2. Discover that it uses ``sourcePathOverride: "/var/lib/webapp/data"``
3. Apply this path override during the restore operation
4. Restore the data from the correct snapshot path

**Manual Override of Auto-Discovery**

You can override the auto-discovered ``sourcePathOverride`` by specifying it explicitly:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: webapp-restore-override
   spec:
     kopia:
       repository: kopia-config
       destinationPVC: webapp-data-restored
       copyMethod: Direct
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production
         # Explicitly override the auto-discovered value
         sourcePathOverride: "/custom/restore/path"

**Cross-Environment Restore Example**

Auto-discovery makes cross-environment restores seamless:

.. code-block:: yaml

   # Restore production data to staging, preserving the path override
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-prod-to-staging
     namespace: staging
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: staging-data
       copyMethod: Direct
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production  # Cross-namespace restore
         # Both PVC name and sourcePathOverride are auto-discovered

This restore operation will:

1. Discover the source PVC name from ``production/webapp-backup``
2. Discover the ``sourcePathOverride`` from the same ReplicationSource
3. Generate the correct identity: ``webapp-backup-production@production``
4. Restore using the correct snapshot path override

**Benefits of sourcePathOverride Auto-Discovery**

1. **Simplified Configuration**: No need to manually specify path overrides
2. **Error Reduction**: Eliminates mismatches between backup and restore path settings
3. **Cross-Environment Compatibility**: Automatic path override preservation across environments
4. **Maintenance Reduction**: Changes to source path overrides don't require destination updates

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
2. **sourceIdentity with explicit sourcePVCName**: Uses provided PVC name for hostname generation
3. **sourceIdentity with auto-discovery**: Fetches PVC name from ReplicationSource
4. **Default generation**: If none specified, generates based on ReplicationDestination name and namespace

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
   # Output: webapp-backup-production@production

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
       message: "No snapshots found for identity 'webapp-backup-production@production'. 
                Available identities in repository: 
                database-backup-production@production (30 snapshots, latest: 2024-01-20T11:00:00Z), 
                app-backup-staging@staging (7 snapshots, latest: 2024-01-19T22:00:00Z)"
     kopia:
       requestedIdentity: "webapp-backup-production@production"
       snapshotsFound: 0
       availableIdentities:
       - identity: "database-backup-production@production"
         snapshotCount: 30
         latestSnapshot: "2024-01-20T11:00:00Z"
       - identity: "app-backup-staging@staging"
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
       requestedIdentity: "webapp-backup-production@production"
       snapshotsFound: 15
       availableIdentities:
       - identity: "webapp-backup-production@production"
         snapshotCount: 15
         latestSnapshot: "2024-01-20T10:30:00Z"
       - identity: "database-backup-production@production"
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

Practical Auto-Discovery Example
---------------------------------

This example demonstrates how auto-discovery simplifies restore configuration:

**Step 1: Create a ReplicationSource for backup**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-backup
     namespace: production
   spec:
     sourcePVC: webapp-persistent-data
     trigger:
       schedule: "0 */6 * * *"
     kopia:
       repository: kopia-config

**Step 2: Restore using sourceIdentity with auto-discovery**

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
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production
         # Note: sourcePVCName not specified - will be auto-discovered

**Step 3: Verify the auto-discovery worked**

.. code-block:: bash

   # Check the requested identity
   kubectl get replicationdestination webapp-restore -o jsonpath='{.status.kopia.requestedIdentity}'
   # Output: webapp-backup@production-webapp-persistent-data
   #         The PVC name "webapp-persistent-data" was auto-discovered

**Benefits of this approach**:

1. No need to remember or look up the PVC name
2. Configuration remains valid even if the source PVC name changes
3. Reduces configuration errors from typos
4. Simplifies restore documentation and procedures

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

enableFileDeletion
   A boolean field that controls whether the destination directory should be cleaned 
   before restore operations. When set to ``true``, all files and directories in the 
   destination (except ``lost+found``) are removed before the restore begins, ensuring 
   the restored data exactly matches the snapshot content. When ``false`` (the default), 
   Kopia's standard behavior applies: existing files are overwritten, new files are 
   created, but extra files not in the snapshot remain untouched.
   
   **Use Cases:**
   
   - **Disaster recovery**: Ensuring exact state restoration without leftover files
   - **Test environment refresh**: Cleaning test data before restoring production snapshots
   - **Clean slate restores**: Removing orphaned or accumulated files from previous restores
   - **Compliance requirements**: Ensuring no unauthorized data remains after restore
   
   **Important Considerations:**
   
   - This operation is destructive - all existing data in the destination will be deleted
   - The ``lost+found`` directory (if present) is preserved for filesystem integrity
   - Ensure you have backups if the destination contains important data
   - The cleaning happens before each restore operation when enabled
   
   **Example Usage:**
   
   .. code-block:: yaml
   
      apiVersion: volsync.backube/v1alpha1
      kind: ReplicationDestination
      metadata:
        name: clean-restore
      spec:
        kopia:
          enableFileDeletion: true  # Clean destination before restore
          destinationPVC: restored-data
          repository: kopia-config
          sourceIdentity:
            sourceName: webapp-backup
            sourceNamespace: production
   
   **Default Behavior (when false or not specified):**
   
   Without this flag, Kopia follows its standard restore behavior:
   
   - Existing files with the same name are overwritten
   - New files from the snapshot are created
   - Extra files not in the snapshot remain untouched
   - This can lead to a mix of old and new data
   
   **When to Use:**
   
   - Enable when you need the destination to exactly match the snapshot
   - Enable for disaster recovery scenarios
   - Enable when refreshing test/staging environments from production
   - Disable when you want to preserve existing files not in the backup
   - Disable when doing partial restores or merging data

repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository. When empty and using
   ``sourceIdentity``, the repository will be automatically discovered from
   the specified ReplicationSource, eliminating the need for manual configuration.

sourceIdentity
   This optional field provides a simplified way to specify which ReplicationSource's 
   snapshots to restore. It automatically generates the correct username and hostname 
   based on the source configuration, with auto-discovery of the source PVC name, 
   sourcePathOverride, and repository configuration.
   
   sourceName
      The name of the ReplicationSource that created the snapshots to restore (required)
   
   sourceNamespace  
      The namespace where the ReplicationSource exists (required)
   
   sourcePVCName
      The name of the source PVC (optional). If not provided, VolSync will automatically 
      fetch the ReplicationSource configuration and discover the PVC name from the 
      ``spec.sourcePVC`` field. This auto-discovery simplifies configuration and reduces errors.
   
   sourcePathOverride
      The path override to use for snapshot restoration (optional). If not provided, VolSync 
      will automatically fetch the ReplicationSource configuration and discover the 
      sourcePathOverride from the ``spec.kopia.sourcePathOverride`` field. When specified, 
      this value overrides any auto-discovered path override. This ensures that restore 
      operations use the correct snapshot path when the source used a path override.
   
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

policyConfig
   This optional field allows applying external policy files during restore operations.
   While restore operations typically don't need policy configuration (policies mainly
   affect backup creation), this can be useful for:
   
   - Setting download speed limits during restore
   - Configuring restore-specific actions
   - Applying custom repository settings for restore operations
   
   The same configuration options as ReplicationSource are available:
   
   configMapName
      Name of a ConfigMap containing policy files (mutually exclusive with secretName)
   
   secretName
      Name of a Secret containing policy files (mutually exclusive with configMapName)
   
   globalPolicyFilename
      Filename for global policy (defaults to "global-policy.json")
   
   repositoryConfigFilename
      Filename for repository config (defaults to "repository.config")
   
   repositoryConfig
      Inline JSON configuration for advanced repository settings

Using External Policies with Restore
-------------------------------------

While restore operations typically don't require policy configuration (since policies
primarily affect how backups are created), there are scenarios where applying policies
during restore can be beneficial.

**Example: Restore with Download Speed Limits**

Limit download speed to prevent network saturation during restore:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: restore-policies
   data:
     repository.config: |
       {
         "downloadSpeed": 52428800,
         "downloadParallelism": 2,
         "enableActions": true
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: throttled-restore
   spec:
     trigger:
       manual: restore-now
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       
       # Apply policies during restore
       policyConfig:
         configMapName: restore-policies
       
       sourceIdentity:
         sourceName: production-backup
         sourceNamespace: production

**Example: Restore with Pre/Post Actions**

Execute commands before and after restore operations:

.. code-block:: yaml

   ---
   apiVersion: v1
   kind: Secret
   metadata:
     name: restore-actions
   type: Opaque
   stringData:
     repository.config: |
       {
         "enableActions": true,
         "actionCommandTimeout": "10m"
       }
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-with-actions
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       destinationPVC: database-restored
       
       policyConfig:
         secretName: restore-actions
       
       # Actions to prepare and verify restore
       actions:
         beforeSnapshot: |
           echo "Starting restore at $(date)" >> /tmp/restore.log
           # Stop services that might access the volume
           pkill -f myapp || true
         afterSnapshot: |
           echo "Restore completed at $(date)" >> /tmp/restore.log
           # Verify restored data
           test -f /data/critical-file.db || exit 1
           # Restart services
           /usr/local/bin/start-app.sh
       
       sourceIdentity:
         sourceName: database-backup
         sourceNamespace: production

**Example: Advanced Restore with Structured Config**

Use inline JSON configuration for complex restore scenarios:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: advanced-restore
   spec:
     trigger:
       manual: restore-now
     kopia:
       destinationPVC: restored-app
       copyMethod: Direct
       
       # Inline repository configuration for restore
       policyConfig:
         repositoryConfig: |
           {
             "caching": {
               "cacheDirectory": "/tmp/restore-cache",
               "maxCacheSize": 5368709120,
               "metadataCacheSize": 1073741824
             },
             "performance": {
               "downloadParallelism": 8,
               "downloadSpeed": 209715200,
               "bufferSize": 33554432
             },
             "verification": {
               "verifyData": true,
               "verifyChecksums": true
             }
           }
       
       # Restore from specific point in time
       restoreAsOf: "2024-01-15T10:30:00Z"
       
       sourceIdentity:
         sourceName: app-backup
         sourceNamespace: production

Best Practices for Restore Policies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

1. **Performance Tuning**:
   - Use download speed limits to prevent network congestion
   - Adjust parallelism based on available resources
   - Configure caching for large restores

2. **Action Scripts**:
   - Stop services before restore to prevent data corruption
   - Verify critical files after restore
   - Send notifications on completion

3. **Security**:
   - Use Secrets for sensitive configuration
   - Validate JSON syntax before applying
   - Keep policy files under 1MB

4. **Testing**:
   - Test restore policies in non-production first
   - Monitor restore performance with different settings
   - Document optimal configurations for different scenarios

Troubleshooting Restore Policies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

**Policy Not Applied During Restore**

Check if the policy ConfigMap/Secret exists:

.. code-block:: bash

   kubectl get configmap restore-policies -o yaml
   kubectl get secret restore-actions -o yaml

**Download Speed Not Limited**

Verify the repository.config is being applied:

.. code-block:: bash

   # Check mover pod logs
   kubectl logs <restore-pod> | grep -i "download.*speed"

**Actions Not Executing**

Ensure enableActions is set to true:

.. code-block:: bash

   # Check the repository config
   kubectl get configmap restore-policies -o jsonpath='{.data.repository\.config}' | jq .

**JSON Validation Errors**

Validate JSON syntax:

.. code-block:: bash

   # Extract and validate JSON
   kubectl get configmap restore-policies -o jsonpath='{.data.repository\.config}' | jq .

Complete Restore Example with All Features
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Here's a comprehensive example combining all restore features with external policies:

.. code-block:: yaml

   # Comprehensive restore policies
   ---
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: full-restore-policies
   data:
     repository.config: |
       {
         "enableActions": true,
         "actionCommandTimeout": "15m",
         "downloadSpeed": 104857600,
         "downloadParallelism": 4,
         "verifyData": true
       }
   
   # Complete restore configuration
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: full-featured-restore
   spec:
     trigger:
       manual: restore-now
     
     kopia:
       # Repository auto-discovered from sourceIdentity
       destinationPVC: production-data-restored
       copyMethod: Snapshot
       
       # Apply external policies
       policyConfig:
         configMapName: full-restore-policies
       
       # Source identification with auto-discovery
       sourceIdentity:
         sourceName: production-backup
         sourceNamespace: production
         # sourcePVCName auto-discovered
         # sourcePathOverride auto-discovered
         # repository auto-discovered
       
       # Point-in-time restore
       restoreAsOf: "2024-01-15T00:00:00Z"
       previous: 1  # Use second-newest from that time
       
       # Clean destination before restore
       enableFileDeletion: true
       
       # Custom CA if needed
       customCA:
         configMapName: custom-ca-bundle
         key: ca-bundle.pem
       
       # Cache configuration
       cacheCapacity: 5Gi
       cacheStorageClassName: fast-ssd
       
       # Performance settings
       parallelism: 4
       
       # Pre/post restore actions
       actions:
         beforeSnapshot: |
           echo "=== Pre-restore checks ==="
           df -h /data
           # Stop application
           systemctl stop myapp || true
           # Backup current state
           tar czf /tmp/pre-restore-backup.tar.gz /data 2>/dev/null || true
         afterSnapshot: |
           echo "=== Post-restore validation ==="
           # Verify critical files
           test -f /data/app.conf || exit 1
           test -d /data/database || exit 1
           # Set permissions
           chown -R app:app /data
           # Start application
           systemctl start myapp
           echo "Restore completed successfully"
       
       # Security context
       moverSecurityContext:
         runAsUser: 1000
         runAsGroup: 1000
         fsGroup: 1000
   
   status:
     # Status will show:
     kopia:
       requestedIdentity: "production-backup-production@production"
       snapshotsFound: 42
       latestSnapshotTime: "2024-01-15T12:00:00Z"