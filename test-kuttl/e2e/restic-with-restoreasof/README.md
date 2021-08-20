# Testing Restic's restoreAsOf field

This test verfies that Restic will restore from the correct snapshots when
provided with timestamps.

Steps:

- 00 - Create a secret containing the restic repository data
- 05 - Creates a source PVC & populate it with some dummy data
- 10 - Backup the source PVC by creating a ReplicationSource
- 13 - Create a PVC for the destination
- 15 - Restore the first snapshot taken before 1980
- 17 - Delete the source pod
- 20 - Assert that a restore did not take place
- 21 - Remove destination pod and verify job
- 22 - Record the current time and provide that timestamp to a new restic-based ReplicationDestination
- 23 - Verify that the new ReplicationDestination was able to restore from restic
- 24 - Remove attached pods
- 25 - Append new data into the source volume
- 26 - Create a new restore snapshot
- 27 - Try restoring from a snapshot earlier than the previously recorded timestamp
- 28 - Verify that the restore did not take place
