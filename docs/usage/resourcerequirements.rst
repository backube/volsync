===========================
Mover resource requirements
===========================

.. toctree::
   :hidden:

VolSync's data movers do not run with any specific limits or requests by default.
Each mover's spec can be modified to set resource requests to set limits or requests.

Note that setting these values can have negative affects as mover pods can fail to be
scheduled if requests are set too high or crash due to lack of resources if limits are
set.

Each mover spec has a spec section where ``moverResources`` can be set. Here is an
example restic ``replicationsource`` that sets resource requests for CPU and memory:

.. code-block:: yaml

  apiVersion: volsync.backube/v1alpha1
  kind: ReplicationSource
  metadata:
    name: source
    namespace: "test-ns"
  spec:
    sourcePVC: data-source
    trigger:
      manual: once
    restic:
      pruneIntervalDays: 1
      repository: restic-secret
      retain:
        hourly: 3
        daily: 2
        monthly: 1
      copyMethod: Snapshot
      cacheCapacity: 1Gi
      # Set specific resource requests for the mover container
      moverResources:
        requests:
          memory: "64Mi"
          cpu: "250m"

For more information about resource requirements and limits in kubernetes, see
`Resource Management for Pods and Containers <https://kubernetes.io/docs/concepts/configuration/manage-resources-containers/>`_.
