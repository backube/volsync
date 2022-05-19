# Build the Syncthing image from Golang
FROM golang:1.18 as builder
USER root

# source directory
WORKDIR /workspace

# latest release as of writing this
ARG SYNCTHING_VERSION="v1.20.1"
ARG SYNCTHING_GIT_HASH="2145b3701d6f4f615e55e87374e13b9a12077a74"

# clone & cd
RUN git clone \
  --depth 1 \
  --branch=${SYNCTHING_VERSION} \
  https://github.com/syncthing/syncthing.git
WORKDIR /workspace/syncthing

# Make sure we have the correct Syncthing release
RUN /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${SYNCTHING_GIT_HASH} ]]"

# We don't vendor modules. Enforce that behavior
ENV GOFLAGS=-mod=readonly

ENV CGO_ENABLED=1

# Create the binary for syncthing
RUN go run build.go -no-upgrade

# Verify that FIPS crypto libs are accessible
# Check removed since official images don't support boring crypto
#RUN nm /workspace/syncthing/bin/syncthing | grep -q goboringcrypto

# Build final container
FROM registry.access.redhat.com/ubi9-minimal

# RUN useradd -ms /bin/bash stuser
# USER root

RUN microdnf --refresh update -y && \
    microdnf clean all && \
    rm -rf /var/cache/yum

EXPOSE 21027/udp 2000/tcp 22000 8384/tcp

COPY --from=builder \
  /workspace/syncthing/bin/syncthing \
  /usr/local/bin/syncthing


# general image args
ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

# set labels
LABEL org.label-schema.build-date="${builddate}" \
      org.label-schema.description="Syncthing-based data mover for VolSync" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="volsync-mover-syncthing" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/volsync" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"
###### syncthing-specific args ############

# Path where the config volume will be mounted
ENV SYNCTHING_CONFIG_DIR="/config"

# Variables that will be used to configure the data volume on startup
# folder path
ENV SYNCTHING_DATA_DIR="/data"
# can be one of 'sendreceive', 'sendonly', 'receiveonly'
ENV SYNCTHING_DATA_TRANSFERMODE="sendreceive"

# move the config to the root directory so it can be copied before syncthing boot
COPY config.xml /config.xml

COPY entry.sh \
  /
RUN chmod a+rx /entry.sh

ENTRYPOINT [ "/bin/bash" ]
