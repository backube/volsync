FROM registry.access.redhat.com/ubi9-minimal

RUN microdnf --refresh update -y && \
    microdnf --nodocs --setopt=install_weak_deps=0 install -y \
      bash \
      rsync \
      stunnel \
    && microdnf clean all

COPY server.sh \
     client.sh \
     /

RUN chmod a+rx /server.sh /client.sh

ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

LABEL org.label-schema.build-date="${builddate}" \
      org.label-schema.description="rsync-over-tls-based data mover for VolSync" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="volsync-mover-rsync-tls" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/volsync" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
