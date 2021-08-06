FROM registry.access.redhat.com/ubi8-minimal

RUN microdnf install -y \
      bzip2 \
    && microdnf clean all

ARG RESTIC_VERSION=0.12.1
ARG RESTIC_SHA256=11d6ee35ec73058dae73d31d9cd17fe79661090abeb034ec6e13e3c69a4e7088

RUN curl -Lo /restic.bz2 https://github.com/restic/restic/releases/download/v${RESTIC_VERSION}/restic_${RESTIC_VERSION}_linux_amd64.bz2 && \
    echo "${RESTIC_SHA256} /restic.bz2" > /restic.sum && \
    sha256sum --check --strict /restic.sum && \
    bzcat restic.bz2 > /usr/local/bin/restic && \
    chmod a+x /usr/local/bin/restic && \
    rm -f /restic.*

COPY entry.sh \
     /

RUN chmod a+rx /entry.sh

ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

LABEL org.label-schema.build-date="${builddate}" \
      org.label-schema.description="restic-based data mover for VolSync" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="volsync-mover-restic" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/volsync" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
