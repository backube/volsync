# Build rclone
FROM golang:1.16 as rclone-builder

WORKDIR /workspace

ARG RCLONE_VERSION=v1.55.1
# hash: git rev-list -n 1 ${RCLONE_VERSION}
ARG RCLONE_GIT_HASH=825dd65e1de3b2a1d3926659194298f10b611923

RUN git clone --depth 1 -b ${RCLONE_VERSION} https://github.com/rclone/rclone.git

WORKDIR /workspace/rclone

# Make sure the Rclone version tag matches the git hash we're expecting
RUN /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${RCLONE_GIT_HASH} ]]"

RUN make rclone

# Build final container
FROM registry.access.redhat.com/ubi8-minimal

ARG rclone_version=v1.55.1

RUN microdnf update -y && \
    microdnf install -y \
      acl \
    && microdnf clean all && \
    rm -rf /var/cache/yum

COPY --from=rclone-builder /workspace/rclone/rclone /usr/local/bin/rclone
COPY active.sh \
     /

RUN chmod a+rx /active.sh

ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

LABEL org.label-schema.build-date="${builddate}" \
      org.label-schema.description="rclone-based data mover for VolSync" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="volsync-mover-rclone" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/volsync" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
