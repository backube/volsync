.. _ssh_keys:

========
SSH Keys
========
Scribe generates SSH keys upon the deployment of a `replicationdestination` object but SSH keys can be provided to Scribe rather than generating new ones. The steps below will describe the process to provide Scribe SSH keys.

Generating Keys
===============
`ssh-keygen` can be used to generate SSH keys. The keys that are created will be used to create secrets which will be used by Scribe.

Two keys need to be generated one. The first SSH key will called `destination`.

.. code-block::

   $ ssh-keygen
   Generating public/private rsa key pair.
   Enter file in which to save the key (/root/.ssh/id_rsa): /root/.ssh/destination
   Enter passphrase (empty for no passphrase):
   Enter same passphrase again:
   Your identification has been saved in /root/.ssh/destination.
   Your public key has been saved in /root/.ssh/destination.pub.
   The key fingerprint is:
   SHA256:KcqQIcKWw+EHyqIkd0dMI/BmBpcGX/4KqxdUzhVpD4E root@krillan
   The key's randomart image is:
   +---[RSA 2048]----+
   | o+oo+=.o+       |
   |* +=o=E.=        |
   |*XooB+oo o       |
   |BosB..o. ..      |
   |. o.. . S        |
   |   o.+ o         |
   |    +..          |
   |   ..            |
   |  ..             |
   +----[SHA256]-----+

The second SSH key will be called `source`.

.. code-block::

   $ ssh-keygen
   Generating public/private rsa key pair.
   Enter file in which to save the key (/root/.ssh/id_rsa): /root/.ssh/source
   Enter passphrase (empty for no passphrase):
   Enter same passphrase again:
   Your identification has been saved in /root/.ssh/source.
   Your public key has been saved in /root/.ssh/source.pub.
   The key fingerprint is:
   SHA256:hNpheyEvyrlTIXASF4auJN5jXgyYgm/1rCKTGfUfJeQ root@krillan
   The key's randomart image is:
   +---[RSA 2048]----+
   |  .o+.           |
   |  +o. ..         |
   |..o+ o= o        |
   |++o.o+E*..       |
   |=+.o++++S        |
   |o.o=.==o         |
   | =o *+ .         |
   |= ..o..          |
   | o ...           |
   +----[SHA256]-----+

Creating Secrets
================
Secrets will be created using the SSH keys that were generated using `ssh-keygen`.
These keys must reside on the cluster and namespace that serves as the replication destination.
Kubectl will be used to create the namespace and the secrets within the namespace.

.. code-block::

   $ kubectl create ns dest
   $ kubectl create secret generic scribe-rsync-dest-dest-database-destination --from-file=destination=/root/.ssh/destination --from-file=source.pub=/root/.ssh/source.pub --from-file=destination.pub=/root/.ssh/destination.pub -n dest

A secret must also be created on the cluster and namespace serving as the replication
source.

.. code-block::

   $ kubectl create ns source
   $ kubectl create secret generic scribe-rsync-dest-src-database-destination --from-file=source=/root/.ssh/source --from-file=source.pub=/root/.ssh/source.pub --from-file=destination.pub=/root/.ssh/destination.pub -n source

Replication Destination Configuration
=====================================
The last step to use these keys is to provide the value of `sshKeys` to the `replicationdestination` object as a field. As an example we will modify `examples/scribe_v1alpha1_replicationdestination.yaml`.

.. code-block:: yaml

   ---
   apiVersion: scribe.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: database-destination
     namespace: dest
   spec:
     rsync:
       serviceType: ClusterIP
       copyMethod: Snapshot
       capacity: 2Gi
       accessModes: [ReadWriteOnce]
       sshKeys: scribe-rsync-dest-dest-database-destination

The `replicationdestination` object can now be created using either `kubectl` or `oc`.
.. code-block::

   $ kubectl create -f examples/scribe_v1alpha1_replicationdestination.yaml
