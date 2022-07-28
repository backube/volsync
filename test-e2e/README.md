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

$ pipenv run ansible-galaxy collection install -p .collections kubernetes.core
Starting galaxy collection install process
Nothing to do. All requested collections are already installed. If you want to reinstall them, consider using `--force`.

# Run a test
$ pipenv run ansible-playbook test_simple_rclone.yml
...

# Run them all in parallel
$ pipenv run ansible-parallel test_*.yml
...
```

## Roles

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
