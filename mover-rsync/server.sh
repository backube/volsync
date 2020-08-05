#! /bin/bash

set -e -o pipefail

# Configure sshd
SSHD_CONFIG="/etc/ssh/sshd_config"
sed -ir 's|^[#\s]*\(.*/etc/ssh/ssh_host_ecdsa_key\)$|#\1|' "$SSHD_CONFIG"
sed -ir 's|^[#\s]*\(.*/etc/ssh/ssh_host_ed25519_key\)$|#\1|' "$SSHD_CONFIG"
sed -ir 's|^[#\s]*\(PasswordAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG"
sed -ir 's|^[#\s]*\(GSSAPIAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG"
sed -ir 's|^[#\s]*\(AllowTcpForwarding\)\s.*$|\1 no|' "$SSHD_CONFIG"
sed -ir 's|^[#\s]*\(X11Forwarding\)\s.*$|\1 no|' "$SSHD_CONFIG"
sed -ir 's|^[#\s]*\(PermitTunnel\)\s.*$|\1 no|' "$SSHD_CONFIG"

# Allow client key to access, but restrict what the client can do.
mkdir -p ~/.ssh
chmod 700 ~/.ssh
echo "command=\"/server-command.sh\",restrict $(</keys/client.pub)" > ~/.ssh/authorized_keys

# Wait for incoming rsync transfer
echo "Waiting for connection..."
rm -f /var/run/nologin
/usr/sbin/sshd -D -e

# When sshd exits, need to return the proper exit code from the rsync operation
CODE=255
if [[ -e /tmp/exit_code ]]; then
    CODE_IN="$(</tmp/exit_code)"
    if [[ $CODE_IN =~ ^[0-9]+$ ]]; then
        CODE="$CODE_IN"
    fi
fi
echo "Exiting... Exit code: $CODE"
exit "$CODE"
