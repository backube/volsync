FROM registry.access.redhat.com/ubi8-minimal

RUN microdnf install -y \
      bzip2 \
    && microdnf clean all

ARG RESTIC_VERSION=0.12.0
ARG RESTIC_SHA256=63d13d53834ea8aa4d461f0bfe32a89c70ec47e239b91f029ed10bd88b8f4b80

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
      org.label-schema.description="restic-based data mover for Scribe" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="scribe-mover-restic" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/scribe" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
