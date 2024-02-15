===============================
Understanding ``rclone-secret``
===============================

The Rclone Secret provides the configuration details to locate and access the intermediary
storage system. It is mounted as a secret on the Rclone data mover pod and provided to the Rclone executable.

The secret should contain the key ``rclone.conf`` that contains the contents of your rclone.conf file. Here is an
example rclone.conf:

.. code:: none

    [aws-s3-bucket]
    type = s3
    provider = AWS
    env_auth = false
    access_key_id = *******
    secret_access_key = ******
    region = <region>
    location_constraint = <region>
    acl = private


In the above example AWS S3 is used as the backend for the intermediary storage system.

    - ``[aws-s3-bucket]``: Name of the remote
    - ``type``: Type of storage
    - ``provider``: Backend provider
    - ``access_key_id``: AWS credentials
    - ``secret_access_key``: AWS credentials
    - ``region``: Region to connect to
    - ``location_constraint``: Must be set to match the ``region``

For detailed instructions follow the `Rclone documentation <https://rclone.org/docs/>`_ on how to create an ``rclone.conf`` configuration file.


Deploy ``rclone-secret``
========================

Assuming the above example is placed in a local file, ``rclone.conf``, the
Secret can be created via:

.. code:: console

    # Create the secret (remember to pass the correct namespace name)
    $ kubectl create -n source secret generic rclone-secret --from-file=rclone.conf=rclone.conf
    $ kubectl get -n source secrets
    NAME                  TYPE                                  DATA   AGE
    default-token-g9vdx   kubernetes.io/service-account-token   3      20s
    rclone-secret         Opaque                                1      17s

This Secret should be created on both the source and the destination locations.

Using ``RCLONE_`` environment variables in ``rclone-secret``
============================================================

Rclone has the ability to set environment variables for configuration. Environment variables that
start with ``RCLONE_`` can be set as key/value pairs in the ``rclone-secret`` and they will be passed
to the rclone mover job.

Here is an example ``rclone-secret`` that sets ``RCLONE_BWLIMIT`` to 5M:

.. code-block:: yaml

   apiVersion: v1
   kind: Secret
   metadata:
     name: rclone-secret
   type: Opaque
   stringData:
     # equivalent to the --bwlimit command line flag
     RCLONE_BWLIMIT: 5M
     # rclone.conf
     rclone.conf: |
       [s3-bucket]
       type = s3
       provider = Minio
       env_auth = false
       access_key_id = user1
       secret_access_key = abc123
       region = us-east-1
       endpoint = http://minio.minio.svc.cluster.local:9000

For detailed information on Rclone environment variables see the
`Rclone environment variable documentation <https://rclone.org/docs/#environment-variables>`_.
