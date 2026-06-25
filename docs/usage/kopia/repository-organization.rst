=======================================
Repository Organization and Deduplication
=======================================

.. contents:: Repository Organization Guide
   :local:

This guide explains how to organize your Kopia repositories for optimal deduplication and storage efficiency. Understanding Kopia's deduplication capabilities will help you make informed decisions about repository structure that can significantly reduce storage costs.

Understanding Kopia's Deduplication
====================================

Kopia uses content-addressable storage with content-defined chunking to achieve excellent deduplication. This means that identical data blocks are stored only once, regardless of which PVC they come from or where they appear in the file tree.

How Deduplication Works
-----------------------

When Kopia backs up your data:

1. **Content Chunking**: Files are split into variable-sized chunks using a rolling hash algorithm. This means that if you insert data at the beginning of a file, only the new chunks need to be stored - the rest remain unchanged.

2. **Content Addressing**: Each chunk is identified by its cryptographic hash (SHA-256). If the same data appears anywhere in any backup, it has the same hash and is stored only once.

3. **Repository-Wide**: Deduplication happens across the entire repository. All PVCs backing up to the same repository share the same chunk storage pool.

4. **Automatic**: You don't need to configure anything special. Deduplication happens automatically whenever duplicate data is detected.

What Gets Deduplicated
-----------------------

Kopia's deduplication is particularly effective for:

**Common System Files**

- Operating system libraries and binaries shared across containers
- Base container image layers (Alpine, Ubuntu, etc.)
- Standard application frameworks and dependencies

**Application Data**

- Configuration files that are similar across environments
- Log files with repeated patterns
- Database dumps with common schemas
- Repeated documents or templates

**Incremental Backups**

- Only changed blocks are stored between snapshots
- File moves and renames are handled efficiently
- Small changes to large files only store the changed chunks

**Real-World Example**

Imagine you have three PVCs, each containing a WordPress installation:

- **Without deduplication** (separate repositories): Each backup stores the complete WordPress files, PHP libraries, and plugins independently. Total: ~150MB × 3 = 450MB
- **With deduplication** (shared repository): Common WordPress core files, PHP libraries, and identical plugins are stored once. Only unique themes, uploads, and configuration differ. Total: ~150MB + 30MB + 30MB = 210MB

**Storage savings: 53%** - and this is before compression!

The Single Repository Advantage
================================

Using a single Kopia repository for all your PVCs is the recommended approach that maximizes deduplication benefits while maintaining security and isolation.

Recommended Configuration
-------------------------

**Single S3 Bucket, No Prefixes**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-shared-repo
     namespace: backup-system
   type: Opaque
   stringData:
     # Single repository for ALL PVCs - no path prefix
     KOPIA_REPOSITORY: s3://company-backups
     KOPIA_PASSWORD: secure-repository-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     # For MinIO or other S3-compatible storage
     AWS_S3_ENDPOINT: https://s3.example.com

.. important::
   Use the **bucket root** without any path prefixes. This is the key to maximum deduplication.

**Multiple PVCs Using the Same Repository**

.. code-block:: yaml

   # Application 1 - Web Frontend
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-data
     namespace: production
   spec:
     sourcePVC: webapp-storage
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-shared-repo
       # Automatic identity: webapp-data@production
   ---
   # Application 2 - Database
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-backup
     namespace: production
   spec:
     sourcePVC: postgres-data
     trigger:
       schedule: "0 3 * * *"
     kopia:
       repository: kopia-shared-repo
       # Automatic identity: database-backup@production
   ---
   # Application 3 - File Storage
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: shared-files
     namespace: production
   spec:
     sourcePVC: nfs-storage
     trigger:
       schedule: "0 4 * * *"
     kopia:
       repository: kopia-shared-repo
       # Automatic identity: shared-files@production

All three backups share the same repository and benefit from deduplication, yet each maintains its own snapshot history and identity.

How Identity Ensures Isolation
-------------------------------

Even though all PVCs share the same repository, Kopia maintains complete isolation between them through unique identities:

**Automatic Identity Generation**

Each ReplicationSource automatically gets a unique identity based on:

- **Username**: Derived from the ReplicationSource name (e.g., ``webapp-data``)
- **Hostname**: Set to the namespace (e.g., ``production``)
- **Combined Identity**: ``webapp-data@production``

**Security Guarantees**

- **Separate snapshots**: Each identity has its own snapshot history
- **No data leakage**: One client cannot see or restore another client's snapshots
- **Independent retention**: Each identity can have different retention policies
- **Concurrent access**: Multiple clients can write to the repository simultaneously

For detailed information about identity management, see :doc:`multi-tenancy` and :doc:`hostname-design`.

Benefits Summary
----------------

Using a single repository provides:

**Storage Efficiency**

- **50-80% reduction** in storage usage is common for similar workloads
- Deduplication across all PVCs, not just within each PVC
- Lower cloud storage costs
- Reduced backup windows due to less data transfer

**Operational Simplicity**

- One repository to monitor and maintain
- Single maintenance schedule for the entire backup infrastructure
- Unified repository policies
- Simplified capacity planning

**Performance**

- Kopia efficiently handles thousands of clients in a single repository
- Shared cache benefits all backup operations
- Concurrent access without conflicts
- No synchronization overhead between repositories

**Cost Optimization**

For a real-world example with 10 PVCs, each containing similar application stacks:

- **Separate repositories**: 10 × 100GB = 1000GB
- **Single repository with deduplication**: ~400GB (60% savings)
- **Monthly savings** (at $0.023/GB for S3): $13.80/month
- **Annual savings**: $165.60/year

The savings scale with the number of PVCs and similarity of data.

When to Use Alternative Organizations
======================================

While a single repository is recommended, there are valid scenarios where you might need different repository structures.

Using S3 Prefixes
-----------------

If organizational requirements dictate logical separation within a single bucket, you can use S3 path prefixes. However, this comes with trade-offs.

**When Prefixes Make Sense**

- **Compliance requirements**: Regulations mandate separation (HIPAA, PCI-DSS, GDPR)
- **Organizational boundaries**: Different departments with separate budgets
- **Billing separation**: Need to track storage costs per team or project
- **Access control**: Different teams need different S3 bucket policies
- **Gradual migration**: Transitioning from separate repositories

Prefix Configuration
--------------------

**Important: Prefix Format**

Kopia requires S3 prefixes to be treated as directories, which requires a trailing slash.
VolSync automatically normalizes prefixes to ensure they always have a trailing slash,
and collapses multiple consecutive slashes for consistency.

**You can specify prefixes in any format** - VolSync will normalize them:

- ``s3://bucket/finance`` → normalized to ``s3://bucket/finance/``
- ``s3://bucket/finance/`` → already correct, unchanged
- ``s3://bucket/finance//`` → normalized to ``s3://bucket/finance/``
- ``s3://bucket/a//b///c`` → normalized to ``s3://bucket/a/b/c/``

**Prefix Configurations (all formats work)**

.. code-block:: yaml

   # Application 1 - Finance Department
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-finance
     namespace: finance
   type: Opaque
   stringData:
     # Any format works - trailing slash is added automatically
     KOPIA_REPOSITORY: s3://company-backups/finance
     # Or explicitly with slash: s3://company-backups/finance/
     KOPIA_PASSWORD: finance-repo-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
   ---
   # Application 2 - HR Department
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-hr
     namespace: hr
   type: Opaque
   stringData:
     # Different prefix, same bucket
     KOPIA_REPOSITORY: s3://company-backups/hr
     KOPIA_PASSWORD: hr-repo-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY

.. important::
   **Automatic Slash Normalization**

   VolSync automatically ensures all S3 prefixes have a trailing slash to treat them
   as directories. Per Kopia's documentation: *"Put trailing slash (/) if you want to
   use prefix as directory."*

   Without the trailing slash, Kopia would concatenate the prefix with repository files,
   creating incorrect paths like ``financekopia.repository`` instead of
   ``finance/kopia.repository``.

   **This normalization happens automatically** - you don't need to worry about trailing
   slashes in your configuration. The system handles it for you and logs the normalization
   during repository connection.

**Common Prefix Patterns**

.. code-block:: yaml

   # By department
   KOPIA_REPOSITORY: s3://backups/finance
   KOPIA_REPOSITORY: s3://backups/engineering
   KOPIA_REPOSITORY: s3://backups/operations

   # By environment
   KOPIA_REPOSITORY: s3://backups/production
   KOPIA_REPOSITORY: s3://backups/staging
   KOPIA_REPOSITORY: s3://backups/development

   # By application
   KOPIA_REPOSITORY: s3://backups/webapp
   KOPIA_REPOSITORY: s3://backups/database
   KOPIA_REPOSITORY: s3://backups/cache

   # Nested structure
   KOPIA_REPOSITORY: s3://backups/production/webapp
   KOPIA_REPOSITORY: s3://backups/production/database
   KOPIA_REPOSITORY: s3://backups/staging/webapp

Understanding the Trade-Offs
-----------------------------

**What You Lose**

Using S3 prefixes creates separate repositories, which means:

- **No cross-prefix deduplication**: Duplicate data between ``s3://bucket/app1`` and ``s3://bucket/app2`` is stored twice
- **Higher storage costs**: Each prefix stores its own complete chunk pool
- **Multiple maintenance operations**: Each prefix requires separate maintenance
- **No shared cache**: Benefits of shared repository cache are lost

**What You Gain**

- **Clear organizational boundaries**: Easy to see which team owns which data
- **Independent lifecycle**: Delete or archive one department's data without affecting others
- **Billing clarity**: S3 cost reports can break down storage by prefix
- **Access control**: Apply different IAM policies to different prefixes
- **Compliance**: Meet separation requirements for regulated data

**Quantifying the Cost**

Consider two applications with 80% data overlap:

- **Single repository**: 100GB (base) + 20GB (unique) = 120GB
- **Separate prefixes**: 100GB + 100GB = 200GB
- **Extra cost**: 80GB × $0.023/GB = $1.84/month per duplicate application

For 10 similar applications, this could mean hundreds of dollars per month in unnecessary storage costs.

Common Mistakes to Avoid
------------------------

**Mistake 1: Adding Prefixes "Just in Case"**

.. code-block:: yaml

   # Don't do this without a good reason!
   KOPIA_REPOSITORY: s3://backups/app1
   KOPIA_REPOSITORY: s3://backups/app2

If you don't have a specific compliance or organizational requirement, use the bucket root:

.. code-block:: yaml

   # Better - maximizes deduplication
   KOPIA_REPOSITORY: s3://backups

**Mistake 2: Per-PVC Prefixes**

.. code-block:: yaml

   # This destroys deduplication benefits
   KOPIA_REPOSITORY: s3://backups/webapp-pvc
   KOPIA_REPOSITORY: s3://backups/database-pvc
   KOPIA_REPOSITORY: s3://backups/cache-pvc

Each PVC already has a unique identity in Kopia. Prefixes are unnecessary and costly.

**Mistake 3: Inconsistent Prefix Usage**

.. code-block:: yaml

   # Mixing prefixes and non-prefixed repositories
   KOPIA_REPOSITORY: s3://backups          # App 1
   KOPIA_REPOSITORY: s3://backups/special  # App 2
   KOPIA_REPOSITORY: s3://backups/test     # App 3

This creates confusion and reduces deduplication. Choose one approach and be consistent.

Using Multiple Buckets
-----------------------

In some cases, you might use completely separate S3 buckets:

**When Multiple Buckets Make Sense**

- **Geographic distribution**: US bucket, EU bucket, APAC bucket for data residency
- **Security levels**: High-security data in a locked-down bucket, general data in standard bucket
- **Storage tiers**: Hot data in one bucket, cold archive data in Glacier bucket
- **Different cloud providers**: AWS bucket, Azure container, GCS bucket

**Configuration Example**

.. code-block:: yaml

   # US Production Data
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-us-production
   stringData:
     KOPIA_REPOSITORY: s3://backups-us-prod
     AWS_REGION: us-east-1
     # ... credentials
   ---
   # EU Production Data (GDPR compliance)
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-eu-production
   stringData:
     KOPIA_REPOSITORY: s3://backups-eu-prod
     AWS_REGION: eu-west-1
     # ... credentials

This approach has the same deduplication limitations as using prefixes, but may be necessary for regulatory or architectural reasons.

Migration Scenarios
===================

Moving Between Repository Structures
-------------------------------------

**Migrating from Prefixes to Single Repository**

If you started with prefixes and want to consolidate for better deduplication:

1. **Create the new single repository secret**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-unified
   stringData:
     KOPIA_REPOSITORY: s3://new-unified-backups
     KOPIA_PASSWORD: new-repo-password
     # ... credentials

2. **Update ReplicationSources gradually**

Start with non-critical PVCs to verify the configuration:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: test-app
   spec:
     kopia:
       repository: kopia-unified  # Changed from kopia-finance
       # Identity is automatically maintained

3. **Run initial backup**

The first backup to the new repository will be a full backup, but subsequent backups will deduplicate against all other data in the unified repository.

4. **Monitor storage usage**

.. code-block:: bash

   # Watch repository grow and observe deduplication
   kubectl logs -f <mover-pod-name>

5. **Migrate remaining PVCs**

Once confident, update all ReplicationSources to use the unified repository.

6. **Clean up old repositories**

After verifying backups and performing test restores from the new repository, you can delete the old prefixed repositories.

.. warning::
   **Data Migration Note**

   There is no automatic way to migrate existing snapshots from one repository to another
   while preserving snapshot history. When you change repositories, you start with a fresh
   backup history. Plan for:

   - Initial full backups to the new repository
   - Retention of old repository until retention periods expire
   - Higher storage usage temporarily while both repositories exist

**Migrating from Single Repository to Prefixes**

If compliance requirements force you to separate data:

1. **Create new prefix-based secrets**

.. code-block:: yaml

   # One secret per prefix/department
   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-finance-only
   stringData:
     KOPIA_REPOSITORY: s3://backups/finance
     KOPIA_PASSWORD: finance-password
     # ... credentials

2. **Update affected ReplicationSources**

.. code-block:: yaml

   # Change repository reference
   spec:
     kopia:
       repository: kopia-finance-only  # Changed from kopia-shared

3. **Accept storage cost increase**

The first backup to each prefixed repository will be a full backup. Monitor storage costs carefully as deduplication benefits are lost.

Best Practices Summary
======================

Repository Organization Decision Tree
--------------------------------------

Use this flowchart to decide on your repository structure:

1. **Do you have compliance requirements for data separation?**

   - **Yes** → Use separate buckets or prefixes per compliance boundary
   - **No** → Continue to question 2

2. **Do you need separate billing or cost tracking?**

   - **Yes** → Use S3 prefixes with cost allocation tags
   - **No** → Continue to question 3

3. **Do different teams need different access controls?**

   - **Yes** → Use S3 prefixes with IAM policies
   - **No** → Continue to question 4

4. **Are you backing up similar workloads?**

   - **Yes** → Use a single repository (maximum deduplication)
   - **No** → Single repository still works, but benefits are smaller

**Recommended Default: Single Repository**

Unless you answered "yes" to questions 1-3, use a single S3 bucket without prefixes.

Configuration Checklist
-----------------------

**For Single Repository (Recommended)**

.. code-block:: yaml

   ✓ Use bucket root: s3://my-backups
   ✓ No path prefixes
   ✓ Same secret shared across namespaces (if appropriate)
   ✓ Let automatic identity generation handle isolation
   ✓ Single maintenance schedule for the repository

**For Prefixed Repositories (When Required)**

.. code-block:: yaml

   ✓ Specify prefixes in any format (slashes added automatically)
   ✓ Consistent prefix naming scheme
   ✓ Document the reason for separation
   ✓ Separate maintenance schedules per prefix
   ✓ Monitor storage costs per prefix
   ✗ Don't use per-PVC prefixes
   ✗ Don't mix prefixed and non-prefixed in same bucket

**For Multiple Buckets (When Necessary)**

.. code-block:: yaml

   ✓ Geographic or regulatory reasons documented
   ✓ Separate secrets per bucket
   ✓ Clear naming convention
   ✓ Independent maintenance schedules
   ✓ Cost tracking per bucket

Monitoring and Verification
----------------------------

**Check Deduplication Effectiveness**

While Kopia doesn't expose per-client deduplication stats, you can monitor overall repository efficiency:

.. code-block:: bash

   # Enable debug logging to see deduplication in action
   # Add to your repository secret:
   # KOPIA_LOG_LEVEL: "debug"

   # Watch backup logs
   kubectl logs -f <replicationsource-pod>

   # Look for messages like:
   # "Hashing file example.txt"
   # "Stored 50 blocks (1.2 MB)"
   # "Deduplicated 150 blocks (3.8 MB)"

**Monitor Storage Growth**

Track your S3 bucket size over time:

.. code-block:: bash

   # AWS CLI
   aws s3 ls s3://my-backups --recursive --summarize --human-readable

   # Check storage growth rate
   # Initial backup: 500GB
   # After 10 similar PVCs: 800GB (instead of 5000GB without deduplication)
   # Deduplication ratio: 84%

**Verify Repository Health**

Regular maintenance keeps the repository optimized:

.. code-block:: yaml

   # Configure KopiaMaintenance for repository optimization
   apiVersion: volsync.backube/v1alpha1
   kind: KopiaMaintenance
   metadata:
     name: repo-maintenance
   spec:
     repository: kopia-shared-repo
     trigger:
       schedule: "0 0 * * 0"  # Weekly on Sunday
     cachePVC: kopia-cache

See :doc:`kopiamaintenance` for detailed maintenance configuration.

Additional Resources
====================

- :doc:`backends` - Detailed S3 and other storage backend configuration
- :doc:`multi-tenancy` - Understanding identity management in shared repositories
- :doc:`hostname-design` - How VolSync ensures unique identities
- :doc:`backup-configuration` - Complete backup configuration options
- :doc:`kopiamaintenance` - Repository maintenance and optimization
- :doc:`troubleshooting` - Debugging repository connection and backup issues

For questions about Kopia's deduplication algorithm and performance characteristics, see the `official Kopia documentation <https://kopia.io/docs/advanced/architecture/>`_.
