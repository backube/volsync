=====================
Mover service account
=====================

.. toctree::
   :hidden:

VolSync normally creates a service account to be used by the data mover pod for each ReplicationSource
and ReplicationDestination in the namespace. Optionally, users can use their own serviceaccounts
instead if they want more control of their serviceaccounts and their roles, or if they
wish to share serviceaccounts between replicationsources or replicationdestinations.

Each ReplicationSource and ReplicationDestination has an optional field ``.spec.<mover>.moverServiceAccount``
where the name of a service account can be set. If this field is set, VolSync will not
create a service account and will instead use the one specified.  This service account must exist
in the same namespace as the corresponding ReplicationSource or ReplicationDestination.

Private registry scenario
=========================

One potential use-case for needing to use your own service account is if you have images stored in
a private registry that requires an image pull secret to be set on pods or added to
serviceaccounts in the namespace.

A service account can be created in the namespace and given access to the image pull secret,
following `these steps. <https://kubernetes.io/docs/tasks/configure-pod-container/configure-service-account/#add-imagepullsecrets-to-a-service-account>`_

Next, this service account can be set in the ReplicationSource or ReplicationDestination.  Here is
an example using a restic mover with a user-created service account called ``my-service-acct``:

.. code-block:: yaml

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: source
    namespace: test-ns
  spec:
    sourcePVC: data-source
    trigger:
      manual: once
    restic:
      moverServiceAccount: my-service-acct # User supplied mover service account
      pruneIntervalDays: 1
      repository: restic-secret
      retain:
        hourly: 3
        daily: 2
        monthly: 1
      copyMethod: Snapshot
      cacheCapacity: 1Gi
