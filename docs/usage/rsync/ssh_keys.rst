=========================
Manual SSH key generation
=========================

Normally, VolSync generates SSH keys upon the deployment of a ReplicationDestination object
but SSH keys can also be provided to VolSync rather than generating new ones. The
steps below will describe the process to provide VolSync SSH keys.

Generating keys
===============

``ssh-keygen`` can be used to generate SSH keys. The keys that are created will
be used to create secrets which will be used by VolSync.

Two key pairs need to be generated. The first SSH key will called ``destination``.

.. code::

   $ ssh-keygen -t rsa -b 4096 -f destination -C "" -N ""
   Generating public/private rsa key pair.
   Your identification has been saved in destination
   Your public key has been saved in destination.pub
   The key fingerprint is:
   SHA256:5gRLpIdeu+3CbkScH7qIsEw6tMNPRdVFUe82ihWw5BU
   The key's randomart image is:
   +---[RSA 4096]----+
   |      ... o*oE.  |
   |     +.  .o + .  |
   |    oo=.   o . . |
   |   ..+++.     o  |
   |    .oooS.   . + |
   |.o  . o*.   o o .|
   |*o.o +..o  . .   |
   |+=o . =.         |
   | .o. o...        |
   +----[SHA256]-----+

The second SSH key will be called `source`:

.. code::

   $ ssh-keygen -t rsa -b 4096 -f source -C "" -N ""
   Generating public/private rsa key pair.
   Your identification has been saved in source
   Your public key has been saved in source.pub
   The key fingerprint is:
   SHA256:NEQNMNsgR43Y3c2dWMyit70JagmbCLNRfakWhWORENU 
   The key's randomart image is:
   +---[RSA 4096]----+
   |    .+OX*O o *.. |
   |    .oo*B E = =  |
   |      .o+o o .   |
   |      ..o.+ .    |
   |     .  S+ . o   |
   |    +   +   o .  |
   |     = o + o . o |
   |    . . o +   o  |
   |         .       |
   +----[SHA256]-----+

Creating secrets
================

Secrets will be created using the SSH keys that were generated above. These keys
must reside on the cluster and namespace that serves as the replication
source/destination.

The destination needs access to the public and private destination keys but only
the public source key:

.. code::

   $ kubectl create ns dest
   $ kubectl create secret generic volsync-rsync-dest-dest-database-destination --from-file=destination=destination --from-file=source.pub=source.pub --from-file=destination.pub=destination.pub -n dest

The source needs access to the public and private source keys but only the public destination key:

.. code::

   $ kubectl create ns source
   $ kubectl create secret generic volsync-rsync-dest-src-database-destination --from-file=source=source --from-file=source.pub=source.pub --from-file=destination.pub=destination.pub -n source

Replication destination configuration
=====================================

The last step to use these keys is to provide the value of ``sshKeys`` to the
ReplicationDestination object as a field. Since the name of a key Secret is
being provided in ``.spec.rsync.sshKeys``, the operator will use this Secret
instead of generating its own and placing it in the ``.status``.

.. code:: yaml

   ---
   apiVersion: volsync.backube/v1alpha1
   kind: ReplicationDestination
   metadata:
     name: database-destination
     namespace: dest
   spec:
     rsync:
       # ... other fields omitted ...
       # This is the name of the Secret we created, above
       sshKeys: volsync-rsync-dest-dest-database-destination

The ReplicationDestination object can now be created:

.. code::

   $ kubectl create -f examples/rsync/volsync_v1alpha1_replicationdestination.yaml

The above steps should be repeated to set the ``sshKeys`` field in the
ReplicationSource.
