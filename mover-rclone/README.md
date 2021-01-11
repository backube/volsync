# Rclone-based data mover

## Getting started

This directory contains the code for the data mover that uses rclone to
transfer data between the source and destination sites using volumesnapshot
feature.

AWS S3 is used as an intermediate storage for high fan-out data replication.

Until there's an operator to deploy this container, the following steps can be
used for experimentation.

### Prerequiste

Create secret to access AWS S3 account by replacing `<access-key-id>` and
`<secret-access-key>` in `rclone.conf`

```shell
[aws-s3-bucket]
type = s3
provider = AWS
env_auth = false
access_key_id = <access_key_id>
secret_access_key = <secret_access_key>
region = <region>
location_constraint = <location_constraint>
acl = private
```

`oc create secret generic rclone-secret --from-file=rclone.conf=rclone.conf`

### Test

- `deploysource.yaml`

This script creates a `volumesnapshot` of the source pvc.
Next it creates a `mover-rclone-pv-claim` from the `volumesnapshot`.
Job `mover-rclone` copies the data from `mover-rclone-pv-claim` to AWS S3 defined
by `RCLONE_CONFIG_SECTION:RCLONE_DEST_PATH`

- `deploydestination.yaml`

This script creates a `mover-rclone-pv-claim` that will
store incoming data from source. Next it create a job `mover-rclone` that copies
the data from AWS S3 defined by `RCLONE_CONFIG_SECTION:RCLONE_DEST_PATH` to
`mover-rclone-pv-claim`
