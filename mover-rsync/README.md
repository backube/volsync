# Rsync-based data mover

This directory contains the code for the data mover that uses
[rsync](https://rsync.samba.org/) to transfer data between the primary and
secondary sites.

Until there's an operator to deploy this container, the following files can be
used for experimentation:

- `make-keys.sh`: This script generates secrets that will allow the primary and
  secondary to mutually authenticate the ssh connection used for the rsync
  transfer.
- `deploy-primary.yaml`: This contains the manifests for the primary site. It
  references the `primary-secret` that was created by `make-keys.sh`. You will
  need to update the address of the secondary site in this file.
- `deploy-secondary.yaml`: This contains the manifests for the secondary site.
  It needs `secondary-secret`, also created by `make-keys.sh`. You may need to
  update the Service definition depending on how you deploy.
