# Ansible-based e2e tests

Get started:

```console
$ python -m pip install --user --upgrade pip
Requirement already satisfied: pip in /home/jstrunk/.local/lib/python3.10/site-packages (22.2.1)

$ pip install --user --upgrade pipenv
Requirement already satisfied: pipenv in /home/jstrunk/.local/lib/python3.10/site-packages (2022.7.24)
Requirement already satisfied: setuptools>=36.2.1 in /usr/lib/python3.10/site-packages (from pipenv) (57.4.0)
Requirement already satisfied: virtualenv in /usr/lib/python3.10/site-packages (from pipenv) (20.13.4)
Requirement already satisfied: pip>=22.0.4 in /home/jstrunk/.local/lib/python3.10/site-packages (from pipenv) (22.2.1)
Requirement already satisfied: certifi in /usr/lib/python3.10/site-packages (from pipenv) (2020.12.5)
Requirement already satisfied: virtualenv-clone>=0.2.5 in /home/jstrunk/.local/lib/python3.10/site-packages (from pipenv) (0.5.7)
Requirement already satisfied: distlib<1,>=0.3.1 in /usr/lib/python3.10/site-packages (from virtualenv->pipenv) (0.3.2)
Requirement already satisfied: filelock<4,>=3.2 in /usr/lib/python3.10/site-packages (from virtualenv->pipenv) (3.3.1)
Requirement already satisfied: platformdirs<3,>=2 in /home/jstrunk/.local/lib/python3.10/site-packages (from virtualenv->pipenv) (2.5.2)
Requirement already satisfied: six<2,>=1.9.0 in /usr/lib/python3.10/site-packages (from virtualenv->pipenv) (1.16.0)

$ pipenv sync
Installing dependencies from Pipfile.lock (84e3d0)...
  🐍   ▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉▉ 12/12 — 00:00:17
To activate this project's virtualenv, run pipenv shell.
Alternatively, run a command inside the virtualenv with pipenv run.
All dependencies are now up-to-date!

$ pipenv run ansible-galaxy install -r requirements.yml
Starting galaxy collection install process
Process install dependency map
Starting collection install process
Downloading https://galaxy.ansible.com/download/kubernetes-core-2.3.2.tar.gz to /home/jstrunk/.ansible/tmp/ansible-local-1420409oj_yy_7w/tmpihwf81je/kubernetes-core-2.3.2-cya2zdq7
Installing 'kubernetes.core:2.3.2' to '/home/jstrunk/src/backube/volsync/test-e2e/.collections/ansible_collections/kubernetes/core'
kubernetes.core:2.3.2 was installed successfully

# Run a test
$ pipenv run ansible-playbook test_simple_rclone.yml
...

# Run them all in parallel
$ pipenv run ansible-parallel test_*.yml
...
```

## S3 test fixture

Restic and Rclone tests use the in-cluster RustFS S3 fixture. From the
repository root, install the HTTP and custom-CA HTTPS instances before running
those tests:

```console
./hack/run-rustfs.sh
RUSTFS_NAMESPACE=rustfs-tls RUSTFS_USE_TLS=1 ./hack/run-rustfs.sh
```

The installer pins the chart and images, creates the `restic-e2e` bucket, and
discovers the cluster's default StorageClass (Kind uses `csi-hostpath-sc`). Set
`RUSTFS_STORAGE_CLASS` to override the selection, or when there is no default or
multiple defaults are configured. The installer upgrades resources in place
and does not delete namespaces. RustFS is used only as a disposable test/dev S3
fixture; these tests do not establish production compatibility.

The installer prefers the repository-pinned Helm at `bin/helm`; run `make helm`
to download it, or set `HELM_BIN` to another Helm 3 or 4 executable. HTTP
installs need no post-renderer. TLS installs use Helm 3's executable
post-renderer or Helm 4's temporary `postrenderer/v1` subprocess plugin. HTTPS
setup also requires OpenSSL and Python 3. On OpenShift, the installer leaves
UID/GID and `fsGroup` assignment to the restricted SCC; server and bucket-init
security settings remain explicitly restricted. Other clusters use UID/GID and
`fsGroup` 10001 for writable volumes.

## Tags

We can use tags to select subsets of tests to run:

- `e2e` - Main operator e2e tests
- `cli` - Tests for the kubectl plugin/cli
- `rclone`, `restic`, `rsync`, `syncthing` - Tests involving specific movers

## Roles

- `cli` - Invoke the VolSync CLI
  - Parameters:
    - `params` - a list that is passed to the CLI executable as ARGV
    - `timeout` - (optional) Timeout for the CLI call to complete (sec)
- `compare_pvc_data` - Compare the contents of 2 PVCs  
  Spawns a Pod that mounts both PVCs and compares their contents, failing if
  they differ
  - Uses: TBD
  - Parameters:
    - `namespace`
    - `pvc1_name`, `pvc2_name`
    - `timeout`: (optional) Time in seconds to wait
  - Returns: none
- `create_namespace` - Creates a temporary test Namespace and deletes it at the
  end of the test
  - Uses: none
  - Parameters:
    - `pss_enforce`: (optional) Enforce a Pod Security Standards level
      (privileged, baseline, restricted)
  - Returns:
    - `namespace` - The name of the temporary namespace that was created
- `create_rclone_secret` - Creates a Secret for accessing the in-cluster RustFS
  instance
  - Uses:
    - `get_rustfs_credentials`
  - Parameters:
    - `rustfs_namespace`
    - `namespace`
    - `rclone_secret_name`
  - Returns: none
- `gather_cluster_info` - Probes the Kubernetes cluster for information about
  its configuration
  - Uses: none
  - Parameters: none
  - Returns:
    - `cluster_info` -
      [Object](https://docs.ansible.com/ansible/latest/collections/kubernetes/core/k8s_cluster_info_module.html#return-values)
      with information about the cluster
- `get_rustfs_credentials` - Retrieves the access/secret keys for accessing the
  in-cluster RustFS instance
  - Uses: none
  - Parameters:
    - `rustfs_namespace`
  - Returns:
    - `rustfs_access_key`
    - `rustfs_secret_key`
- `pvc_has_data` - Ensure a file in a PVC contains specific data
  - Uses:
    - `gather_cluster_info`
  - Parameters:
    - `data`: Expected file contents
    - `namespace`: Namespace holding the PVC
    - `path`: Path from root of PVC file system
    - `pvc_name`: Name of the PVC object
    - `timeout`: (optional) Time in seconds to wait
  - Returns: none
- `write_to_pvc` - Write data into a PVC  
  Spawn a Pod that writes some data into a file on the PVC
  - Uses: TBD
  - Parameters:
    - `namespace`
    - `pvc_name`
    - `path`: Path relative to the root of the PVC's file system
    - `data`: File contents
  - Returns: none

Open questions:

- OpenShift and vanilla kube (specifically kind w/ csi-hostpath) require
  different Pod security settings so that they can run and successfully write
  into a mounted PVC. How can we simplify that templating across all the places
  where we want to create a Pod-like thing (Pod, Job, Deployment, etc.)?
