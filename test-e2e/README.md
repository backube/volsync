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
  - Parameters: none
  - Returns:
    - `namespace` - The name of the temporary namespace that was created
- `create_rclone_secret` - Creates a Secret for accessing the in-cluster MinIO
  instance
  - Uses:
    - `get_minio_credentials`
  - Parameters:
    - `minio_namespace`
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
- `get_minio_credentials` - Retrieves the user/password keys for accessing the
  in-cluster MinIO instance
  - Uses: none
  - Parameters:
    - `minio_namespace`
  - Returns:
    - `minio_access_key`
    - `minio_secret_key`
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
