# Build rclone
FROM golang:1.18 as builder
USER root

WORKDIR /workspace

ARG RCLONE_VERSION=v1.58.1
# hash: git rev-list -n 1 ${RCLONE_VERSION}
ARG RCLONE_GIT_HASH=02f04b08bd26b50dfbfb07672c926e49bd070573

RUN git clone --depth 1 -b ${RCLONE_VERSION} https://github.com/rclone/rclone.git

WORKDIR /workspace/rclone

# Make sure the Rclone version tag matches the git hash we're expecting
RUN /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${RCLONE_GIT_HASH} ]]"

# We don't vendor modules. Enforce that behavior
ENV GOFLAGS=-mod=readonly
# Remove link flag that strips symbols so that we can verify crypto libs
RUN sed -i 's/--ldflags "-s /--ldflags "/g' Makefile
RUN make rclone

# Verify that FIPS crypto libs are accessible
# Check removed since official images don't support boring crypto
#RUN nm rclone | grep -q goboringcrypto

# Build final container
FROM registry.access.redhat.com/ubi9-minimal

RUN microdnf --refresh update -y && \
    microdnf --nodocs --setopt=install_weak_deps=0 install -y \
      acl \
    && microdnf clean all && \
    rm -rf /var/cache/yum

COPY --from=builder /workspace/rclone/rclone /usr/local/bin/rclone
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
