FROM centos:8

RUN yum update -y && \
    yum install -y \
      bash \
      openssh-clients \
      openssh-server \
      perl \
      rsync \
    && yum clean all && \
    rm -rf /var/cache/yum

COPY source.sh \
     destination.sh \
     destination-command.sh \
     /

RUN chmod a+rx /source.sh /destination.sh \destination-command.sh && \
    ln -s /keys/destination /etc/ssh/ssh_host_rsa_key && \
    ln -s /keys/destination.pub /etc/ssh/ssh_host_rsa_key.pub && \
    install /usr/share/doc/rsync/support/rrsync /usr/local/bin && \
    \
    SSHD_CONFIG="/etc/ssh/sshd_config" && \
    sed -ir 's|^[#\s]*\(.*/etc/ssh/ssh_host_ecdsa_key\)$|#\1|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(.*/etc/ssh/ssh_host_ed25519_key\)$|#\1|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(PasswordAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(GSSAPIAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(AllowTcpForwarding\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(X11Forwarding\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(PermitTunnel\)\s.*$|\1 no|' "$SSHD_CONFIG"

ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

LABEL org.label-schema.build-date="${builddate}" \
      org.label-schema.description="rsync-based data mover for Scribe" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="scribe-mover-rsync" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/scribe" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
