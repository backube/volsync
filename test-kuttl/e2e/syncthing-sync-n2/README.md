# Syncthing Sync For N=2

These tests verify that syncthing-sync works for the case where there
are only Syncthing nodes.

- 00 - Create PVC `test-data-1`
- 03 - Populate PVC `test-data-1` with a text file
- 03 - Assert that the test file was written correctly
- 06 - Create ReplicationSource `syncthing-1` and connect it to `test-data-1`
- 09 - Create secondary PVC `test-data-2` 
- 12 - Create ReplicationSource `syncthing-2` and connect it to `test-data-2`,
       and connect configure it to connect to `syncthing-1`
- 15 - Configure `syncthing-1` to connect to `syncthing-2`
- 18 - Validate that the data was received from `syncthing-1` and exists in
`test-data-2`