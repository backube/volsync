===============================
Understanding ``rclone-secret``
===============================

What is ``rclone-secret``?
==========================

This file provides the configuration details to locate and access the intermediary
storage system. It is mounted as a secret on the Rclone data mover pod.


.. code:: bash

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

For detailed instructions follow `Rclone <https://rclone.org/docs/>`_


Deploy ``rclone-secret``
========================

Source side
------------------------------

.. code:: bash

    $ kubectl create secret generic rclone-secret --from-file=rclone.conf=./examples/rclone.conf -n source
    $ kubectl get secrets -n source
    NAME                  TYPE                                  DATA   AGE
    default-token-g9vdx   kubernetes.io/service-account-token   3      20s
    rclone-secret         Opaque                                1      17s



Destination side
-----------------------------

.. code:: bash

    $ kubectl create secret generic rclone-secret --from-file=rclone.conf=./examples/rclone.conf -n dest
    $ kubectl get secrets -n dest
    NAME                  TYPE                                  DATA   AGE
    default-token-5ngtg   kubernetes.io/service-account-token   3      17s
    rclone-secret         Opaque                                1      6s







