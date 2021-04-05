===============================
Understanding ``restic-config``
===============================

What is ``restic-config``?
==========================

The place where your backups will be saved is called a "repository". 
This file provides the configuration details to locate and access restic repository.
It is mounted as a secret on the Restic data backup pod.


.. code:: yaml

    apiVersion: v1
    kind: Secret
    metadata:
        name: restic-config
    type: Opaque
    stringData:
        # The repository url
        RESTIC_REPOSITORY: s3:http://minio.minio.svc.cluster.local:9000/restic-repo
        # The repository encryption key
        RESTIC_PASSWORD: my-secure-restic-password
        # ENV vars specific to the back end
        # https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html
        AWS_ACCESS_KEY_ID: access
        AWS_SECRET_ACCESS_KEY: password


In the above example Minio is used for restic repository.

    - ``restic-config``: Name of the secret
    - ``RESTIC_REPOSITORY``: For automated backups, restic accepts the repository location in the environment variable RESTIC_REPOSITORY
    - ``RESTIC_PASSWORD``: Password to access to restic repository
    - ``AWS_ACCESS_KEY_ID``:  Minio access key id
    - ``AWS_SECRET_ACCESS_KEY``: Minio secret access key


For detailed instructions follow `Restic with minio <https://restic.readthedocs.io/en/stable/030_preparing_a_new_repo.html#minio-server>`_


Deploy ``restic-config``
========================

Source side
------------------------------

.. code:: bash

    $ kubectl create -f example/source-restic/ -n source
    $ kubectl get secrets -n source
    NAME                  TYPE                                  DATA   AGE
    default-token-g9vdx   kubernetes.io/service-account-token   3      20s
    restic-config         Opaque                                1      17s



Destination side
-----------------------------

.. code:: bash

    $ kubectl create -f example/source-restic/ -n dest
    $ kubectl get secrets -n dest
    NAME                  TYPE                                  DATA   AGE
    default-token-5ngtg   kubernetes.io/service-account-token   3      17s
    restic-config         Opaque                                1      6s







