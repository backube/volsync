===================
Kopia-based backup
===================

.. toctree::
   :hidden:

   database_example
   backends
   multi-tenancy
   backup-configuration
   restore-configuration
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

Getting Started
===============

To get started with Kopia backups in VolSync, you'll need to configure a storage backend and create your first backup. Here's a quick overview of the main components:

**1. Configure Storage Backend**

First, set up a repository configuration secret for your chosen storage backend.
VolSync supports multiple storage options including S3, Azure Blob Storage, Google Cloud Storage,
and many others.

See :doc:`backends` for detailed configuration examples for all supported storage backends.

**2. Create Backup Configuration**

Configure a ReplicationSource to define your backup policy, including source PVC,
schedule, and backup options.

See :doc:`backup-configuration` for backup setup and configuration options.

**3. Set up Restore Operations**

When needed, configure a ReplicationDestination to restore data from your backups,
including point-in-time recovery, previous snapshot selection, and leveraging
enhanced error reporting for troubleshooting.

See :doc:`restore-configuration` for restore operations, enhanced error reporting,
and the ``sourceIdentity`` helper field with auto-discovery.

**4. Configure Multi-Tenancy (Optional)**

For shared environments, configure custom usernames and hostnames to organize
backups across different tenants or environments.

See :doc:`multi-tenancy` for multi-tenancy configuration and identity management.

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

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: mydata-restore
   spec:
     trigger:
       manual: restore-now
     kopia:
       repository: kopia-config
       destinationPVC: restored-data
       copyMethod: Direct
       # Use sourceIdentity to specify which backup source to restore from
       sourceIdentity:
         sourceName: mydata-backup
         sourceNamespace: default
         # sourcePVCName is optional - auto-discovered from ReplicationSource if not provided
       # Optionally use previous parameter to restore from older snapshots
       previous: 1  # Skip latest, use previous snapshot

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
   Complete guide to configuring all supported storage backends including S3,
   Azure Blob Storage, Google Cloud Storage, Backblaze B2, WebDAV, SFTP, Rclone,
   Google Drive, and more. Includes environment variables reference and troubleshooting.

:doc:`multi-tenancy`
   Comprehensive guide to multi-tenant setups, automatic username/hostname generation,
   namespace-based identity management, customization options, and troubleshooting.

:doc:`backup-configuration`
   Detailed backup setup including ReplicationSource configuration, scheduling,
   source path overrides, and backup options.

:doc:`restore-configuration`
   Complete restore operations guide including enhanced error reporting, snapshot discovery,
   ``sourceIdentity`` helper with auto-discovery, ``previous`` parameter, point-in-time recovery,
   and restore options.

:doc:`troubleshooting`
   Comprehensive troubleshooting guide covering enhanced error reporting, snapshot discovery,
   common error scenarios, identity mismatch issues, multi-tenant repository debugging,
   and systematic debugging approaches.

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