==================
Storage Backends
==================

.. contents:: Kopia Storage Backend Configuration
   :local:

Kopia supports various storage backends with their respective configuration formats:

.. note::
   **Alternative: Filesystem Destination**
   
   Instead of configuring a remote storage backend, you can now use a PersistentVolumeClaim 
   as a filesystem-based backup destination. This is ideal for local backups, NFS storage, 
   or air-gapped environments. See :doc:`filesystem-destination` for details.

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

``KOPIA_MANUAL_CONFIG``
   JSON configuration object for manual repository configuration. When provided, overrides VolSync's automatic repository format configuration. See the :doc:`advanced-features` section for detailed usage.

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

   When using ``repositoryPVC`` in the ReplicationSource, the controller automatically sets ``KOPIA_REPOSITORY`` to ``filesystem:///kopia/repository``.
   For manual filesystem configurations, use ``KOPIA_REPOSITORY`` with a ``filesystem://`` URL (e.g., ``filesystem:///mnt/backup``)

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