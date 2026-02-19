================================
Kopia Hostname Design Explained
================================

.. contents:: Understanding VolSync's Intentional Hostname Design
   :local:

Executive Summary
=================

VolSync's Kopia integration uses a deliberate hostname design where:

- **Hostname = Namespace** (always, unless explicitly customized)
- **Username = ReplicationSource/ReplicationDestination name** (by default)
- **Result**: Unique identities like ``webapp-backup@production``, ``db-backup@production``

This is **intentional design**, not a limitation. It leverages Kubernetes' built-in uniqueness 
guarantees to provide robust multi-tenancy without collision risks.

The Design Philosophy
=====================

Core Principle
--------------

VolSync treats the Kubernetes namespace as the multi-tenancy boundary. This aligns with 
Kubernetes' own design philosophy where namespaces provide:

- Resource isolation
- Access control boundaries
- Logical grouping of related resources

By making hostname equal to namespace, VolSync extends this model to backup repositories.

Why Namespace-Only Hostnames?
------------------------------

**1. Leverages Kubernetes Guarantees**

Kubernetes enforces that object names must be unique within a namespace. By using:

- Namespace as hostname
- Object name as username

We get guaranteed unique identities without additional complexity.

**2. Predictable Behavior**

You always know the hostname will be the namespace name. No need to:

- Calculate combined string lengths
- Wonder if PVC names are included
- Debug unexpected hostname formats

**3. Multi-Tenancy Clarity**

Each namespace represents a tenant. All backups from that namespace share the same hostname, 
making it clear they belong to the same tenant.

**4. Simplified Management**

Repository administrators can apply policies at the namespace (hostname) level, such as:

- Retention policies per namespace
- Access controls per namespace
- Quota management per namespace

How It Works in Practice
=========================

Single Namespace, Multiple Sources
-----------------------------------

Consider a production namespace with multiple applications:

.. code-block:: yaml

   # First application backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-backup
     namespace: production
   spec:
     sourcePVC: webapp-data
     kopia:
       repository: shared-repo
   # Generated identity: webapp-backup@production

   ---
   # Database backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-backup
     namespace: production
   spec:
     sourcePVC: postgres-data
     kopia:
       repository: shared-repo
   # Generated identity: database-backup@production

   ---
   # Cache backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: cache-backup
     namespace: production
   spec:
     sourcePVC: redis-data
     kopia:
       repository: shared-repo
   # Generated identity: cache-backup@production

Result:

- All three share hostname: ``production``
- Each has unique username: ``webapp-backup``, ``database-backup``, ``cache-backup``
- Each has unique identity: No collision possible
- Clear tenant boundary: All belong to the ``production`` tenant

Multi-Environment Setup
-----------------------

Different environments with consistent naming:

.. code-block:: yaml

   # Production environment
   metadata:
     name: app-backup
     namespace: production
   # Identity: app-backup@production

   # Staging environment
   metadata:
     name: app-backup
     namespace: staging
   # Identity: app-backup@staging

   # Development environment
   metadata:
     name: app-backup
     namespace: development
   # Identity: app-backup@development

Benefits:

- Same ReplicationSource name across environments
- Different hostnames (namespaces) keep them separate
- Easy to identify environment from hostname
- Simplified automation and templating

Addressing Common Concerns
==========================

"But Multiple Sources Share the Same Hostname!"
------------------------------------------------

**This is intentional and safe.**

While multiple ReplicationSources in a namespace share the same hostname, they have different 
usernames. The combination (username@hostname) is always unique because:

1. Kubernetes prevents duplicate object names in a namespace
2. Each ReplicationSource has a unique name
3. Therefore, each gets a unique username
4. Result: Unique identity guaranteed

**Example**:

- ``webapp-backup@production`` - Unique
- ``db-backup@production`` - Unique
- ``cache-backup@production`` - Unique

All share hostname ``production`` but have different usernames.

"What About PVC Names?"
------------------------

**PVC names don't affect hostname by design.**

The PVC name is irrelevant to the backup identity because:

1. The ReplicationSource name already provides uniqueness
2. PVC names can change without affecting backup identity
3. Multiple ReplicationSources might backup the same PVC
4. The namespace boundary is more meaningful than PVC names

"Is This a Security Risk?"
---------------------------

**No, there's no security risk.**

Each ReplicationSource still has:

- A unique identity (username@hostname)
- Separate snapshot history
- Independent backup operations
- No cross-contamination possible

The shared hostname simply indicates they belong to the same tenant (namespace).

"Can This Cause Collisions?"
-----------------------------

**No, collisions are impossible.**

Because:

1. Kubernetes enforces unique names within a namespace
2. Each ReplicationSource gets a unique username
3. The combination is always unique
4. Kopia treats each identity separately

Advanced Scenarios
==================

Custom Identity Configuration
------------------------------

While the default behavior is recommended, you can customize:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: special-backup
     namespace: production
   spec:
     kopia:
       # Override the defaults
       username: "production-tier-1"
       hostname: "datacenter-east.production"

When to use custom configuration:

- Integration with existing backup systems
- Specific organizational requirements
- Complex multi-region setups
- Legacy compatibility needs

.. note::
   **How Custom Identity Works**

   When you specify custom ``username`` or ``hostname`` values:

   1. They are set as ``KOPIA_OVERRIDE_USERNAME`` and ``KOPIA_OVERRIDE_HOSTNAME`` environment variables
   2. These variables are used with ``--override-username`` and ``--override-hostname`` flags during ``kopia repository connect``
   3. Once connected with the custom identity, all snapshots automatically use it
   4. The override flags do NOT exist for ``kopia snapshot create`` (removed in Kopia v0.6.0)

Repository Organization
-----------------------

With namespace-based hostnames, repositories organize naturally:

.. code-block:: text

   Repository Structure:
   ├── production/           # All production namespace backups
   │   ├── webapp-backup/    # Webapp snapshots
   │   ├── db-backup/        # Database snapshots
   │   └── cache-backup/     # Cache snapshots
   ├── staging/              # All staging namespace backups
   │   ├── webapp-backup/    # Staging webapp snapshots
   │   └── db-backup/        # Staging database snapshots
   └── development/          # All development namespace backups
       └── app-backup/       # Dev app snapshots

Cross-Namespace Restore
------------------------

The predictable hostname makes cross-namespace restore simple:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-prod-to-staging
     namespace: staging
   spec:
     kopia:
       sourceIdentity:
         sourceName: webapp-backup
         sourceNamespace: production  # Hostname will be "production"
       # Restores from webapp-backup@production

Best Practices
==============

Naming Conventions
------------------

**Recommended naming patterns**:

1. **Descriptive names**: ``webapp-backup``, ``database-backup``
2. **Include backup frequency**: ``webapp-hourly``, ``database-daily``
3. **Indicate data type**: ``postgres-backup``, ``redis-backup``
4. **Consistent across environments**: Same names in dev/staging/prod

Repository Access Control
-------------------------

**Leverage namespace-based hostnames for access control**:

.. code-block:: yaml

   # Repository policy example
   retentionPolicy:
     production:    # Hostname-based policy
       daily: 30
       weekly: 12
       monthly: 6
     staging:       # Different policy for staging
       daily: 7
       weekly: 4
     development:   # Minimal retention for dev
       daily: 3

Monitoring and Alerting
-----------------------

**Monitor at the namespace level**:

.. code-block:: yaml

   # Alert on namespace-level backup failures
   alert: BackupFailure
   expr: |
     kopia_backup_failed{hostname="production"} > 0
   annotations:
     summary: "Production namespace backup failed"

Documentation
-------------

**Document your backup strategy**:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: backup-documentation
     namespace: volsync-system
   data:
     strategy.md: |
       # Backup Strategy
       
       ## Identity Scheme
       - Hostname: Always namespace name
       - Username: ReplicationSource name
       
       ## Namespaces
       - production: Critical production data
       - staging: Staging environment data
       - development: Development data
       
       ## Backup Sources
       - webapp-backup: Web application data
       - database-backup: PostgreSQL database
       - cache-backup: Redis cache data

Comparison with Other Approaches
=================================

Why Not Include PVC Names?
---------------------------

Other approaches might include PVC names in hostnames. VolSync doesn't because:

1. **Unnecessary Complexity**: PVC names don't add uniqueness
2. **Change Management**: PVC renames would affect backup identity
3. **Length Limitations**: Combined names might exceed limits
4. **Unclear Boundaries**: Mixes infrastructure (PVC) with logical (namespace) boundaries

Why Not Use Fully Qualified Names?
-----------------------------------

Some might expect ``namespace.cluster.local`` style hostnames. We use simple namespace names because:

1. **Simplicity**: Shorter, clearer identities
2. **Portability**: Same format across different clusters
3. **Kopia Compatibility**: Works better with Kopia's identity model
4. **User Experience**: Easier to type and remember

Migration and Compatibility
===========================

For Existing Users
------------------

If you have existing backups with different hostname formats:

1. **Continue using custom hostnames**: Explicitly set hostname to maintain compatibility
2. **Gradual migration**: Run old and new formats in parallel
3. **Document the transition**: Keep track of which sources use which format

For New Users
-------------

Start with the default behavior:

1. Let VolSync generate hostnames (namespace-only)
2. Let VolSync generate usernames (from object names)
3. Use custom configuration only when necessary
4. Document any customizations

Summary
=======

VolSync's hostname design is intentional and provides:

 **Guaranteed Uniqueness**: Leverages Kubernetes constraints
 **Clear Multi-Tenancy**: Namespace as tenant boundary
 **Predictable Behavior**: Always know what to expect
 **Simplified Management**: Easy to understand and operate
 **No Collision Risk**: Impossible to have identity conflicts
 **Kubernetes-Native**: Aligns with platform philosophy

This design makes VolSync's Kopia integration robust, predictable, and easy to manage in 
multi-tenant Kubernetes environments.

Key Takeaways
-------------

1. **Hostname = Namespace** (always, unless customized)
2. **Username = Object Name** (by default)
3. **Unique Identity Guaranteed** (Kubernetes enforces it)
4. **Multi-Tenancy Built-In** (namespace is the boundary)
5. **No Security Risk** (each source isolated)
6. **Intentional Design** (not a bug or limitation)

For more information, see:

- :doc:`multi-tenancy` - Detailed multi-tenancy documentation
- :doc:`backup-configuration` - Backup configuration guide
- :doc:`troubleshooting` - Troubleshooting guide