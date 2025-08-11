===================
Kopia-based backup
===================

.. toctree::
   :hidden:

   database_example
   backends
   filesystem-destination
   hostname-design
   multi-tenancy
   backup-configuration
   restore-configuration
   enable_file_deletion
   cross-namespace-restore
   troubleshooting
   custom-ca

.. sidebar:: Contents

   .. contents:: Backing up using Kopia
      :local:

VolSync supports taking backups of PersistentVolume data using the Kopia-based
data mover. A ReplicationSource defines the backup policy (target, frequency,
and retention), while a ReplicationDestination is used for restores.

The Kopia mover is different than most of VolSync's other movers because it is
not meant for synchronizing data between clusters. This mover is specifically
designed for data backup with advanced features like compression, deduplication,
and concurrent access.

Kopia vs. Restic
=================

While both Kopia and Restic are backup tools supported by VolSync, Kopia offers
several advantages:

**Performance**: Kopia typically provides faster backup and restore operations
due to its efficient chunking algorithm and support for parallel uploads.

**Compression**: Kopia supports multiple compression algorithms (zstd, gzip, s2)
with zstd providing better compression ratios and speed compared to Restic's options.

**Concurrent Access**: Kopia safely supports multiple clients writing to the same
repository simultaneously, while Restic requires careful coordination to avoid
conflicts.

**Modern Architecture**: Kopia uses a more modern content-addressable storage
design that enables features like shallow clones and efficient incremental backups.

**Actions/Hooks**: Kopia provides built-in support for pre and post snapshot
actions, making it easier to ensure data consistency for applications like databases.

**Maintenance**: Kopia's maintenance operations (equivalent to Restic's prune)
are more efficient and can run concurrently with backups.

**Security**: The Kopia mover in VolSync supports enhanced security settings including
``readOnlyRootFilesystem: true`` in pod security contexts, with automatic adjustments
to handle Kopia's temporary file requirements during restore operations.

Getting Started
===============

To get started with Kopia backups in VolSync, you'll need to configure a storage backend and create your first backup. Here's a quick overview of the main components:

**1. Configure Storage Backend**

First, set up a repository configuration secret for your chosen storage backend.
VolSync supports multiple storage options including S3, Azure Blob Storage, Google Cloud Storage,
filesystem destinations via PVC, and many others.

See :doc:`backends` for detailed configuration examples for all supported remote storage backends,
or :doc:`filesystem-destination` for using PVCs as backup destinations.

**2. Create Backup Configuration**

Configure a ReplicationSource to define your backup policy, including source PVC,
schedule, and backup options.

See :doc:`backup-configuration` for backup setup and configuration options.

**3. Set up Restore Operations**

When needed, configure a ReplicationDestination to restore data from your backups.
Identity configuration is now OPTIONAL - VolSync automatically determines the identity
for simple same-namespace restores when the destination name matches the source name.

See :doc:`restore-configuration` for restore operations, automatic identity generation,
the ``sourceIdentity`` helper field for cross-namespace restores, and enhanced error reporting.

For cross-namespace restore scenarios including disaster recovery and environment cloning,
see :doc:`cross-namespace-restore`.

**4. Understand Identity Management**

VolSync automatically manages identity for you! When no identity is specified,
it generates a username from the destination name and namespace, and uses the
namespace as the hostname. This makes simple restores configuration-free.

See :doc:`hostname-design` for understanding the intentional hostname design,
and :doc:`multi-tenancy` for multi-tenancy configuration and customization options.

**5. Troubleshooting**

Leverage the enhanced error reporting and snapshot discovery features to quickly
identify and resolve issues.

See :doc:`troubleshooting` for comprehensive debugging guidance.

**6. Custom CA (If Needed)**

If using self-signed certificates, configure custom certificate authority settings.

See :doc:`custom-ca` for custom certificate authority configuration.

Quick Example
=============

Here's a complete example showing how to set up a basic Kopia backup:

**Step 1: Create repository secret**

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: s3://my-backup-bucket/kopia-backups
     KOPIA_PASSWORD: my-secure-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     AWS_S3_ENDPOINT: http://minio.minio.svc.cluster.local:9000

**Step 2: Create backup**

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     sourcePVC: mydata
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM
     kopia:
       repository: kopia-config

**Step 3: Restore when needed**

.. note::
   Identity configuration is now OPTIONAL! For simple same-namespace restores,
   just use the same name as your ReplicationSource.

.. code-block:: yaml

   # Simple restore - no identity configuration needed!
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: mydata-backup  # Same name as ReplicationSource
   spec:
     trigger:
       manual: restore-now
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       # No identity configuration needed!
       # Automatically uses:
       #   username: mydata-backup-default
       #   hostname: default

   ---
   # For cross-namespace or different-name restores, use sourceIdentity:
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-from-prod
     namespace: staging
   spec:
     trigger:
       manual: restore-now
     kopia:
       destinationPVC: staging-data
       copyMethod: Direct
       sourceIdentity:
         sourceName: mydata-backup
         sourceNamespace: production

**Step 4: Check status if issues occur**

.. code-block:: bash

   # Check restore status and available identities
   kubectl get replicationdestination mydata-restore -o yaml
   
   # View discovered identities if snapshots not found
   kubectl get replicationdestination mydata-restore -o json | jq '.status.kopia.availableIdentities'

Documentation Sections
======================

The Kopia documentation has been organized into focused sections for easier navigation:

:doc:`backends`
   Complete guide to configuring all supported remote storage backends including S3,
   Azure Blob Storage, Google Cloud Storage, Backblaze B2, WebDAV, SFTP, Rclone,
   Google Drive, and more. Includes environment variables reference and troubleshooting.

:doc:`filesystem-destination`
   Comprehensive guide to using PersistentVolumeClaims as filesystem-based backup
   destinations. Covers configuration, security, migration from remote storage,
   and use cases for local and network-attached storage.

:doc:`hostname-design`
   Detailed explanation of VolSync's intentional hostname design where hostname equals
   namespace. Covers the design philosophy, benefits, multi-tenancy model, and why
   this approach ensures unique identities without collision risks.

:doc:`multi-tenancy`
   Comprehensive guide to multi-tenant setups, automatic username/hostname generation,
   namespace-based identity management, customization options, and troubleshooting.

:doc:`backup-configuration`
   Detailed backup setup including ReplicationSource configuration, scheduling,
   sourcePath and sourcePathOverride features, and backup options.

:doc:`restore-configuration`
   Complete restore operations guide including enhanced error reporting, snapshot discovery,
   ``sourceIdentity`` helper with auto-discovery of PVC names, sourcePathOverride, and
   repository configurations, ``previous`` parameter, point-in-time recovery, and restore options.

:doc:`enable_file_deletion`
   Comprehensive guide to the ``enableFileDeletion`` feature for ReplicationDestinations.
   Controls whether destination directories are cleaned before restore to ensure exact
   snapshot matching. Covers use cases, behavior differences, migration guidance, and best practices.

:doc:`cross-namespace-restore`
   Comprehensive guide for restoring Kopia backups across namespaces. Covers disaster recovery,
   environment cloning, namespace migration, and testing procedures with detailed examples,
   security considerations, and troubleshooting steps.

:doc:`troubleshooting`
   Comprehensive troubleshooting guide covering enhanced error reporting, snapshot discovery,
   common error scenarios, identity mismatch issues, sourcePathOverride troubleshooting,
   multi-tenant repository debugging, and systematic debugging approaches.

:doc:`custom-ca`
   Instructions for configuring custom certificate authorities for self-signed
   certificates and secure connections.

:doc:`database_example`
   Step-by-step example of backing up a MySQL database using Kopia.

Additional Resources
===================

For advanced topics including policy configuration, performance tuning, and troubleshooting,
refer to the comprehensive sections in each documentation file. The modular organization
makes it easier to find specific information and maintain focus on the task at hand.