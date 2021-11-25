# Testing multiple syncs with snapshot using rsync

This test verifies that volsync can perform multiple syncs and 
create a new unique snapshot each time.  Will also verify that
old replicationdestination snapshots are cleaned up.

Steps:

- 00 - Creates a ReplicationDestination
- 05 - Creates a source PVC & populate it with some dummy data
- 08 - Waits for the Secret & address to be ready (from the rd)
- 10 - Backup the source PVC by creating a ReplicationSource with a manual trigger - also confirm 
  ReplicationDestination sync is complete
- 15 - Shutdown source Pod to prevent additional writes to source PVC
- 25 - Create a PVC for the destination - save name of restore snapshot to 25-snapshot.txt
- 30 - Verify data on the destination PVC matches the source PVC
- 35 - Updates source PVC with additional dummy data
- 40 - Trigger ReplicationSource/Destination to backup again
- 45 - Shutdown source Pod to prevent additional writes to source PVC
- 55 - Create a PVC for the destination - save name of restore snapshot to 55-snapshot.txt
- 60 - Verify data on the destination PVC (from latest sync) matches the source PVC
- 65 - Verify snapshot from the initial sync is cleaned up