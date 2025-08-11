=====================================
Multi-tenancy and Shared Repositories
=====================================

.. contents:: Multi-tenancy Configuration Guide
   :local:

The VolSync Kopia mover implements multi-tenancy through the automatic generation of unique usernames and hostnames for each backup client. This ensures that multiple ReplicationSources and ReplicationDestinations can safely share the same Kopia repository without conflicts.

Each Kopia client requires a unique identity consisting of:

- **Username**: Identifies the tenant/user in the repository
- **Hostname**: Identifies the specific host/instance within that tenant


Simplified Multi-Tenancy with Namespace-Only Hostnames
-------------------------------------------------------

VolSync's hostname generation now uses **namespace-only identification**, providing simplified multi-tenant isolation by:

- **Single hostname per namespace**: All PVCs in a namespace share the same hostname identity
- **Clear tenant boundaries**: Each namespace represents a distinct tenant with its own hostname
- **Simplified access control**: Repository administrators can implement namespace-based policies easily
- **Predictable behavior**: Hostname is always just the namespace name, regardless of PVC names
- **No naming conflicts**: Each namespace has a unique hostname, eliminating PVC-based collisions

When multiple clients connect to the same repository with different identities, Kopia can:

- Maintain separate snapshot histories per tenant (namespace)
- Apply different retention policies based on namespace patterns
- Prevent conflicts during concurrent operations across tenants
- Enable namespace-based repository access control

Understanding identity generation
---------------------------------

VolSync automatically generates usernames and hostnames based on your Kubernetes resources. The generation logic prioritizes user customization while providing simple, predictable defaults. The hostname is always just the namespace name (unless customized), making multi-tenancy straightforward.

Username generation logic
~~~~~~~~~~~~~~~~~~~~~~~~~

The username generation follows this priority order:

1. **Custom Username (Highest Priority)**
   
   If ``spec.kopia.username`` is specified, it is used exactly as provided without any sanitization or modification.

2. **Default Username Generation**
   
   When no custom username is provided, VolSync generates one from the ReplicationSource/ReplicationDestination name:
   
   a. **With Namespace**: If the combined length of ``{objectName}-{namespace}`` ≤ 50 characters, use this format
   b. **Object Name Only**: If the combined name is too long, use just the sanitized object name
   c. **Sanitization**: Remove invalid characters and apply character restrictions
   d. **Fallback**: Use "volsync-default" if sanitization results in an empty string

Username examples
~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   # Example 1: Custom username (no modifications applied)
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: production
   spec:
     kopia:
       username: "backup-user@company.com"  # Used exactly as-is
       # Generated username: backup-user@company.com

---

.. code-block:: yaml

   # Example 2: Default generation with namespace
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-data
     namespace: prod
   spec:
     kopia:
       # No username specified
       # Generated username: app-data-prod (≤50 chars)

---

.. code-block:: yaml

   # Example 3: Long names - object name only
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: very-long-application-backup-with-detailed-name
     namespace: production-environment
   spec:
     kopia:
       # Combined length > 50 chars
       # Generated username: very-long-application-backup-with-detailed-name

---

.. code-block:: yaml

   # Example 4: Special characters sanitized
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app@service.backup
     namespace: dev-test
   spec:
     kopia:
       # Special chars removed: @ and . are invalid
       # Generated username: appservicebackup-dev-test

Hostname generation logic
~~~~~~~~~~~~~~~~~~~~~~~~~

The hostname generation follows this simple priority order:

1. **Custom Hostname (Highest Priority)**
   
   If ``spec.kopia.hostname`` is specified, it is used exactly as provided without modification.

2. **Namespace-Only Hostname (Default)**
   
   When no custom hostname is provided, the hostname is ALWAYS just the namespace name:
   
   - Uses the resource's namespace name directly
   - PVC names are NEVER included in the hostname
   - All PVCs in a namespace share the same hostname
   - **Format**: ``{namespace}`` (always, regardless of PVC names or length)

3. **Fallback Hostname**
   
   If namespace is empty or becomes empty after sanitization, use "volsync-default"

4. **Sanitization**
   
   For all generated hostnames:
   
   - Replace underscores with hyphens
   - Remove invalid characters (only alphanumeric, dots, and hyphens allowed)
   - Trim leading/trailing hyphens and dots
   - Use "volsync-default" if sanitization results in empty string

Hostname examples
~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   # Example 1: Custom hostname (unchanged behavior)
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: db-backup
     namespace: production
   spec:
     sourcePVC: mysql-data
     kopia:
       hostname: "mysql-primary.production.local"  # Used exactly as-is
       # Generated hostname: mysql-primary.production.local

---

.. code-block:: yaml

   # Example 2: Namespace-only hostname (default behavior)
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: prod
   spec:
     sourcePVC: app-data
     kopia:
       # No hostname specified
       # Generated hostname: prod (always just namespace, PVC name ignored)

---

.. code-block:: yaml

   # Example 3: Multiple PVCs in same namespace share hostname
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: production-environment
   spec:
     sourcePVC: long-application-storage-pvc-name-v2
     kopia:
       # No hostname specified
       # Generated hostname: production-environment (always namespace only)
   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: db-backup
     namespace: production-environment
   spec:
     sourcePVC: database-pvc
     kopia:
       # No hostname specified
       # Generated hostname: production-environment (same as above, all share namespace hostname)

Character sanitization rules
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

**Username Sanitization**

**Allowed Characters**: ``a-z``, ``A-Z``, ``0-9``, ``-`` (hyphen), ``_`` (underscore)

**Sanitization Process**:

1. Remove all characters not in the allowed set
2. Trim leading and trailing hyphens and underscores
3. If result is empty, use "volsync-default"

**Examples**:

============================================  ==========================
Original Name                                Sanitized Username
============================================  ==========================
``app-backup``                              ``app-backup`` (no change)
``app_backup_job``                          ``app_backup_job`` (no change)  
``app@service.com``                         ``appservicecom``
``-special-chars-``                         ``special-chars``
``@#$%``                                    ``volsync-default``
============================================  ==========================

**Hostname Sanitization**

**Allowed Characters**: ``a-z``, ``A-Z``, ``0-9``, ``.`` (dot), ``-`` (hyphen)

**Sanitization Process**:

1. Replace underscores (``_``) with hyphens (``-``)
2. Remove all characters not in the allowed set
3. Trim leading and trailing hyphens and dots
4. If result is empty, use "volsync-default"

**Examples**:

============================================  ==========================
Original Name                                Sanitized Hostname  
============================================  ==========================
``app-storage-pvc``                         ``app-storage-pvc`` (no change)
``app_storage_pvc``                         ``app-storage-pvc`` (underscores replaced)
``mysql.primary.host``                      ``mysql.primary.host`` (no change)
``host@domain.com``                         ``hostdomain.com``
``--.invalid.--``                           ``invalid``
``___``                                     ``volsync-default``
============================================  ==========================

Customization guide
--------------------

When to use custom values
~~~~~~~~~~~~~~~~~~~~~~~~~

**Custom Username**:

- **Multi-tenant environments**: Use meaningful tenant identifiers like ``tenant-a``, ``dept-finance``
- **Email-based identification**: ``user@company.com`` (will be preserved exactly)
- **Legacy compatibility**: Match existing Kopia repository users
- **Regulatory compliance**: Meet specific naming requirements

**Custom Hostname**:

- **Infrastructure alignment**: Match actual hostnames like ``web01.prod.company.com``
- **Logical grouping**: ``primary-db``, ``backup-replica``, ``cache-layer``
- **Environment consistency**: ``app.production``, ``app.staging``, ``app.development``

Configuration examples
~~~~~~~~~~~~~~~~~~~~~~

**Scenario 1: Multi-Environment Setup**

.. code-block:: yaml

   # Production environment
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-backup
     namespace: production
   spec:
     kopia:
       username: "webapp-prod"
       hostname: "webapp.production.cluster"
   ---
   # Staging environment  
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-backup
     namespace: staging  
   spec:
     kopia:
       username: "webapp-staging"
       hostname: "webapp.staging.cluster"

**Scenario 2: Department-Based Tenancy**

.. code-block:: yaml

   # Finance department backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: accounting-db
     namespace: finance
   spec:
     kopia:
       username: "finance-dept"
       hostname: "accounting-primary"
   ---
   # HR department backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: employee-db
     namespace: hr
   spec:
     kopia:
       username: "hr-dept" 
       hostname: "hr-primary"

Troubleshooting Multi-Tenant Repositories
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

**Using Discovery Features**

VolSync provides enhanced discovery features to help manage multi-tenant repositories:

**Discovering All Tenants/Identities**

To see all identities (tenants) in a shared repository:

.. code-block:: bash

   # Create a temporary ReplicationDestination for discovery
   cat <<EOF | kubectl apply -f -
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: tenant-discovery
     namespace: default
   spec:
     trigger:
       manual: discover
     kopia:
       repository: kopia-config
       destinationPVC: temp-discovery
       copyMethod: Direct
   EOF
   
   # Wait for status to populate
   sleep 10
   
   # View all tenants/identities
   kubectl get replicationdestination tenant-discovery -o json | \
     jq '.status.kopia.availableIdentities[] | 
         {identity: .identity, snapshots: .snapshotCount, latest: .latestSnapshot}'
   
   # Clean up
   kubectl delete replicationdestination tenant-discovery

Example output showing multiple tenants:

.. code-block:: json

   {
     "identity": "finance-dept@finance-accounting-data",
     "snapshots": 45,
     "latest": "2024-01-20T10:00:00Z"
   }
   {
     "identity": "hr-dept@hr-employee-data",
     "snapshots": 30,
     "latest": "2024-01-20T09:30:00Z"
   }
   {
     "identity": "webapp-backup@production-webapp-data",
     "snapshots": 60,
     "latest": "2024-01-20T11:00:00Z"
   }

**Common Issues**

**Issue 1: Repository Access Conflicts**

*Problem*: Multiple backups seem to interfere with each other

*Solution*: Use the discovery features to verify unique identities:

.. code-block:: bash

   # Check what identity a source is using
   kubectl describe replicationsource my-backup -n my-namespace
   
   # Use discovery to see all identities
   kubectl get replicationdestination <discovery-dest> -o json | \
     jq '.status.kopia.availableIdentities[].identity'

*Alternative Solution*: When restoring, use the ``sourceIdentity`` field to automatically 
match the source's identity:

.. code-block:: yaml

   spec:
     kopia:
       sourceIdentity:
         sourceName: my-backup
         sourceNamespace: my-namespace
         # sourcePVCName: optional - auto-discovered if not provided

**Issue 2: Hostname Changed to Namespace-Only**

*Problem*: Generated hostnames changed from including PVC names to just namespace after VolSync update

*Explanation*: VolSync now uses namespace-only hostname generation for simplicity

*New Behavior*:
- Hostname is ALWAYS just the namespace: ``{namespace}``
- PVC names are NEVER included in the hostname
- All PVCs in a namespace share the same hostname identity
- This simplifies multi-tenancy and makes behavior predictable

**Issue 3: All PVCs Share Same Hostname**

*Problem*: Multiple PVCs in the same namespace have the same hostname

*Explanation*: This is the expected behavior - hostname is always just the namespace

*Implications*:

- All PVCs in a namespace share the same Kopia hostname identity
- This simplifies multi-tenancy - one hostname per namespace
- Different PVCs are distinguished by their snapshot paths, not hostnames
- If you need separate hostnames per PVC, use custom hostname configuration

*Verify the hostname*:

   .. code-block:: bash
   
      # Check what identity was actually generated
      kubectl get replicationdestination <name> -o jsonpath='{.status.kopia.requestedIdentity}'
      # The hostname part (after @) will always be just the namespace

**Issue 4: Identifying Snapshots from Wrong Tenant**

*Problem*: Restored wrong tenant's data

*Solution*: Use the enhanced error reporting to identify correct tenant:

.. code-block:: bash

   # View error message with available identities
   kubectl describe replicationdestination <name> | grep -A 10 "Message:"
   
   # List all available identities with details
   kubectl get replicationdestination <name> -o json | \
     jq '.status.kopia.availableIdentities[] | 
         select(.identity | contains("<namespace>"))'

The error message will show all available identities, making it easy to identify 
the correct one for your tenant/namespace.

**Character Validation Patterns**

The API enforces validation patterns for custom usernames and hostnames:

**Pattern**: ``^[a-zA-Z0-9][a-zA-Z0-9._-]*[a-zA-Z0-9]$|^[a-zA-Z0-9]$``

**Requirements**:

- Must start and end with alphanumeric character
- Middle characters can include ``.``, ``_``, ``-``
- Single character names are allowed
- Cannot be empty

**Valid Examples**:

- ``user1``
- ``backup-user`` 
- ``tenant.backup_job``
- ``a`` (single character)

**Invalid Examples**:

- ``-backup-user`` (starts with hyphen)
- ``backup-user-`` (ends with hyphen)
- ``.backup.user.`` (starts/ends with dot)
- ``backup user`` (contains space)
- ```` (empty string)

Identity Requirement for ReplicationDestination
------------------------------------------------

.. important::
   **Kopia ReplicationDestination requires explicit identity configuration**
   
   Unlike other movers, Kopia ReplicationDestination cannot automatically determine which 
   snapshots to restore from because:
   
   - The destination doesn't know the source PVC name (part of the hostname)
   - Multiple backup sources may exist in the same repository
   - Each source has a unique identity (username@hostname)
   
   You **MUST** provide either:
   
   1. ``sourceIdentity`` with at least ``sourceName`` and ``sourceNamespace`` (recommended)
   2. Both ``username`` AND ``hostname`` fields explicitly
   
   Without this, the ReplicationDestination will fail validation with an error.

Simplified Restore with sourceIdentity
---------------------------------------

For ReplicationDestination resources, the ``sourceIdentity`` field provides a streamlined 
approach to restoring from specific sources in multi-tenant repositories:

**Traditional Approach (Manual Identity)**

.. code-block:: yaml

   # You need to know the exact username and hostname
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-data
   spec:
     kopia:
       # Must match exactly what the source used
       username: "webapp-backup-production"
       hostname: "production-webapp-pvc"

**Simplified Approach (sourceIdentity with Auto-Discovery)**

.. code-block:: yaml

   # Just specify the source name and namespace
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-data
   spec:
     kopia:
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production
         # sourcePVCName is optional - auto-discovered but doesn't affect hostname
       # VolSync automatically:
       # 1. Fetches the ReplicationSource configuration
       # 2. Discovers the sourcePVC name from the source
       # 3. Generates matching username/hostname

**Approach with Explicit PVC Name**

.. code-block:: yaml

   # Optionally specify the source PVC name explicitly
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-data
   spec:
     kopia:
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production
         sourcePVCName: webapp-data  # Optional - for reference only, doesn't affect hostname

This is especially useful in multi-tenant scenarios where:

- Multiple teams share the same repository
- You need to restore data across namespaces
- Identity generation rules have changed over time
- You want to avoid manual identity management errors

Best practices for shared repositories
---------------------------------------

**Naming Strategies**

**Environment-Based**:

.. code-block:: yaml

   # Pattern: {app}-{env}
   spec:
     kopia:
       username: "webapp-prod"
       hostname: "web01.production"

**Department-Based**:

.. code-block:: yaml

   # Pattern: {dept}-{resource}
   spec:
     kopia:
       username: "finance-database"
       hostname: "accounting-primary"

**Function-Based**:

.. code-block:: yaml

   # Pattern: {function}-{instance}
   spec:
     kopia:
       username: "backup-agent"
       hostname: "web-tier-01"

**Security Considerations**

**Username Security**:

- Use descriptive but not sensitive information
- Avoid including secrets or passwords
- Consider audit trail requirements