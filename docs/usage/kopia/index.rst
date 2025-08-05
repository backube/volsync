===================
Kopia-based backup
===================

.. toctree::
   :hidden:

   database_example

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

Specifying a repository
=======================

For both backup and restore operations, it is necessary to specify a backup
repository for Kopia. The repository and connection information are defined in
a ``kopia-config`` Secret.

Below is an example showing how to use a repository stored on an S3-compatible backend (Minio).

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     # The repository url
     KOPIA_REPOSITORY: s3://backup-bucket/my-backups
     # The repository encryption password
     KOPIA_PASSWORD: my-secure-kopia-password
     # S3 credentials
     AWS_ACCESS_KEY_ID: access
     AWS_SECRET_ACCESS_KEY: password
     # S3 endpoint (required for non-AWS S3)
     AWS_S3_ENDPOINT: http://minio.minio.svc.cluster.local:9000

This Secret will be referenced for both backup (ReplicationSource) and for
restore (ReplicationDestination). The key names in this configuration Secret
directly correspond to the environment variable names supported by Kopia.

.. note::
   When providing credentials for Google Cloud Storage, the
   ``GOOGLE_APPLICATION_CREDENTIALS`` key should contain the actual contents of
   the json credential file, not just the path to the file.

The path used in the ``KOPIA_REPOSITORY`` is the s3 bucket but can optionally
contain a folder name within the bucket as well. This can be useful
if multiple PVCs are to be backed up to the same S3 bucket.

**S3 Nested Path Configuration**

VolSync's Kopia mover supports complex nested paths within S3 buckets. When you specify a repository URL like ``s3://bucket/path1/path2/path3``, the mover automatically:

1. Extracts the bucket name (``bucket``)
2. Extracts the prefix path (``path1/path2/path3``)  
3. Configures Kopia to use the prefix appropriately

This enables sophisticated repository organization:

.. code-block:: yaml

  # Different applications using the same bucket
  KOPIA_REPOSITORY: s3://company-backups/production/database/mysql-primary
  KOPIA_REPOSITORY: s3://company-backups/production/database/postgresql-secondary
  KOPIA_REPOSITORY: s3://company-backups/staging/application/web-frontend

As an example one kopia-config secret could use:

.. code-block:: yaml

  KOPIA_REPOSITORY: s3://backup-bucket/pvc-1-backup

While another (saved in a separate kopia-config secret) could use:

.. code-block:: yaml

  KOPIA_REPOSITORY: s3://backup-bucket/pvc-2-backup

.. note::
   Unlike some other backup tools, Kopia supports multiple clients writing to
   the same repository path safely. However, for organizational purposes and
   test isolation, it's still recommended to use separate paths for different PVCs.

.. note::
   If necessary, the repository will be automatically initialized (i.e.,
   ``kopia repository create``) during the first backup.

Supported backends
------------------

Kopia supports various storage backends with their respective configuration formats:

S3-compatible storage (AWS S3, MinIO, etc.)
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: s3://my-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     # For non-AWS S3 (MinIO, etc.)
     AWS_S3_ENDPOINT: http://minio.example.com:9000
     # Optional: specify region
     AWS_REGION: us-west-2

**Alternative S3 Configuration**

You can also use the new Kopia-specific S3 environment variables for more explicit configuration:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: s3://my-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     # Kopia-specific S3 variables
     KOPIA_S3_BUCKET: my-bucket
     KOPIA_S3_ENDPOINT: minio.example.com:9000
     KOPIA_S3_DISABLE_TLS: "true"  # For HTTP endpoints
     # Standard AWS credentials
     AWS_ACCESS_KEY_ID: AKIAIOSFODNN7EXAMPLE
     AWS_SECRET_ACCESS_KEY: wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY
     AWS_DEFAULT_REGION: us-west-2

.. note::
   The ``KOPIA_S3_*`` variables provide more explicit control over S3 configuration and support nested repository paths. When using ``KOPIA_REPOSITORY: s3://my-bucket/path1/path2``, Kopia will automatically extract the prefix (``path1/path2``) and configure the repository accordingly.

Filesystem storage
~~~~~~~~~~~~~~~~~~

For local or network-attached storage:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: filesystem:///mnt/backups
     KOPIA_PASSWORD: my-secure-password

Google Cloud Storage
~~~~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: gcs://my-gcs-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     # Service account credentials (JSON content, not file path)
     GOOGLE_APPLICATION_CREDENTIALS: |
       {
         "type": "service_account",
         "project_id": "my-project",
         "private_key_id": "key-id",
         "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
         "client_email": "backup-service@my-project.iam.gserviceaccount.com",
         "client_id": "123456789",
         "auth_uri": "https://accounts.google.com/o/oauth2/auth",
         "token_uri": "https://oauth2.googleapis.com/token"
       }

**Alternative GCS Configuration**

You can also use the new Kopia-specific GCS environment variables:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: gcs://my-gcs-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     # Kopia-specific GCS variables
     KOPIA_GCS_BUCKET: my-gcs-bucket
     GOOGLE_PROJECT_ID: my-project
     # Service account credentials (JSON content, not file path)
     GOOGLE_APPLICATION_CREDENTIALS: |
       {
         "type": "service_account",
         "project_id": "my-project",
         "private_key_id": "key-id",
         "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
         "client_email": "backup-service@my-project.iam.gserviceaccount.com",
         "client_id": "123456789",
         "auth_uri": "https://accounts.google.com/o/oauth2/auth",
         "token_uri": "https://oauth2.googleapis.com/token"
       }

Azure Blob Storage
~~~~~~~~~~~~~~~~~~

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: azure://container/backups
     KOPIA_PASSWORD: my-secure-password
     # Standard Azure credentials
     AZURE_STORAGE_ACCOUNT: mystorageaccount
     AZURE_STORAGE_KEY: storage-key-here
     # Alternative: using SAS token
     # AZURE_STORAGE_SAS_TOKEN: sv=2020-08-04&ss=bfqt&srt=sco&sp=rwdlacupx&se=2021-01-01T00:00:00Z&st=2020-01-01T00:00:00Z&spr=https,http&sig=signature

**Alternative Azure Configuration**

You can also use the new Kopia-specific Azure environment variables:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: azure://container/backups
     KOPIA_PASSWORD: my-secure-password
     # Kopia-specific Azure variables
     KOPIA_AZURE_CONTAINER: container
     KOPIA_AZURE_STORAGE_ACCOUNT: mystorageaccount
     KOPIA_AZURE_STORAGE_KEY: storage-key-here
     # Optional: Azure endpoint suffix for non-public clouds
     AZURE_ENDPOINT_SUFFIX: core.windows.net
     # Optional: Account name and key (alternative naming)
     AZURE_ACCOUNT_NAME: mystorageaccount
     AZURE_ACCOUNT_KEY: storage-key-here
     # Optional: SAS token authentication
     AZURE_ACCOUNT_SAS: sv=2020-08-04&ss=bfqt&srt=sco&sp=rwdlacupx

Backblaze B2
~~~~~~~~~~~~

Backblaze B2 provides cost-effective cloud storage with simple integration. Use this backend when you need affordable offsite backup storage with good performance characteristics.

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: b2://my-backup-bucket/backups
     KOPIA_PASSWORD: my-secure-password
     # Backblaze B2 credentials
     B2_ACCOUNT_ID: 12345abcdef67890
     B2_APPLICATION_KEY: your-application-key-here
     # Optional: specify bucket name explicitly
     KOPIA_B2_BUCKET: my-backup-bucket

**Use Cases**

* **Cost-effective offsite backups** - B2's pricing structure is particularly attractive for backup workloads
* **Long-term retention** - Ideal for archives and compliance backups due to low storage costs
* **Multi-cloud strategy** - Alternative to AWS/Azure/GCS for geographic or vendor diversification

**Configuration Notes**

* The ``B2_ACCOUNT_ID`` is your master application key ID or restricted key ID
* Use restricted application keys for enhanced security in production environments
* The repository URL format supports nested paths: ``b2://bucket/path/to/backups``
* Bucket names must be globally unique across all Backblaze B2 accounts

**Troubleshooting**

* Verify credentials with the B2 CLI: ``b2 authorize-account <account-id> <application-key>``
* Ensure the bucket exists and the application key has read/write permissions
* Check that the application key hasn't expired or been revoked

WebDAV
~~~~~~

WebDAV provides HTTP-based access to remote filesystems. This backend is useful for backing up to network-attached storage devices, cloud storage services that support WebDAV, or custom WebDAV servers.

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: webdav://webdav.example.com/backups
     KOPIA_PASSWORD: my-secure-password
     # WebDAV server credentials
     WEBDAV_URL: https://webdav.example.com/remote.php/dav/files/username/
     WEBDAV_USERNAME: backup-user
     WEBDAV_PASSWORD: webdav-user-password

**Use Cases**

* **Network-attached storage** - Synology, QNAP, and other NAS devices with WebDAV support
* **Cloud storage services** - Nextcloud, ownCloud, Box, and other WebDAV-compatible services
* **Enterprise file servers** - Corporate file servers with WebDAV interface
* **Hybrid cloud scenarios** - On-premises storage with cloud accessibility

**Configuration Options**

.. code-block:: yaml

   stringData:
     KOPIA_REPOSITORY: webdav://webdav.example.com/backups
     KOPIA_PASSWORD: my-secure-password
     # Full WebDAV endpoint URL (required)
     WEBDAV_URL: https://webdav.example.com/remote.php/dav/files/username/
     WEBDAV_USERNAME: backup-user
     WEBDAV_PASSWORD: webdav-user-password
     # For HTTP-only endpoints (not recommended for production)
     # WEBDAV_URL: http://internal-webdav.company.com/dav/

**Security Considerations**

* Always use HTTPS endpoints for production environments to protect credentials
* Consider using application-specific passwords rather than main account passwords
* Implement proper TLS certificate validation for WebDAV servers
* Use network policies to restrict access to WebDAV endpoints from within the cluster

**Troubleshooting**

* Test WebDAV connectivity: ``curl -u username:password -X PROPFIND https://webdav.example.com/path/``
* Verify the WebDAV URL includes the correct path and protocol
* Check server logs for authentication or permission errors
* Ensure the WebDAV server supports the required HTTP methods (GET, PUT, DELETE, PROPFIND)

SFTP
~~~~

SFTP (SSH File Transfer Protocol) provides secure file transfer over SSH connections. This backend is ideal for backing up to remote servers, VPS instances, or any system with SSH access.

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: sftp://backup-server.example.com/backups
     KOPIA_PASSWORD: my-secure-password
     # SFTP server connection details
     SFTP_HOST: backup-server.example.com
     SFTP_PORT: "22"
     SFTP_USERNAME: backup-user
     SFTP_PASSWORD: ssh-user-password
     SFTP_PATH: /home/backup-user/kopia-backups

**SSH Key Authentication**

For enhanced security, use SSH key authentication instead of password authentication:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: sftp://backup-server.example.com/backups
     KOPIA_PASSWORD: my-secure-password
     # SFTP server connection details
     SFTP_HOST: backup-server.example.com
     SFTP_PORT: "22"
     SFTP_USERNAME: backup-user
     SFTP_PATH: /home/backup-user/kopia-backups
     # SSH private key content (alternative to password)
     SFTP_KEY_FILE: |
       -----BEGIN OPENSSH PRIVATE KEY-----
       b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAAAFwAAAAdzc2gtcn
       ...
       -----END OPENSSH PRIVATE KEY-----

**Use Cases**

* **Remote server backups** - VPS, dedicated servers, or cloud instances with SSH access
* **On-premises infrastructure** - Backup to internal servers or appliances
* **Secure file transfer** - Leveraging SSH's built-in encryption and authentication
* **Legacy system integration** - Connect to older systems that support SFTP but not modern cloud APIs

**Configuration Notes**

* The ``SFTP_PORT`` defaults to 22 if not specified
* The ``SFTP_PATH`` should be an absolute path on the remote server
* SSH key authentication is preferred over password authentication for security
* The repository URL format: ``sftp://hostname/path`` or ``sftp://hostname:port/path``

**SSH Key Management**

1. Generate an SSH key pair on your client system:
   
   .. code-block:: console

      $ ssh-keygen -t ed25519 -f kopia-backup-key -C "kopia-backup@cluster"

2. Add the public key to the remote server's ``~/.ssh/authorized_keys``

3. Include the private key content in the ``SFTP_KEY_FILE`` field

**Troubleshooting**

* Test SSH connectivity: ``ssh -p 22 backup-user@backup-server.example.com``
* Verify the remote path exists and is writable by the backup user
* Check SSH server logs for authentication failures
* Ensure SSH key format is correct (PEM format, not OpenSSH format for some versions)
* Verify firewall rules allow SSH traffic on the specified port

Rclone
~~~~~~

Rclone provides access to over 40 different cloud storage providers through a unified interface. This backend enables backing up to virtually any cloud storage service supported by Rclone.

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: rclone://remote-name:/backups
     KOPIA_PASSWORD: my-secure-password
     # Rclone configuration
     RCLONE_REMOTE_PATH: remote-name:/backups
     # Optional: specify rclone executable path
     RCLONE_EXE: /usr/local/bin/rclone
     # Rclone configuration content
     RCLONE_CONFIG: |
       [remote-name]
       type = s3
       provider = AWS
       access_key_id = YOUR_ACCESS_KEY
       secret_access_key = YOUR_SECRET_KEY
       region = us-west-2
       
       [dropbox-remote]
       type = dropbox
       token = {"access_token":"...","token_type":"bearer",...}

**Supported Cloud Providers**

Rclone supports numerous cloud storage services including:

* **Major cloud providers**: AWS S3, Azure Blob, Google Cloud Storage, Google Drive
* **File hosting services**: Dropbox, OneDrive, Box, pCloud
* **Object storage**: Backblaze B2, Wasabi, DigitalOcean Spaces
* **FTP/SFTP**: Any FTP, SFTP, or WebDAV server
* **Local/Network storage**: Local filesystem, SMB/CIFS shares

**Use Cases**

* **Multi-cloud strategy** - Single interface for multiple cloud providers
* **Provider-specific features** - Access specialized features of different cloud services
* **Migration scenarios** - Easy switching between different storage providers
* **Complex routing** - Chain multiple storage backends or use advanced Rclone features

**Advanced Configuration Examples**

**Google Drive via Rclone**:

.. code-block:: yaml

   stringData:
     KOPIA_REPOSITORY: rclone://gdrive:/kopia-backups
     RCLONE_REMOTE_PATH: gdrive:/kopia-backups
     RCLONE_CONFIG: |
       [gdrive]
       type = drive
       scope = drive
       token = {"access_token":"ya29.a0...","token_type":"Bearer",...}
       team_drive = 

**Multiple Remotes Setup**:

.. code-block:: yaml

   stringData:
     KOPIA_REPOSITORY: rclone://primary:/backups
     RCLONE_REMOTE_PATH: primary:/backups
     RCLONE_CONFIG: |
       [primary]
       type = s3
       provider = AWS
       access_key_id = PRIMARY_KEY
       secret_access_key = PRIMARY_SECRET
       region = us-west-2
       
       [backup]
       type = b2
       account = BACKBLAZE_ACCOUNT_ID
       key = BACKBLAZE_APPLICATION_KEY

**Performance Considerations**

* Rclone performance varies significantly between providers
* Some providers support parallel uploads, others perform better with sequential operations
* Consider using Rclone's caching features for frequently accessed data
* Network latency to the storage provider affects backup and restore speeds

**Troubleshooting**

* Test Rclone configuration: ``rclone ls remote-name:`` using the same config
* Verify the remote name matches exactly between ``RCLONE_REMOTE_PATH`` and ``RCLONE_CONFIG``
* Check Rclone logs for authentication or connectivity issues
* Ensure the Rclone executable is available in the container (``RCLONE_EXE`` if custom path)
* Validate JSON tokens in the configuration for OAuth-based providers

Google Drive
~~~~~~~~~~~~

Google Drive provides direct integration with Google's consumer and enterprise file storage service. This backend is particularly useful for organizations already using Google Workspace or for personal backup scenarios.

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: kopia-config
   type: Opaque
   stringData:
     KOPIA_REPOSITORY: gdrive://folder-id
     KOPIA_PASSWORD: my-secure-password
     # Google Drive folder ID (required)
     GOOGLE_DRIVE_FOLDER_ID: 1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms
     # OAuth2 credentials JSON content
     GOOGLE_DRIVE_CREDENTIALS: |
       {
         "type": "service_account",
         "project_id": "my-backup-project",
         "private_key_id": "key-id-here",
         "private_key": "-----BEGIN PRIVATE KEY-----\n...\n-----END PRIVATE KEY-----\n",
         "client_email": "backup-service@my-backup-project.iam.gserviceaccount.com",
         "client_id": "123456789012345678901",
         "auth_uri": "https://accounts.google.com/o/oauth2/auth",
         "token_uri": "https://oauth2.googleapis.com/token",
         "auth_provider_x509_cert_url": "https://www.googleapis.com/oauth2/v1/certs",
         "client_x509_cert_url": "https://www.googleapis.com/robot/v1/metadata/x509/backup-service%40my-backup-project.iam.gserviceaccount.com"
       }

**Setting up Google Drive Access**

1. **Create a Google Cloud Project**:
   
   * Go to the Google Cloud Console
   * Create a new project or select an existing one
   * Enable the Google Drive API

2. **Create Service Account Credentials**:
   
   * Navigate to "Credentials" in the Google Cloud Console
   * Create a new service account
   * Generate and download the JSON key file
   * Use the JSON content as the ``GOOGLE_DRIVE_CREDENTIALS`` value

3. **Share the Google Drive Folder**:
   
   * Create a folder in Google Drive for backups
   * Share the folder with the service account email address
   * Grant "Editor" permissions to allow read/write access
   * Copy the folder ID from the Google Drive URL

**Finding the Folder ID**

The Google Drive folder ID can be found in the URL when viewing the folder:

.. code-block:: console

   # Google Drive folder URL:
   https://drive.google.com/drive/folders/1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms
   
   # The folder ID is:
   1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms

**Use Cases**

* **Google Workspace integration** - Seamless backup for organizations using Google Workspace
* **Personal backups** - Easy setup for individual users with Google accounts
* **Collaboration scenarios** - Shared backup folders with team access controls
* **Cross-platform access** - Backups accessible through Google Drive web interface and apps

**Google Workspace vs Personal Accounts**

**Google Workspace (Enterprise)**:

* Higher storage quotas and better performance
* Advanced sharing and permission controls
* Organization-level security policies
* Better support for service accounts

**Personal Google Accounts**:

* 15GB free storage (shared across Google services)
* OAuth2 user credentials instead of service accounts
* Limited API quotas and rate limits
* Suitable for personal or small-scale backups

**OAuth2 User Credentials (Alternative)**

For personal Google accounts, you can use OAuth2 user credentials instead of service accounts:

.. code-block:: yaml
   
   stringData:
     KOPIA_REPOSITORY: gdrive://folder-id
     GOOGLE_DRIVE_FOLDER_ID: 1BxiMVs0XRA5nFMdKvBdBZjgmUUqptlbs74OgvE2upms
     GOOGLE_DRIVE_CREDENTIALS: |
       {
         "client_id": "123456789.apps.googleusercontent.com",
         "client_secret": "your-client-secret",
         "refresh_token": "1//04...",
         "type": "authorized_user"
       }

**Performance and Limitations**

* Google Drive API has rate limits that may affect large backup operations
* File size limits: 5TB per file for Google Workspace, 750GB for personal accounts
* Concurrent upload limits may require tuning ``parallelism`` settings
* Consider using Google Cloud Storage instead for high-performance backup scenarios

**Troubleshooting**

* Verify service account has access to the specified folder
* Check that the Google Drive API is enabled in your Google Cloud project
* Ensure the folder ID is correct and the folder exists
* Validate the JSON credentials format and that the private key is properly escaped
* Monitor API quotas in the Google Cloud Console for rate limiting issues
* Test access using the Google Drive API explorer or Google Cloud SDK

Environment Variables Reference
-------------------------------

VolSync's Kopia mover supports a comprehensive set of environment variables for configuring different storage backends and repository settings:

**Core Kopia Variables**

``KOPIA_REPOSITORY``
   The repository URL specifying the storage backend and path (required)

``KOPIA_PASSWORD``
   The repository encryption password (required)

**S3-Compatible Storage Variables**

``AWS_ACCESS_KEY_ID``, ``AWS_SECRET_ACCESS_KEY``
   Standard AWS S3 credentials

``AWS_S3_ENDPOINT``
   S3 endpoint URL for non-AWS S3 services

``AWS_DEFAULT_REGION``, ``AWS_REGION``
   AWS region for the S3 bucket

``AWS_PROFILE``
   AWS profile to use for authentication

``KOPIA_S3_BUCKET``
   S3 bucket name (alternative to extracting from KOPIA_REPOSITORY)

``KOPIA_S3_ENDPOINT``
   S3 endpoint hostname and port (alternative to AWS_S3_ENDPOINT)

``KOPIA_S3_DISABLE_TLS``
   Set to "true" to disable TLS for HTTP-only S3 endpoints

**Azure Blob Storage Variables**

``AZURE_STORAGE_ACCOUNT``, ``KOPIA_AZURE_STORAGE_ACCOUNT``
   Azure storage account name

``AZURE_STORAGE_KEY``, ``KOPIA_AZURE_STORAGE_KEY``
   Azure storage account key

``AZURE_STORAGE_SAS_TOKEN``
   Azure SAS token for authentication

``AZURE_ACCOUNT_NAME``, ``AZURE_ACCOUNT_KEY``, ``AZURE_ACCOUNT_SAS``
   Alternative Azure credential variable names

``AZURE_ENDPOINT_SUFFIX``
   Azure endpoint suffix for non-public clouds

``KOPIA_AZURE_CONTAINER``
   Azure blob container name

**Google Cloud Storage Variables**

``GOOGLE_APPLICATION_CREDENTIALS``
   Google service account credentials (JSON content)

``GOOGLE_PROJECT_ID``
   Google Cloud project ID

``KOPIA_GCS_BUCKET``
   GCS bucket name

**Filesystem Storage Variables**

``KOPIA_FS_PATH``
   Filesystem path for local or network-attached storage repositories

**Backblaze B2 Variables**

``B2_ACCOUNT_ID``
   Backblaze B2 account ID (master or restricted application key ID)

``B2_APPLICATION_KEY``
   Backblaze B2 application key

``KOPIA_B2_BUCKET``
   B2 bucket name (alternative to extracting from KOPIA_REPOSITORY)

**WebDAV Variables**

``WEBDAV_URL``
   WebDAV server endpoint URL (required)

``WEBDAV_USERNAME``
   Username for WebDAV authentication

``WEBDAV_PASSWORD``
   Password for WebDAV authentication

**SFTP Variables**

``SFTP_HOST``
   SFTP server hostname or IP address

``SFTP_PORT``
   SFTP server port (defaults to 22 if not specified)

``SFTP_USERNAME``
   Username for SFTP authentication

``SFTP_PASSWORD``
   Password for SFTP authentication (alternative to key authentication)

``SFTP_PATH``
   Remote path on the SFTP server for backup storage

``SFTP_KEY_FILE``
   SSH private key content for key-based authentication (alternative to password)

**Rclone Variables**

``RCLONE_REMOTE_PATH``
   Rclone remote path specification (format: remote-name:/path)

``RCLONE_EXE``
   Path to the Rclone executable (optional, defaults to system rclone)

``RCLONE_CONFIG``
   Complete Rclone configuration file content

**Google Drive Variables**

``GOOGLE_DRIVE_FOLDER_ID``
   Google Drive folder ID where backups will be stored

``GOOGLE_DRIVE_CREDENTIALS``
   OAuth2 credentials JSON content (service account or user credentials)

.. note::
   Environment variables are displayed securely in mover logs as ``[SET]`` or ``[NOT SET]`` to prevent credential exposure while providing configuration visibility for troubleshooting.

Configuring backup
==================

A backup policy is defined by a ReplicationSource object that uses the Kopia
replication method.

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup
   spec:
     # The PVC to be backed up
     sourcePVC: mydata
     trigger:
       # Take a backup every 30 minutes
       schedule: "*/30 * * * *"
     kopia:
       # Run maintenance (garbage collection, etc.) every 7 days
       maintenanceIntervalDays: 7
       # Name of the Secret with the connection information
       repository: kopia-config
       # Retention policy for backups
       retain:
         hourly: 6
         daily: 7
         weekly: 4
         monthly: 6
         yearly: 2
       # Compression algorithm (zstd, gzip, s2, none)
       compression: zstd
       # Number of parallel upload streams
       parallelism: 2
       # Clone the source volume prior to taking a backup to ensure a
       # point-in-time image.
       copyMethod: Clone
       # The StorageClass to use when creating the PiT copy (same as source PVC if omitted)
       #storageClassName: my-sc-name
       # The VSC to use if the copy method is Snapshot (default if omitted)
       #volumeSnapshotClassName: my-vsc-name
       # Override the source path name in snapshots (optional)
       #sourcePathOverride: /var/lib/postgresql/data

Backup options
--------------

There are a number of additional configuration options not shown in the above
example. VolSync's Kopia mover options closely follow those of Kopia itself.

.. include:: ../inc_src_opts.rst

actions
   This allows you to define pre and post snapshot actions (hooks) that will
   be executed before and after taking snapshots. This can be used to ensure
   data consistency by running database flush commands, application quiesce
   operations, etc.

   beforeSnapshot
      Command to run before taking a snapshot
   afterSnapshot
      Command to run after taking a snapshot

cacheCapacity
   This determines the size of the Kopia metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the repository metadata. The default is ``1 Gi``.
cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.
cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.
compression
   This specifies the compression algorithm to use. Options are:
   
   * ``zstd`` - Zstandard compression (recommended, default)
   * ``gzip`` - Gzip compression
   * ``s2`` - S2 compression (fast)
   * ``none`` - No compression

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

maintenanceIntervalDays
   This determines the number of days between running maintenance operations
   on the repository. Maintenance includes garbage collection, compaction,
   and other housekeeping tasks. Setting this option allows a trade-off
   between storage efficiency and access costs.
parallelism
   This specifies the number of parallel upload streams to use when backing up
   data. Higher values can improve performance on fast networks but may increase
   memory usage. The default is ``1``.
repository
   This is the name of the Secret (in the same Namespace) that holds the
   connection information for the backup repository.
retain
   This has sub-fields for ``hourly``, ``daily``, ``weekly``, ``monthly``, and
   ``yearly`` that allow setting the number of each type of backup to retain.

   When more than the specified number of backups are present in the repository,
   they will be removed during the next maintenance operation.
sourcePathOverride
   This optional field allows you to override the source path name that appears
   in Kopia snapshots. When specified, it must be an absolute path (starting with
   ``/``). This is useful for maintaining consistent snapshot naming when the actual
   filesystem path differs from the logical application path. See the
   :ref:`source-path-override` section for detailed usage examples.

.. _source-path-override:

Source Path Override
====================

The ``sourcePathOverride`` field provides a powerful way to control how your backup source paths appear in Kopia snapshots. This feature allows you to use a different path name in snapshots than the actual filesystem path where the data is stored, enabling better organization and consistency in your backup repository.

Purpose and Benefits
--------------------

By default, Kopia uses the actual mount point of your PVC as the source path in snapshots. However, there are many scenarios where you might want to override this behavior:

**Consistency Across Environments**: Maintain the same logical path across different clusters or environments, even when the underlying storage configuration differs.

**Application-Centric Naming**: Use paths that reflect the application's perspective rather than Kubernetes' internal mount points.

**Simplified Organization**: Create clean, predictable snapshot names that make backup management easier.

**Migration Support**: Preserve original application paths when migrating workloads between different storage systems.

Common Use Cases
----------------

Database Backups
~~~~~~~~~~~~~~~~~

Database applications often expect data to be located at specific standard paths. When backing up database data, you can preserve these logical paths regardless of where Kubernetes mounts the PVC:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: postgres-backup
   spec:
     sourcePVC: postgres-data
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       # Data is mounted at /data in the pod, but we want snapshots 
       # to show the standard PostgreSQL data directory path
       sourcePathOverride: /var/lib/postgresql/data
       retain:
         daily: 7
         weekly: 4
       copyMethod: Clone

In this example, even though the PVC might be mounted at ``/data`` inside the container, the Kopia snapshots will show the path as ``/var/lib/postgresql/data``, making it clear that this is PostgreSQL data and maintaining consistency with standard PostgreSQL installations.

Application Configuration Backups
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When backing up application configurations, you may want to preserve the logical application paths:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-config-backup
   spec:
     sourcePVC: app-config
     trigger:
       schedule: "0 1 * * *"
     kopia:
       repository: kopia-config
       # PVC mounted at /config, but we want to preserve the app's perspective
       sourcePathOverride: /opt/myapp/config
       retain:
         daily: 14
         monthly: 6
       copyMethod: Snapshot

This ensures that when you view snapshots or perform restores, the paths reflect where the application expects to find its configuration files.

Filesystem Snapshot Backups
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

When backing up data from storage system snapshots or temporary mounts, you can preserve the original filesystem paths:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: filesystem-backup
   spec:
     sourcePVC: snapshot-mount
     trigger:
       schedule: "0 3 * * *"
     kopia:
       repository: kopia-config
       # Backup from temporary snapshot mount but preserve original path
       sourcePathOverride: /home/users
       retain:
         daily: 30
         weekly: 12
       copyMethod: Direct

This is particularly useful when backing up data from storage system snapshots where the data is temporarily mounted for backup purposes, but you want to maintain the original filesystem structure in your backup repository.

Clean Snapshot Organization
~~~~~~~~~~~~~~~~~~~~~~~~~~~

Create well-organized backup repositories with predictable path structures:

.. code-block:: yaml

   # Application data backup
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-data-backup
   spec:
     sourcePVC: webapp-storage
     trigger:
       schedule: "*/6 * * * *"
     kopia:
       repository: kopia-config
       sourcePathOverride: /applications/webapp/data
       copyMethod: Clone

   ---
   # Log data backup  
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: webapp-logs-backup
   spec:
     sourcePVC: webapp-logs
     trigger:
       schedule: "0 4 * * *"
     kopia:
       repository: kopia-config
       sourcePathOverride: /applications/webapp/logs
       copyMethod: Direct

This approach creates a logical hierarchy in your backup repository that makes it easy to understand what each snapshot contains, regardless of the actual Kubernetes PVC mount points.

Integration with Multi-Tenancy
-------------------------------

The ``sourcePathOverride`` feature works seamlessly with Kopia's built-in multi-tenancy features, which use username and hostname to organize snapshots. VolSync automatically configures these based on the Kubernetes namespace and ReplicationSource name:

**Default Behavior** (without sourcePathOverride):
  Snapshots appear as: ``<namespace>@<replicationsource-name>:/actual/mount/path``

**With sourcePathOverride**:
  Snapshots appear as: ``<namespace>@<replicationsource-name>:/your/custom/path``

This provides excellent isolation and organization across multiple applications and namespaces while maintaining meaningful path names:

.. code-block:: yaml

   # Namespace: production
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mysql-primary
   spec:
     sourcePVC: mysql-data
     kopia:
       repository: kopia-config
       sourcePathOverride: /var/lib/mysql
       # Results in snapshots like: production@mysql-primary:/var/lib/mysql

   ---
   # Namespace: staging  
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mysql-primary
   spec:
     sourcePVC: mysql-data
     kopia:
       repository: kopia-config
       sourcePathOverride: /var/lib/mysql
       # Results in snapshots like: staging@mysql-primary:/var/lib/mysql

Both applications can use the same repository and the same logical path, but they remain completely isolated due to the namespace-based user identification.

Technical Implementation
------------------------

The ``sourcePathOverride`` feature is implemented using Kopia's ``--override-source`` flag, which provides native support for custom source paths. This ensures compatibility with all Kopia features and maintains the integrity of the backup repository.

**Key Technical Details**:

- Must be an absolute path (starting with ``/``)
- Only applies to ReplicationSource (backup operations)
- Not used for ReplicationDestination (restore operations use repository metadata)
- Compatible with all repository backends (S3, Azure, GCS, filesystem)
- Works with all copy methods (Direct, Clone, Snapshot)
- Integrates with Kopia policies and actions

Configuration Examples
-----------------------

Basic Path Override
~~~~~~~~~~~~~~~~~~~

Simple override for cleaner snapshot naming:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: data-backup
   spec:
     sourcePVC: application-data
     trigger:
       schedule: "0 */4 * * *"
     kopia:
       repository: kopia-config
       sourcePathOverride: /app/data
       retain:
         hourly: 6
         daily: 7

Multi-Application Environment
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Organize multiple applications in a single repository:

.. code-block:: yaml

   # Frontend application
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: frontend-backup
     namespace: web-tier
   spec:
     sourcePVC: frontend-data
     kopia:
       repository: shared-backup-config
       sourcePathOverride: /services/frontend/data

   ---
   # Backend API
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: api-backup
     namespace: api-tier
   spec:
     sourcePVC: api-data
     kopia:
       repository: shared-backup-config
       sourcePathOverride: /services/api/data

   ---
   # Database
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-backup
     namespace: data-tier
   spec:
     sourcePVC: postgres-data
     kopia:
       repository: shared-backup-config
       sourcePathOverride: /services/database/postgresql

This creates a well-organized repository structure where snapshots clearly indicate which service they belong to, making backup management much easier.

Path Override with Actions
~~~~~~~~~~~~~~~~~~~~~~~~~~

Combine path override with pre/post snapshot actions:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mysql-backup
   spec:
     sourcePVC: mysql-data
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       sourcePathOverride: /var/lib/mysql
       actions:
         beforeSnapshot: "mysqldump --single-transaction --all-databases > /var/lib/mysql/backup.sql"
         afterSnapshot: "rm -f /var/lib/mysql/backup.sql"
       retain:
         daily: 7
         weekly: 4

Advanced Configuration with Policies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Use path override with policy-based configuration:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: database-backup-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd"
         },
         "retention": {
           "keepDaily": 14,
           "keepWeekly": 8,
           "keepMonthly": 6
         },
         "files": {
           "ignore": [
             "*.log",
             "*.tmp",
             "mysql-bin.*"
           ]
         },
         "actions": {
           "beforeSnapshotRoot": {
             "script": "mysqldump --single-transaction --all-databases > /var/lib/mysql/full-backup.sql",
             "timeout": "15m",
             "mode": "essential"
           },
           "afterSnapshotRoot": {
             "script": "rm -f /var/lib/mysql/full-backup.sql",
             "timeout": "2m",
             "mode": "optional"
           }
         }
       }

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mysql-backup-with-policies
   spec:
     sourcePVC: mysql-data
     trigger:
       schedule: "0 2 * * *"
     kopia:
       repository: kopia-config
       sourcePathOverride: /var/lib/mysql
       policyConfig:
         configMapName: database-backup-policies
       copyMethod: Clone

Best Practices
--------------

**Use Meaningful Paths**: Choose paths that clearly indicate the type of data being backed up and its purpose. Use standard application paths when possible (e.g., ``/var/lib/postgresql/data`` for PostgreSQL).

**Maintain Consistency**: Use the same path override across all environments (development, staging, production) for the same application to ensure consistency.

**Consider Restoration**: While restore operations don't use the override path directly, having logical snapshot names makes it much easier to identify the correct snapshot to restore.

**Organize by Function**: Group related applications under common path prefixes (e.g., ``/services/frontend``, ``/services/backend``, ``/services/database``).

**Document Your Strategy**: Maintain documentation of your path override strategy so team members understand the organization scheme.

**Test Restore Scenarios**: Verify that your path override strategy doesn't complicate restore procedures, especially in disaster recovery scenarios.

Troubleshooting
---------------

**Invalid Path Format**

The most common issue is using relative paths instead of absolute paths:

.. code-block:: yaml

   # Incorrect - relative path
   sourcePathOverride: var/lib/mysql
   
   # Correct - absolute path
   sourcePathOverride: /var/lib/mysql

**Path Override Not Appearing**

If your path override doesn't appear in snapshots, verify:

1. The field is correctly specified in the ReplicationSource
2. The ReplicationSource is using the Kopia mover (not Restic or another mover)
3. Check the mover job logs for any override-related messages

**Snapshot Identification**

To verify that your path override is working, examine the Kopia repository:

.. code-block:: console

   # List snapshots to see the override paths
   $ kubectl exec -it <kopia-job-pod> -- kopia snapshot list
   
   # Look for your custom path in the snapshot listings
   $ kubectl logs <replicationsource-job-pod> | grep -i override

The path override feature provides powerful flexibility for organizing and managing your Kopia backups within VolSync, enabling you to create clean, consistent, and meaningful backup repositories regardless of the underlying Kubernetes storage configuration.

Performing a restore
====================

Data from a backup can be restored using the ReplicationDestination CR. In most
cases, it is desirable to perform a single restore into an empty
PersistentVolume.

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

The example, shown above, will restore the data from the most recent backup. To
restore an older version of the data, the ``shallow`` and ``restoreAsOf``
fields can be used. See below for more information on their meaning.

Restore options
---------------

There are a number of additional configuration options not shown in the above
example.

.. include:: ../inc_dst_opts.rst

cacheCapacity
   This determines the size of the Kopia metadata cache volume. This volume
   contains cached metadata from the backup repository. It must be large enough
   to hold the repository metadata. The default is ``1 Gi``.
cacheStorageClassName
   This is the name of the StorageClass that should be used when provisioning
   the cache volume. It defaults to ``.spec.storageClassName``, then to the name
   of the StorageClass used by the source PVC.
cacheAccessModes
   This is the access mode(s) that should be used to provision the cache volume.
   It defaults to ``.spec.accessModes``, then to the access modes used by the
   source PVC.
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

Using a custom certificate authority
====================================

Normally, Kopia will use a default set of certificates to verify the validity
of remote repositories when making https connections. However, users that deploy
with a self-signed certificate will need to provide their CA's certificate via
the ``customCA`` option.

The custom CA certificate needs to be provided in a Secret or ConfigMap to
VolSync. For example, if the CA certificate is a file in the current directory
named ``ca.crt``, it can be loaded as a Secret or a ConfigMap.

Example using a customCA loaded as a secret:

.. code-block:: console

   $ kubectl create secret generic tls-secret --from-file=ca.crt=./ca.crt
   secret/tls-secret created

   $ kubectl describe secret/tls-secret
   Name:         tls-secret
   Namespace:    default
   Labels:       <none>
   Annotations:  <none>

   Type:  Opaque

   Data
   ====
   ca.crt:  1127 bytes

This Secret would then be used in the ReplicationSource and/or
ReplicationDestination objects:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup-with-customca
   spec:
     # ... fields omitted ...
     kopia:
       # ... other fields omitted ...
       customCA:
         secretName: tls-secret
         key: ca.crt

To use a customCA in a ConfigMap, specify ``configMapName`` in the spec instead
of ``secretName``, for example:

.. code-block:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: mydata-backup-with-customca
   spec:
     # ... fields omitted ...
     kopia:
       # ... other fields omitted ...
       customCA:
         configMapName: tls-configmap-name
         key: ca.crt

Troubleshooting
===============

Common issues and solutions when using the Kopia mover:

Repository connection issues
----------------------------

**Problem**: Kopia fails to connect to the repository with authentication errors.

**Solution**: Verify that the credentials in your ``kopia-config`` Secret are correct:

.. code-block:: console

   $ kubectl get secret kopia-config -o yaml
   $ kubectl describe secret kopia-config

For S3-compatible storage, ensure the endpoint URL is correct and accessible from the cluster.

**Problem**: Repository connection fails with endpoint or TLS errors.

**Solution**: Check the mover job logs for secure environment variable status. The logs will show which variables are ``[SET]`` or ``[NOT SET]`` without exposing actual values:

.. code-block:: console

   $ kubectl logs <kopia-job-pod-name>
   
   === ENVIRONMENT VARIABLES STATUS ===
   KOPIA_REPOSITORY: [SET]
   KOPIA_PASSWORD: [SET]  
   KOPIA_S3_BUCKET: [SET]
   KOPIA_S3_ENDPOINT: [SET]
   AWS_ACCESS_KEY_ID: [SET]
   AWS_SECRET_ACCESS_KEY: [NOT SET]  # This indicates a missing credential

For S3 endpoints using HTTP (not HTTPS), ensure ``KOPIA_S3_DISABLE_TLS: "true"`` is set in your Secret.

**Problem**: Repository initialization fails.

**Solution**: Ensure the storage backend is accessible and the bucket/container exists:

.. code-block:: console

   # Check if the storage backend is reachable
   $ kubectl run test-pod --image=curlimages/curl --rm -it -- curl -v http://minio.minio.svc.cluster.local:9000

Cache volume issues
-------------------

**Problem**: Kopia mover fails with "no space left on device" errors.

**Solution**: Increase the cache capacity in your ReplicationSource/ReplicationDestination:

.. code-block:: yaml

   kopia:
     cacheCapacity: 5Gi  # Increase from default 1Gi

The cache volume stores repository metadata and must be sized appropriately for your repository. Larger repositories with many snapshots require more cache space.

**Problem**: Cache PVC remains after ReplicationDestination is deleted.

**Solution**: Set ``cleanupCachePVC: true`` in your ReplicationDestination to automatically clean up cache volumes:

.. code-block:: yaml

   kopia:
     cleanupCachePVC: true

**Problem**: Cache volume uses wrong StorageClass or access modes.

**Solution**: Explicitly configure cache volume settings:

.. code-block:: yaml

   kopia:
     cacheCapacity: 2Gi
     cacheStorageClassName: fast-ssd
     cacheAccessModes:
       - ReadWriteOnce

The cache volume configuration follows this priority:
1. Explicitly set ``cacheStorageClassName`` and ``cacheAccessModes``
2. ReplicationSource/ReplicationDestination ``storageClassName`` and ``accessModes``  
3. Source PVC ``storageClassName`` and ``accessModes``

Performance issues
------------------

**Problem**: Backups are slow or time out.

**Solutions**:

1. Increase parallelism for faster uploads:

   .. code-block:: yaml

      kopia:
        parallelism: 4  # Default is 1

2. Use faster compression or disable compression:

   .. code-block:: yaml

      kopia:
        compression: s2   # Faster than zstd
        # or
        compression: none # No compression

3. Increase mover resource limits:

   .. code-block:: yaml

      kopia:
        moverResources:
          limits:
            cpu: "2"
            memory: 4Gi
          requests:
            cpu: "1"
            memory: 2Gi

Snapshot consistency issues
---------------------------

**Problem**: Database backups are inconsistent or corrupted.

**Solution**: Use ``beforeSnapshot`` actions to ensure consistency:

.. code-block:: yaml

   kopia:
     actions:
       beforeSnapshot: "sync && echo 3 > /proc/sys/vm/drop_caches"
       # For databases, use appropriate flush/lock commands
       # beforeSnapshot: "mysqldump --single-transaction --all-databases > /data/backup.sql"

**Problem**: Actions fail or time out.

**Solution**: Ensure commands are compatible with the container environment and have appropriate timeouts. Actions run in a basic shell environment within the data container.

Debugging and logging
---------------------

**Secure Environment Variable Logging**

VolSync's Kopia mover provides secure logging of environment variables to help with troubleshooting without exposing sensitive credentials:

.. code-block:: console

   $ kubectl logs <kopia-job-pod-name> | grep "ENVIRONMENT VARIABLES STATUS" -A 10
   
   === ENVIRONMENT VARIABLES STATUS ===
   KOPIA_REPOSITORY: [SET]
   KOPIA_PASSWORD: [SET]
   KOPIA_S3_BUCKET: [SET]
   KOPIA_S3_ENDPOINT: [SET]
   AWS_ACCESS_KEY_ID: [SET]
   AWS_SECRET_ACCESS_KEY: [SET]

This output helps identify missing configuration without revealing actual credential values.

**Cache and Log Directory Information**

The mover logs also provide detailed information about cache and log directory setup:

.. code-block:: console

   === DEBUG: Environment Setup ===
   KOPIA_CACHE_DIR: /tmp/kopia-cache
   KOPIA_CACHE_DIRECTORY: /tmp/kopia-cache
   KOPIA_LOG_DIR: /tmp/kopia-cache/logs
   Cache directory writable: Yes
   Log directory exists: Yes

This helps diagnose cache volume mounting and permission issues.

**Connection Debug Information**

For S3 repositories, the mover provides detailed connection debugging:

.. code-block:: console

   === S3 Connection Debug ===
   KOPIA_S3_BUCKET: [SET]
   KOPIA_S3_ENDPOINT: [SET]
   KOPIA_S3_DISABLE_TLS: [SET]

This helps identify S3-specific configuration issues without exposing credentials.

Repository maintenance issues
-----------------------------

**Problem**: Repository grows too large despite retention policies.

**Solution**: Ensure maintenance is running regularly:

.. code-block:: yaml

   kopia:
     maintenanceIntervalDays: 3  # Run maintenance more frequently

Check the ``lastMaintenance`` field in the ReplicationSource status to verify maintenance is occurring.

**Problem**: Multiple backup sources conflict.

**Solution**: While Kopia supports concurrent access, for better isolation use separate repository paths:

.. code-block:: yaml

   # Source 1
   KOPIA_REPOSITORY: s3://bucket/app1-backups
   
   # Source 2
   KOPIA_REPOSITORY: s3://bucket/app2-backups

Restore issues
--------------

**Problem**: Restore fails with "snapshot not found" errors.

**Solution**: Verify the snapshot exists and check timestamp format:

.. code-block:: yaml

   kopia:
     restoreAsOf: "2024-01-15T18:30:00Z"  # Must use RFC-3339 format

**Problem**: Partial restore or missing files.

**Solution**: Ensure the destination PVC has sufficient space and appropriate permissions. Check that the ``copyMethod`` is set correctly for your use case.

Backend-specific issues
-----------------------

**Backblaze B2 Issues**

**Problem**: B2 authentication failures.

**Solution**: Verify your B2 credentials and bucket permissions:

.. code-block:: console

   # Test B2 credentials locally
   $ b2 authorize-account <account-id> <application-key>
   $ b2 list-buckets

Ensure the application key has read/write permissions for the specified bucket.

**Problem**: B2 bucket not found or access denied.

**Solution**: Check that the bucket name in ``KOPIA_REPOSITORY`` matches exactly and that the bucket exists in your B2 account. Bucket names are case-sensitive.

**WebDAV Issues**

**Problem**: WebDAV connection failures or timeout errors.

**Solution**: Verify WebDAV server accessibility and credentials:

.. code-block:: console

   # Test WebDAV connectivity
   $ curl -u username:password -X PROPFIND https://webdav.example.com/path/

Check that the WebDAV URL is correct and includes the proper path. Ensure the server supports required HTTP methods (PROPFIND, GET, PUT, DELETE).

**Problem**: WebDAV SSL/TLS certificate errors.

**Solution**: For self-signed certificates, use the ``customCA`` option or ensure proper certificate validation. For internal servers, consider using HTTP with appropriate network security policies.

**SFTP Issues**

**Problem**: SFTP connection refused or authentication failures.

**Solution**: Verify SSH connectivity and credentials:

.. code-block:: console

   # Test SSH connection
   $ ssh -p 22 backup-user@backup-server.example.com
   
   # Test with specific key
   $ ssh -i /path/to/key -p 22 backup-user@backup-server.example.com

Ensure the SSH service is running on the specified port and that firewall rules allow connections.

**Problem**: SFTP path permission errors.

**Solution**: Verify that the backup user has read/write access to the specified ``SFTP_PATH``. The path should exist and be writable by the backup user.

**Problem**: SSH key format issues.

**Solution**: Ensure the SSH key is in the correct format. Some systems require PEM format rather than OpenSSH format:

.. code-block:: console

   # Convert OpenSSH to PEM format if needed
   $ ssh-keygen -p -m PEM -f ~/.ssh/id_rsa

**Rclone Issues**

**Problem**: Rclone remote not found or configuration errors.

**Solution**: Verify the Rclone configuration syntax and remote names:

.. code-block:: console

   # Test Rclone configuration locally
   $ rclone --config rclone.conf ls remote-name:

Ensure the remote name in ``RCLONE_REMOTE_PATH`` exactly matches the section name in ``RCLONE_CONFIG``.

**Problem**: Rclone provider-specific authentication failures.

**Solution**: Check provider-specific requirements:

* **OAuth2 providers**: Ensure tokens are valid and not expired
* **API key providers**: Verify keys have proper permissions
* **Regional providers**: Check endpoint URLs and regional settings

**Problem**: Rclone performance issues.

**Solution**: Consider provider-specific optimizations:

.. code-block:: yaml

   kopia:
     parallelism: 1  # Some providers perform better with sequential operations
     # or
     parallelism: 8  # Others benefit from higher parallelism

**Google Drive Issues**

**Problem**: Google Drive API authentication failures.

**Solution**: Verify service account setup and folder sharing:

1. Ensure the Google Drive API is enabled in your Google Cloud project
2. Verify the service account email has been shared the target folder
3. Check that the credentials JSON is properly formatted

**Problem**: Google Drive quota or rate limit errors.

**Solution**: Monitor API usage in the Google Cloud Console:

.. code-block:: console

   # Check quota usage in Google Cloud Console
   # Navigate to: APIs & Services > Quotas

Consider reducing ``parallelism`` to lower the API request rate or upgrading to Google Workspace for higher quotas.

**Problem**: Google Drive folder ID not found.

**Solution**: Verify the folder ID is correct and the folder exists:

1. Open the folder in Google Drive web interface
2. Copy the folder ID from the URL
3. Ensure the service account has been granted access to the folder

**Problem**: Large file upload failures to Google Drive.

**Solution**: Google Drive has file size limits:

* Personal accounts: 750GB per file
* Google Workspace: 5TB per file

Consider using Google Cloud Storage instead for very large backup files.

Advanced policy configuration
===============================

VolSync supports Kopia's advanced policy-based configuration system, allowing users to define comprehensive backup policies using ConfigMaps or Secrets. This enables fine-grained control over Kopia's behavior including compression, retention, ignore patterns, error handling, and more.

Overview of Kopia policies
---------------------------

Kopia uses a hierarchical policy system with four levels:

1. **Global Policy** - Applies to all snapshots in the repository
2. **Per-Host Policy (@host)** - Applies to all snapshots from a specific machine  
3. **Per-User Policy (user@host)** - Applies to all snapshots from a specific user
4. **Per-Directory Policy (user@host:path)** - Applies to specific directories

More specific policies override less specific ones (Directory → User → Host → Global).

VolSync currently supports global policy configuration, which provides comprehensive control over backup behavior across the entire repository.

Policy configuration options
-----------------------------

The ``policyConfig`` field allows you to specify ConfigMaps or Secrets containing Kopia policy JSON files:

.. code-block:: yaml

   kopia:
     repository: kopia-config
     policyConfig:
       # Use either configMapName OR secretName, not both
       configMapName: kopia-policies
       # secretName: kopia-policy-secret
       
       # Optional: customize filenames (defaults shown)
       globalPolicyFilename: global-policy.json
       repositoryConfigFilename: repository.config

.. note::
   The ``policyConfig`` field is available for both ReplicationSource and ReplicationDestination objects, allowing policy-driven configuration for both backup and restore operations.

``configMapName``
   The name of a ConfigMap containing policy configuration files. Use this for non-sensitive policy data.

``secretName``
   The name of a Secret containing policy configuration files. Use this for policies containing sensitive information like scripts or credentials.

``globalPolicyFilename``
   The filename for the global policy configuration within the ConfigMap/Secret. Defaults to ``global-policy.json``.

``repositoryConfigFilename``
   The filename for repository-specific settings within the ConfigMap/Secret. Defaults to ``repository.config``.

Creating policy files
----------------------

Global policy file format
~~~~~~~~~~~~~~~~~~~~~~~~~~

The global policy file should be in JSON format and can include comprehensive backup settings:

.. code-block:: json

   {
     "compression": {
       "compressorName": "zstd",
       "minSize": 1024,
       "maxSize": 1048576
     },
     "retention": {
       "keepLatest": 10,
       "keepHourly": 24,
       "keepDaily": 30,
       "keepWeekly": 4,
       "keepMonthly": 12,
       "keepAnnual": 3
     },
     "files": {
       "ignore": [
         ".DS_Store",
         "Thumbs.db",
         "*.tmp",
         "*.log",
         "node_modules/",
         ".git/",
         "__pycache__/"
       ],
       "ignoreCacheDirectories": true,
       "noParentIgnoreRules": false
     },
     "errorHandling": {
       "ignoreFileErrors": false,
       "ignoreDirectoryErrors": false
     },
     "upload": {
       "maxParallelFileReads": 16,
       "maxParallelSnapshots": 4,
       "parallelUploads": 8
     },
     "actions": {
       "beforeSnapshotRoot": {
         "script": "sync && echo 3 > /proc/sys/vm/drop_caches",
         "timeout": "5m",
         "mode": "essential"
       },
       "afterSnapshotRoot": {
         "script": "echo 'Backup completed'",
         "timeout": "2m",
         "mode": "optional"
       }
     }
   }

Repository configuration file format
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

The repository configuration file controls repository-wide settings:

.. code-block:: json

   {
     "storage": {
       "type": "s3"
     },
     "caching": {
       "maxCacheSize": 1073741824,
       "maxListCacheDuration": 600
     },
     "enableActions": true,
     "compression": {
       "onlyCompress": ["*.txt", "*.log"],
       "neverCompress": ["*.jpg", "*.png", "*.mp4"],
       "minSize": 1024,
       "maxSize": 1073741824
     }
   }

.. note::
   The ``enableActions`` setting in the repository configuration is required for pre/post snapshot actions defined in policies to execute. Without this setting, action scripts will be ignored even if defined in the global policy.

Policy configuration examples
-----------------------------

Basic policy configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~

Create a ConfigMap with comprehensive backup policies:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd",
           "minSize": 1024
         },
         "retention": {
           "keepLatest": 5,
           "keepDaily": 14,
           "keepWeekly": 8,
           "keepMonthly": 6
         },
         "files": {
           "ignore": [
             "*.log",
             "*.tmp",
             ".cache/",
             "node_modules/"
           ],
           "ignoreCacheDirectories": true
         }
       }
     repository.config: |
       {
         "enableActions": true,
         "caching": {
           "maxCacheSize": 2147483648
         }
       }

Use the policy configuration in a ReplicationSource:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: app-backup-with-policies
   spec:
     sourcePVC: app-data
     trigger:
       schedule: "0 2 * * *"  # Daily at 2 AM
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: kopia-policies
       # Standard fields still work as fallbacks
       cacheCapacity: 5Gi
       copyMethod: Snapshot

Migration from basic configuration
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

You can gradually migrate from basic VolSync configuration to policy-based configuration while maintaining backward compatibility:

**Before (Basic Configuration)**:

.. code-block:: yaml

   kopia:
     repository: kopia-config
     retain:
       daily: 7
       weekly: 4
     compression: zstd
     parallelism: 2

**After (Policy-Based Configuration)**:

.. code-block:: yaml

   kopia:
     repository: kopia-config
     # Add policy configuration
     policyConfig:
       configMapName: kopia-policies
     # Keep existing fields as fallbacks
     retain:
       daily: 7
       weekly: 4  
     compression: zstd
     parallelism: 2

This approach allows incremental adoption of policy-based configuration while ensuring existing backups continue to work.

Advanced policy with actions
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

For applications requiring specific pre/post backup actions:

.. code-block:: yaml

   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-database-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd"
         },
         "retention": {
           "keepLatest": 3,
           "keepDaily": 7,
           "keepWeekly": 4
         },
         "actions": {
           "beforeSnapshotRoot": {
             "script": "mysqldump --single-transaction --all-databases > /data/backup.sql",
             "timeout": "10m",
             "mode": "essential"
           },
           "afterSnapshotRoot": {
             "script": "rm -f /data/backup.sql",
             "timeout": "1m",
             "mode": "optional"
           }
         },
         "files": {
           "ignore": [
             "*.log",
             "mysql-bin.*"
           ]
         }
       }

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: database-backup
   spec:
     sourcePVC: mysql-data
     trigger:
       schedule: "0 1 * * *"
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: kopia-database-policies
       copyMethod: Clone

Environment-specific policies
~~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Different policies for development and production environments:

.. code-block:: yaml

   # Development policies (faster, less retention)
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-dev-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "s2"  # Faster compression
         },
         "retention": {
           "keepLatest": 3,
           "keepDaily": 7
         },
         "upload": {
           "parallelUploads": 2
         }
       }

   ---
   # Production policies (better compression, longer retention)
   apiVersion: v1
   kind: ConfigMap
   metadata:
     name: kopia-prod-policies
   data:
     global-policy.json: |
       {
         "compression": {
           "compressorName": "zstd"  # Better compression
         },
         "retention": {
           "keepLatest": 10,
           "keepDaily": 30,
           "keepWeekly": 12,
           "keepMonthly": 12,
           "keepAnnual": 5
         },
         "upload": {
           "parallelUploads": 8
         }
       }

Using policies with ReplicationDestination
------------------------------------------

Policy configuration can also be used with ReplicationDestination for restore operations:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-with-policies
   spec:
     trigger:
       manual: restore-once
     kopia:
       repository: kopia-config
       policyConfig:
         configMapName: kopia-policies
       destinationPVC: restored-data
       copyMethod: Direct

Policy precedence and interaction
---------------------------------

When both policy files and VolSync spec fields are provided:

1. **Policy files take precedence** for settings they define
2. **VolSync spec fields act as fallbacks** for undefined policy settings  
3. **Repository-level settings** override both for repository-wide configurations

For example, if both ``policyConfig`` and spec-level ``retain`` are specified:

.. code-block:: yaml

   kopia:
     policyConfig:
       configMapName: kopia-policies  # Contains retention: {"keepDaily": 14}
     retain:
       daily: 7   # This becomes fallback since policy defines keepDaily
       weekly: 4  # This is used since policy doesn't define keepWeekly

Troubleshooting policy configuration
------------------------------------

Verifying policy application
~~~~~~~~~~~~~~~~~~~~~~~~~~~~

Check if policies are being applied correctly:

.. code-block:: console

   # Check the ConfigMap contents
   $ kubectl get configmap kopia-policies -o yaml
   
   # View job logs to see policy import messages
   $ kubectl logs <replicationsource-job-name>
   
   # Look for policy import success/failure messages
   $ kubectl logs <replicationsource-job-name> | grep -i policy

Common policy issues
~~~~~~~~~~~~~~~~~~~~

**Invalid JSON format**
   Policy files must be valid JSON. Use a JSON validator to check syntax before creating ConfigMaps/Secrets.

**Missing policy files**
   Ensure the specified filenames exist in the ConfigMap/Secret with the correct names. Default filenames are ``global-policy.json`` and ``repository.config``.

**Policy import failures**
   Check job logs for specific error messages about policy import failures. Common issues include invalid policy syntax or conflicting policy settings.

**ConfigMap/Secret not found**
   Verify the ConfigMap or Secret exists in the same namespace as the ReplicationSource/ReplicationDestination. Policy resources must be in the same namespace as the VolSync resources.

**Actions not executing**
   Ensure ``enableActions`` is set to ``true`` in the repository configuration file. Actions defined in policies will be silently ignored if repository-level actions are disabled.

**Policy precedence confusion**
   Remember that policy file settings override VolSync spec fields. If unexpected behavior occurs, check both policy files and spec fields to understand which settings are taking precedence.

Best practices for policy management
------------------------------------

1. **Use ConfigMaps** for non-sensitive policy data
2. **Use Secrets** for policies containing sensitive scripts or configurations  
3. **Test policies** in development environments before production use
4. **Version control** your policy configurations
5. **Document policy changes** and their expected impact
6. **Monitor backup success** after implementing new policies
7. **Use meaningful names** for ConfigMaps/Secrets to identify their purpose
8. **Validate JSON** before creating ConfigMaps/Secrets

Security considerations
-----------------------

VolSync's Kopia mover includes several security features and considerations:

**Secure Credential Handling**

* Environment variables containing credentials are never logged in plaintext
* Mover logs show ``[SET]`` or ``[NOT SET]`` status for troubleshooting without credential exposure
* Connection debugging information excludes sensitive values while providing configuration visibility

**Policy Configuration Security**

Policy files can contain executable scripts in the ``actions`` section. Consider these security aspects:

* **Validate script content** before deploying policies
* **Use Secrets** for policies containing sensitive information
* **Apply appropriate RBAC** to ConfigMaps/Secrets containing policies
* **Monitor policy changes** through change management processes
* **Limit script complexity** to reduce potential security risks

**Repository Access Security**

* Repository passwords should be unique per repository for isolation
* Use separate repository paths even when Kopia supports concurrent access
* Consider using SAS tokens or temporary credentials for cloud storage when possible
* Regularly rotate storage backend credentials according to your security policies

Policy configuration quick reference
====================================

Field reference
---------------

.. code-block:: yaml

   kopia:
     repository: kopia-config
     policyConfig:
       # Required: specify either configMapName OR secretName
       configMapName: my-policies     # ConfigMap containing policy files
       secretName: my-policy-secret   # Secret containing policy files
       
       # Optional: custom filenames (defaults shown)
       globalPolicyFilename: global-policy.json      # Global policy file
       repositoryConfigFilename: repository.config   # Repository config file

Global policy structure
-----------------------

.. code-block:: json

   {
     "compression": {
       "compressorName": "zstd|gzip|s2|none",
       "minSize": 1024,
       "maxSize": 1048576
     },
     "retention": {
       "keepLatest": 10,
       "keepHourly": 24,
       "keepDaily": 30,
       "keepWeekly": 4,
       "keepMonthly": 12,
       "keepAnnual": 3
     },
     "files": {
       "ignore": ["*.tmp", "*.log", ".cache/"],
       "ignoreCacheDirectories": true,
       "noParentIgnoreRules": false
     },
     "errorHandling": {
       "ignoreFileErrors": false,
       "ignoreDirectoryErrors": false
     },
     "upload": {
       "maxParallelFileReads": 16,
       "maxParallelSnapshots": 4,
       "parallelUploads": 8
     },
     "actions": {
       "beforeSnapshotRoot": {
         "script": "sync && echo 3 > /proc/sys/vm/drop_caches",
         "timeout": "5m",
         "mode": "essential|optional"
       }
     }
   }

Repository configuration structure
----------------------------------

.. code-block:: json

   {
     "enableActions": true,
     "caching": {
       "maxCacheSize": 1073741824,
       "maxListCacheDuration": 600
     },
     "compression": {
       "onlyCompress": ["*.txt", "*.log"],
       "neverCompress": ["*.jpg", "*.png", "*.mp4"],
       "minSize": 1024,
       "maxSize": 1073741824
     }
   }

Common use cases
----------------

**Basic policy setup**:
  Use ``configMapName`` with comprehensive retention and compression settings

**Database backups**:
  Use policy actions for consistent snapshots with ``beforeSnapshot`` commands

**Multi-environment**:
  Create separate ConfigMaps for dev, staging, and production policies

**Sensitive configurations**:
  Use ``secretName`` for policies containing scripts or credentials

**Migration**:
  Add ``policyConfig`` while keeping existing spec fields as fallbacks

Kopia-specific features
=======================

Compression
-----------

Kopia offers several compression algorithms that can significantly reduce backup
size and transfer time:

* **zstd** (default): Excellent compression ratio with good speed
* **gzip**: Standard compression, widely compatible
* **s2**: Fast compression with lower CPU usage
* **none**: No compression for already compressed data

.. code-block:: yaml

   kopia:
     compression: zstd

Parallelism
-----------

Kopia can upload data using multiple parallel streams, which can significantly
improve backup performance on high-bandwidth connections:

.. code-block:: yaml

   kopia:
     parallelism: 4  # Use 4 parallel upload streams

Actions (Hooks)
---------------

Kopia supports pre and post snapshot actions that can be used to ensure data
consistency before taking backups:

.. code-block:: yaml

   kopia:
     actions:
       beforeSnapshot: "mysqldump --single-transaction --routines --triggers --all-databases > /data/mysql-dump.sql"
       afterSnapshot: "rm -f /data/mysql-dump.sql"

These actions run inside the source PVC container and can be used to:

* Flush database buffers
* Create consistent application snapshots  
* Pause application writes
* Clean up temporary files after backup

.. note::
   For more advanced action configuration, consider using the ``policyConfig`` option which allows defining actions with timeouts, error handling modes, and more sophisticated scripting capabilities.

Concurrent Access
-----------------

Unlike some other backup tools, Kopia supports multiple clients safely accessing
the same repository. This means multiple VolSync instances can backup to the
same repository path without corruption, making it easier to manage centralized
backup repositories.

What's New in VolSync Kopia Implementation
===========================================

VolSync's Kopia mover includes several enhancements over the basic Kopia functionality:

**Enhanced Environment Variable Support**

* **S3-specific variables**: ``KOPIA_S3_BUCKET``, ``KOPIA_S3_ENDPOINT``, ``KOPIA_S3_DISABLE_TLS``
* **Azure-specific variables**: ``KOPIA_AZURE_CONTAINER``, ``KOPIA_AZURE_STORAGE_ACCOUNT``, ``KOPIA_AZURE_STORAGE_KEY``
* **GCS-specific variables**: ``KOPIA_GCS_BUCKET``, ``GOOGLE_PROJECT_ID``
* **Automatic prefix extraction**: Support for nested repository paths like ``s3://bucket/path1/path2/path3``

**Security and Debugging Improvements**

* **Secure credential logging**: Environment variables show ``[SET]``/``[NOT SET]`` status without exposing values
* **Comprehensive debug output**: Connection, cache, and environment information for troubleshooting
* **Enhanced error reporting**: Clear identification of configuration issues

**Advanced Cache Management**

* **Flexible cache configuration**: Control cache size, StorageClass, and access modes
* **Automatic cleanup**: Optional cache PVC cleanup with ``cleanupCachePVC`` setting
* **Intelligent defaults**: Cache configuration inherits from source PVC settings when not specified

**Policy-Based Configuration**

* **ConfigMap/Secret-based policies**: Store comprehensive Kopia policies in Kubernetes resources
* **Global policy support**: Repository-wide policy configuration for advanced features
* **Action integration**: Pre/post snapshot actions with timeout and error handling
* **Backward compatibility**: Existing VolSync configurations continue to work with policy enhancements

**Repository Management**

* **Automatic initialization**: Repositories are created automatically on first backup
* **Concurrent access support**: Safe multi-client repository access with proper isolation
* **Maintenance scheduling**: Configurable maintenance intervals for repository optimization
* **Advanced retention**: Sophisticated retention policies through policy configuration

These enhancements make VolSync's Kopia mover suitable for enterprise backup scenarios while maintaining ease of use for simple configurations.