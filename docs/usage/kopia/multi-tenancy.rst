=====================================
Multi-tenancy and Shared Repositories
=====================================

.. contents:: Multi-tenancy Configuration Guide
   :local:

The VolSync Kopia mover implements multi-tenancy through the automatic generation of unique usernames and hostnames for each backup client. This ensures that multiple ReplicationSources and ReplicationDestinations can safely share the same Kopia repository without conflicts.

Each Kopia client requires a unique identity consisting of:

- **Username**: Identifies the tenant/user in the repository
- **Hostname**: Identifies the specific host/instance within that tenant

Enhanced Multi-Tenancy with Namespace-First Hostnames
------------------------------------------------------

VolSync's hostname generation now prioritizes **namespace-based identification**, providing superior multi-tenant isolation by:

- **Grouping backups by namespace**: All backups from the same namespace share a common hostname prefix, making it easier to identify and manage tenant-specific data
- **Improving access control**: Repository administrators can implement namespace-based policies more effectively
- **Simplifying troubleshooting**: Issues can be quickly isolated to specific namespaces/tenants
- **Reducing naming conflicts**: Namespace-first approach minimizes hostname collisions between different tenants using similar PVC names

When multiple clients connect to the same repository with different identities, Kopia can:

- Maintain separate snapshot histories per tenant (namespace)
- Apply different retention policies based on namespace patterns
- Prevent conflicts during concurrent operations across tenants
- Enable namespace-based repository access control

Understanding identity generation
---------------------------------

VolSync automatically generates usernames and hostnames based on your Kubernetes resources. The generation logic prioritizes user customization while providing sensible defaults optimized for multi-tenant environments that ensure uniqueness and compatibility.

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

The hostname generation follows this priority order, designed to optimize multi-tenancy by prioritizing namespace-based identification:

1. **Custom Hostname (Highest Priority)**
   
   If ``spec.kopia.hostname`` is specified, it is used exactly as provided without modification.

2. **Namespace-Based Hostname**
   
   Use the namespace as the primary identifier to improve multi-tenant isolation:
   
   - Start with the resource's namespace name
   - **Append PVC name if total length ≤ 50 characters**:
     
     - **ReplicationSource**: Appends ``spec.sourcePVC`` if specified
     - **ReplicationDestination**: Appends ``spec.kopia.destinationPVC`` if specified
   
   - **Use namespace-only if combined length > 50 characters**
   - **Format**: ``{namespace}`` or ``{namespace}-{pvc-name}`` (when space allows)

3. **Fallback Hostname**
   
   If namespace is empty or becomes empty after sanitization, use the format ``namespace-{name}``

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

   # Example 2: Namespace-first hostname with PVC appended
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: prod
   spec:
     sourcePVC: app-data
     kopia:
       # No hostname specified
       # Combined length: "prod" (4) + "-" (1) + "app-data" (8) = 13 chars ≤ 50
       # Generated hostname: prod-app-data

---

.. code-block:: yaml

   # Example 3: Namespace-only when combined length exceeds 50 characters
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup
     namespace: production-environment
   spec:
     sourcePVC: long-application-storage-pvc-name-v2
     kopia:
       # No hostname specified
       # Combined would be: "production-environment-long-application-storage-pvc-name-v2" = 69 chars > 50
       # Generated hostname: production-environment (namespace only)

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

Troubleshooting
~~~~~~~~~~~~~~~

**Common Issues**

**Issue 1: Repository Access Conflicts**

*Problem*: Multiple backups seem to interfere with each other

*Solution*: Verify that username/hostname combinations are unique:

.. code-block:: bash

   # Check generated identities
   kubectl describe replicationsource my-backup -n my-namespace
   
   # Look for these fields in the status:
   # status.kopia.lastSnapshotId
   # status.latestMoverStatus.logs (contains identity info)

**Issue 2: Unexpected Hostname After Namespace-First Update**

*Problem*: Generated hostnames changed from PVC-based to namespace-based after VolSync update

*Explanation*: VolSync now prioritizes namespace-first hostname generation for better multi-tenancy

*New Behavior*:
- Hostnames start with namespace: ``{namespace}`` or ``{namespace}-{pvc-name}``
- PVC name is appended only if total length ≤ 50 characters
- This improves tenant isolation and reduces naming conflicts

**Issue 3: Understanding Length-Based PVC Inclusion**

*Problem*: PVC name sometimes appears in hostname, sometimes doesn't

*Explanation*: VolSync uses a 50-character limit to determine hostname format

*Debug Steps*:

1. Calculate combined namespace + PVC length:

   .. code-block:: bash
   
      # Check if PVC will be included
      NAMESPACE="your-namespace"
      PVC_NAME="your-pvc"
      COMBINED="${NAMESPACE}-${PVC_NAME}"
      echo "Combined length: $(echo -n "$COMBINED" | wc -c)"
      if [ $(echo -n "$COMBINED" | wc -c) -le 50 ]; then
          echo "Hostname will be: $COMBINED"
      else
          echo "Hostname will be: $NAMESPACE (PVC dropped)"
      fi

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