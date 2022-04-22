# Syncthing Sync

The purpose of these tests are to establish the following:
1. Syncthing propagates the contents of one PVC to all of the others
2. Syncthing can properly connect with all of the other nodes in the cluster
3. Syncthing can recovery after loss of nodes in the cluster

- 00 - Create `N` PVCs.
- 05 - Create `N` ReplicationSources, configure each to sync the corresponding `n-th` PVC.
- 10 - Configure all of the ReplicationSources to connect to each other.
- 15 - Wait until the ReplicationSources are fully-connected.
- 20 - Populate the first PVC with a test file.
- 25 - Verify that the test file has been propagated to the other PVCs.
- 30 - Delete all running Syncthing Pods and verify that their new Syncthing IDs are the same.
- 35 - Ensure that all Syncthing instances have reconnected after losing a node in the cluster.