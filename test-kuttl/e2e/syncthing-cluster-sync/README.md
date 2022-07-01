# Syncthing Sync

The purpose of these tests is to establish that the following
is true about our Syncthing-based cluster:

<!-- the list  below is formatted as such to hack
around mdl breaking with ordered lists.
I've really tried as hard as possible, but there seems
to be some -->

VolSync is able to propagate the changes made on one ReplicationSource
to all the other ReplicationSources in the cluster. We also want to determine
whether our ReplicationSources are able to properly connect with each other.
Lastly, we want to  ensure that VolSync can recover after loss of
nodes in the cluster.

The tests are ordered as follows:

- 00 - Create $n$ PVCs.
- 05 - Create $n$ ReplicationSources, configure each to sync
  the corresponding $n^{th}$ PVC.
- 10 - Configure all of the ReplicationSources to connect to each other.
- 15 - Wait until the ReplicationSources are fully-connected.
- 20 - Populate the first PVC with a test file.
- 25 - Verify that the test file has been propagated to the
  other PVCs.
- 30 - Delete all running Syncthing Pods and verify that
  their new Syncthing IDs are the same.
- 35 - Ensure that all Syncthing instances have reconnected
  after losing a node in the cluster.