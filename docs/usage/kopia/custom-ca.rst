====================================
Using a Custom Certificate Authority
====================================

.. contents:: Custom CA Configuration
   :local:

Normally, Kopia will use a default set of certificates to verify the validity
of remote repositories when making https connections. However, users that deploy
with a self-signed certificate will need to provide their CA's certificate via
the ``customCA`` option.

Setting up Custom CA
---------------------

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

Using ConfigMap for Custom CA
------------------------------

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