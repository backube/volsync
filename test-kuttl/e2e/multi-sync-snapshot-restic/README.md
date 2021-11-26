# Testing multiple syncs with snapshot using restic

This test verifies that volsync can perform multiple syncs and
create a new unique snapshot each time.  Will also verify that
old replicationdestination snapshots are cleaned up.

Steps:

- 00 - Create a secret containing the restic repository data
- 05 - Creates a source PVC & populate it with some dummy data
- 10 - Backup the source PVC by creating a ReplicationSource with a manual trigger
- 15 - Shutdown source Pod to prevent additional writes to source PVC
- 20 - Create a new restore snapshot by creating a ReplicationDestination
- 25 - Create a PVC for the destination - save name of restore snapshot to 25-snapshot.txt
- 30 - Verify data on the destination PVC matches the source PVC
- 35 - Updates source PVC with additional dummy data
- 40 - Trigger ReplicationSource to backup again
- 45 - Shutdown source Pod to prevent additional writes to source PVC
- 50 - Trigger ReplicationDestination to sync again (new snapshot should be created)
- 55 - Create a PVC for the destination - save name of restore snapshot to 55-snapshot.txt
- 60 - Verify data on the destination PVC (from latest sync) matches the source PVC
- 65 - Verify snapshot from the initial sync is cleaned up
