# simple-rsync

This test does a simple e2e test of rsync-based replication.

Steps:

- 00 - Creates a ReplicationDestination
- 05 - Starts a pod to populate a PVC with a data file
- 10 - Waits for the Secret & address to be ready (from the rd)
- 15 - Uses the Secret & address to create a ReplicationSource to sync the PVC
  from 05
- 20 - Waits for a successful sync
- 25 - Uses the VolumeSnapshot in the RD's latestImage to provision a new PVC
- 30 - Runs a job to verify the contents of the new PVC and original PVC are the
  same.
