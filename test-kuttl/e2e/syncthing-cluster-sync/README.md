# Syncthing Sync For N=5

This test verifies the functionality of VolSync when deploying Syncthing-based ReplicationSources.
We create five different PVCs, and attach each one to a corresponding ReplicationSource using Syncthing as our data mover.
Once the ReplicationSources are successfully deployed, we configure each one to connect to the others within the namespace.
After the movers are configured, we populate a single PVC with a test file and verify that the data is replicated to all the other PVCs.

- 00 - Create `N` PVCs
- 05 - Create `N` ReplicationSources, configure each to sync the corresponding `n-th` PVC
- 10 - Configure all of the ReplicationSources to connect to each other
- 15 - Wait until the ReplicationSources are fully-connected
- 20 - Populate the first PVC with a test file
- 25 - Verify that the test file has been propogated to the other PVCs
