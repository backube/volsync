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

COPY primary.sh \
     secondary.sh \
     secondary-command.sh \
     /

RUN chmod a+rx /primary.sh /secondary.sh \secondary-command.sh && \
    ln -s /keys/secondary /etc/ssh/ssh_host_rsa_key && \
    ln -s /keys/secondary.pub /etc/ssh/ssh_host_rsa_key.pub && \
    install /usr/share/doc/rsync/support/rrsync /usr/local/bin

ARG builddate="(unknown)"
ARG version="(unknown)"

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
