============================
Kopia Additional Arguments
============================

The ``additionalArgs`` field allows passing custom command-line arguments to Kopia for advanced use cases not directly exposed by VolSync's API.

When to Use
===========

Use ``additionalArgs`` for:

* Excluding files/directories from backups
* Controlling filesystem boundaries (``--one-file-system``)
* Tuning performance (``--parallel``, ``--upload-speed``)
* Ignoring cache directories (``--ignore-cache-dirs``)
* Handling permission errors during restore

.. important::
   Always prefer VolSync's native configuration options when available. Only use ``additionalArgs`` for features not directly exposed.

Configuration
=============

Add arguments to your ReplicationSource or ReplicationDestination:

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationSource
   metadata:
     name: backup-source
   spec:
     sourcePVC: my-data
     trigger:
       schedule: "*/30 * * * *"
     kopia:
       repository: kopia-secret
       additionalArgs:
         - "--one-file-system"
         - "--ignore-cache-dirs"
         - "--parallel=8"

.. code-block:: yaml

   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: restore-dest
   spec:
     trigger:
       manual: restore-now
     kopia:
       repository: kopia-secret
       destinationPVC: restored-data
       additionalArgs:
         - "--ignore-permission-errors"
         - "--parallel=4"

Forbidden Flags
===============

For security reasons, the following flags are blocked:

* ``--password``, ``--password-file`` - Use repository secret instead
* ``--config-file``, ``--config`` - Managed by VolSync
* ``--key-id``, ``--key-data`` - Use repository secret
* ``--username``, ``--hostname`` - Use VolSync's identity fields
* ``--repository``, ``--repo`` - Configured via repository secret
* ``AWS_*``, ``AZURE_*``, ``GOOGLE_*`` credential flags - Use repository secret

Common Use Cases
================

Exclude Files
-------------

.. code-block:: yaml

   additionalArgs:
     - "--exclude=*.tmp"
     - "--exclude=cache/"
     - "--exclude-caches"

Performance Tuning
------------------

.. code-block:: yaml

   additionalArgs:
     - "--parallel=16"              # Increase parallel operations
     - "--upload-speed=100"         # Limit upload to 100 MB/s
     - "--compression=s2-parallel-8" # Use parallel compression

Filesystem Boundaries
---------------------

.. code-block:: yaml

   additionalArgs:
     - "--one-file-system"          # Don't cross filesystem boundaries
     - "--ignore-inode-changes"     # Ignore inode number changes

Restore Options
---------------

.. code-block:: yaml

   additionalArgs:
     - "--ignore-permission-errors"  # Continue on permission errors
     - "--no-overwrite-files"       # Don't overwrite existing files
     - "--no-overwrite-directories" # Don't overwrite existing directories

Limitations
===========

* Maximum 20 arguments per ReplicationSource/Destination
* Arguments are passed as-is to Kopia without validation
* Invalid arguments will cause backup/restore failures
* Check Kopia logs in the job pod for debugging

Troubleshooting
===============

To verify arguments are being applied:

.. code-block:: bash

   # Check the job pod logs
   kubectl logs -l volsync.backube/replication-name=backup-source

   # Look for lines showing the Kopia command being executed
   # Additional arguments should appear in the command

If backups/restores fail after adding arguments:

1. Remove the ``additionalArgs`` to verify the issue
2. Check Kopia documentation for correct argument syntax
3. Test arguments locally with Kopia CLI if possible
4. Add arguments one at a time to identify the problematic one