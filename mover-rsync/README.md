# Rsync-based data mover

This directory contains the code for the data mover that uses
[rsync](https://rsync.samba.org/) to transfer data between the source and
destination sites.

Until there's an operator to deploy this container, the following files can be
used for experimentation:

- `make-keys.sh`: This script generates secrets that will allow the source and
  destination to mutually authenticate the ssh connection used for the rsync
  transfer.
- `deploy-source.yaml`: This contains the manifests for the source site. It
  references the `source-secret` that was created by `make-keys.sh`. You will
  need to update the address of the destination site in this file.
- `deploy-destination.yaml`: This contains the manifests for the destination
  site. It needs `destination-secret`, also created by `make-keys.sh`. You may
  need to update the Service definition depending on how you deploy.
