======================
Mover permission model
======================

.. toctree::
   :hidden:

.. sidebar:: Contents

   .. contents:: Mover permission model
      :local:

VolSync's data movers are responsible for copying the data from one location to
the other. These data movers run as Pods in the user's Namespace, where it can
have access to the data PVCs that it needs to replicate.

The data movers need to run with sufficient privileges to be able to read and
write the affected data. In most cases, running the mover with the same
permissions that are granted to the workload in the Namespace is sufficient.

In some cases, it is necessary to run the data movers with elevated privileges.
The main example of this is where the UID/GID of files need to be preserved,
particularly for ReadWriteMany PVCs. In this case, the mover Pod needs to be
granted sufficient capabilities to read/write files regardless of their
ownership and to modify that ownership.

The choice of whether to run the data movers as normal users or with elevated
privileges depends on the data to be replicated and the security model for the
cluster(s).

Affected movers
===============

The current set of movers that support this dual permission model is:

- rclone
- restic
- rsync-tls
- syncthing

.. note::
  The legacy rsync mover that uses ssh as the transport only supports the elevated permission model.

Controlling mover permissions
=============================

The VolSync operator is responsible for managing the data mover Pods and their
associated ServiceAccounts. By default, the supported movers will be run with
normal user privileges for movers that support this dual permission model.

To signal that the movers in a particular Namespace should be granted additional
permissions, an annotation must be added to the user's Namespace. VolSync checks
for the annotation ``volsync.backube/privileged-movers`` to see if it has a
value of ``true`` before granting elevated privileges to the mover Pods.

For example, the following command will annotate a namespace to allow privileged
movers:

.. code-block:: console
  :emphasize-lines: 12

  $ kubectl annotate ns/elevated-demo volsync.backube/privileged-movers=true
  namespace/elevated-demo annotated

  $ kubectl get ns/elevated-demo -oyaml
  apiVersion: v1
  kind: Namespace
  metadata:
    annotations:
      openshift.io/sa.scc.mcs: s0:c26,c20
      openshift.io/sa.scc.supplemental-groups: 1000690000/10000
      openshift.io/sa.scc.uid-range: 1000690000/10000
      volsync.backube/privileged-movers: "true"
    creationTimestamp: "2022-12-06T16:34:54Z"
    labels:
      kubernetes.io/metadata.name: elevated-demo
      pod-security.kubernetes.io/audit: restricted
      pod-security.kubernetes.io/audit-version: v1.24
      pod-security.kubernetes.io/warn: restricted
      pod-security.kubernetes.io/warn-version: v1.24
    name: elevated-demo
    resourceVersion: "507456"
    uid: 7f3d2184-876b-4f70-9b1d-12462ebda512
  spec:
    finalizers:
    - kubernetes
  status:
    phase: Active


Since the annotation must be added at the Namespace level, cluster
administrators can control which Namespaces will have access to movers with
elevated permissions.

Mover's security context
========================

Kubernetes supports `setting a security context
<https://kubernetes.io/docs/tasks/configure-pod-container/security-context/>`_
for Pods (and individual containers) that controls security aspects such as
which UID or GID they are assigned or which supplemental groups they are a
member of. The VolSync movers can also have their security context configured by
setting a PodSecurityContext in the ``.spec.<mover>.moverSecurityContext`` field
of ReplicationSource and ReplicationDestination objects. This allows matching
the permissions of the mover to that of the primary workload in the Namespace.

As general guidance, if the primary workload specifies a security context, that
same security context should be used for VolSync.

Privilege escalation when using privileged movers
=================================================

By default, movers with elevated privileges are not permitted. This is because
users with access to the Namespace can leverage the data mover's ServiceAccount
to run their own Pods with the same permissions granted to the data mover. In
the case of privileged movers, this would allow normal users to run Pods with a
UID of 0 (root) and to gain access to the DAC_OVERRIDE capability, among others.

Using rsync-tls with UID 0
==========================

When running unprivileged, the data movers explicitly drop all capabilities to
conform to the restricted PSS. For Kubernetes distributions that do not
automatically assign non-root UIDs, this causes the data movers to run as UID 0,
but without any of the normal "superuser" abilities of root. While most movers
are able to handle this case easily, the rsync-tls mover cannot due to a
limitation of rsync.

When using rsync-tls, ensure that the mover is either running with a non-zero
UID or is run with elevated privileges via the VolSync Namespace annotation.
