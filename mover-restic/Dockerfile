# Build restic
FROM golang:1.18 as builder
USER root

WORKDIR /workspace

# hash: git rev-list -n 1 ${RESTIC_VERSION}
ARG RESTIC_VERSION=v0.13.1
ARG RESTIC_GIT_HASH=594f155eb6faf57dd02508283f8d84dfa4c125a7
RUN git clone --depth 1 -b ${RESTIC_VERSION} https://github.com/restic/restic.git
RUN cd restic && /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${RESTIC_GIT_HASH} ]]"

ARG MINIOGO_VERSION=v7.0.14
ARG MINIOGO_GIT_HASH=33097a3528aba03acc94231fe8fe0d174c6b6411
RUN git clone --depth 1 -b ${MINIOGO_VERSION} https://github.com/minio/minio-go.git
RUN cd minio-go && /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${MINIOGO_GIT_HASH} ]]"

ADD patch-sources.sh .
RUN ./patch-sources.sh

WORKDIR /workspace/restic

# We don't vendor modules. Enforce that behavior
ENV GOFLAGS=-mod=readonly
# Preserve symbols so that we can verify crypto libs
RUN sed -i 's/preserveSymbols := false/preserveSymbols := true/g' build.go
RUN go run build.go --enable-cgo

# Verify that FIPS crypto libs are accessible
# Check removed since official images don't support boring crypto
#RUN nm restic | grep -q goboringcrypto

# Build final container
FROM registry.access.redhat.com/ubi9-minimal

RUN microdnf --refresh update -y && \
    microdnf clean all && \
    rm -rf /var/cache/yum

COPY --from=builder /workspace/restic/restic /usr/local/bin/restic
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
