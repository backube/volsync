######################################################################
# Establish a common builder image for all golang-based images
FROM golang:1.24 AS golang-builder
USER root
WORKDIR /workspace
# We don't vendor modules. Enforce that behavior
ENV GOFLAGS=-mod=readonly
ENV GO111MODULE=on
ENV CGO_ENABLED=1
ARG TARGETOS
ARG TARGETARCH
ENV GOOS=${TARGETOS:-linux}
ENV GOARCH=${TARGETARCH}


######################################################################
# Build the manager binary
FROM golang-builder AS manager-builder

# Copy the Go Modules manifests & download dependencies
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY cmd/ cmd/
COPY api/ api/
COPY internal/ internal/

# Build
ARG version_arg="(unknown)"
ARG tags_arg=""
RUN go build -a -o manager -ldflags "-X=main.volsyncVersion=${version_arg}" -tags "${tags_arg}" ./cmd/...


######################################################################
# Build rclone
FROM golang-builder AS rclone-builder

ARG RCLONE_VERSION=v1.71.2
ARG RCLONE_GIT_HASH=84a85367111c84061c959e01e0c320618e492dc6

RUN git clone --depth 1 -b ${RCLONE_VERSION} https://github.com/rclone/rclone.git
WORKDIR /workspace/rclone

# Make sure the Rclone version tag matches the git hash we're expecting
RUN /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${RCLONE_GIT_HASH} ]]"

# Tell Go to use the standard BFD linker instead of gold
RUN make rclone BUILD_ARGS="-ldflags='-extldflags=-fuse-ld=bfd'"


######################################################################
# Build restic
FROM golang-builder AS restic-builder

COPY /mover-restic/restic ./restic
COPY /mover-restic/minio-go ./minio-go

WORKDIR /workspace/restic

RUN go run build.go --enable-cgo


######################################################################
# Build syncthing
FROM golang-builder AS syncthing-builder

ARG SYNCTHING_VERSION="v1.30.0"
ARG SYNCTHING_GIT_HASH="0945304a79d6bbaeac7fc2cc1b06f57d3cf66622"

RUN git clone --depth 1 -b ${SYNCTHING_VERSION} https://github.com/syncthing/syncthing.git
WORKDIR /workspace/syncthing

# Make sure we have the correct Syncthing release
RUN /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${SYNCTHING_GIT_HASH} ]]"

RUN go run build.go -no-upgrade


######################################################################
# Build diskrsync binary
FROM golang-builder AS diskrsync-builder

ARG DISKRSYNC_VERSION="v1.3.0"
ARG DISKRSYNC_GIT_HASH="507805c4378495fc2267b77f6eab3d6bb318c86c"

RUN git clone --depth 1 -b ${DISKRSYNC_VERSION} https://github.com/dop251/diskrsync.git
WORKDIR /workspace/diskrsync

# Make sure we have the correct diskrsync release
RUN /bin/bash -c "[[ $(git rev-list -n 1 HEAD) == ${DISKRSYNC_GIT_HASH} ]]"

RUN go build -a -o bin/diskrsync ./diskrsync


######################################################################
# Build diskrsync-tcp binary
FROM golang-builder AS diskrsync-tcp-builder

# Copy the Go Modules manifests & download dependencies
COPY go.mod go.mod
COPY go.sum go.sum
RUN go mod download

# Copy the go source
COPY diskrsync-tcp/ diskrsync-tcp/

# Build
ARG version_arg="(unknown)"
RUN go build -a -o diskrsync-tcp/diskrsync-tcp -ldflags "-X=main.volsyncVersion=${version_arg}" diskrsync-tcp/main.go

######################################################################
# Final container
FROM registry.access.redhat.com/ubi9-minimal
WORKDIR /

RUN microdnf --refresh update -y && \
    microdnf --nodocs --setopt=install_weak_deps=0 install -y \
        acl             `# rclone - getfacl/setfacl` \
        openssh         `# rsync/ssh - ssh key generation in operator` \
        openssh-clients `# rsync/ssh - ssh client` \
        openssh-server  `# rsync/ssh - ssh server` \
        perl            `# rsync/ssh - rrsync script` \
        stunnel         `# rsync-tls` \
        openssl         `# syncthing - server certs` \
        vim-minimal     `# for mover debug` \
        tar             `# for mover debug` \
    && microdnf --setopt=install_weak_deps=0 install -y \
        `# docs are needed so rrsync gets installed for ssh variant` \
        rsync           `# rsync/ssh, rsync-tls - rsync, rrsync` \
    && microdnf clean all && \
    rm -rf /var/cache/yum

##### VolSync operator
COPY --from=manager-builder /workspace/manager /manager

##### rclone
COPY --from=rclone-builder /workspace/rclone/rclone /usr/local/bin/rclone
COPY /mover-rclone/active.sh \
     /mover-rclone/
RUN chmod a+rx /mover-rclone/*.sh

##### restic
COPY --from=restic-builder /workspace/restic/restic /usr/local/bin/restic
COPY /mover-restic/entry.sh \
     /mover-restic/
RUN chmod a+rx /mover-restic/*.sh

##### rsync (ssh)
COPY /mover-rsync/source.sh \
     /mover-rsync/destination.sh \
     /mover-rsync/destination-command.sh \
     /mover-rsync/
RUN chmod a+rx /mover-rsync/*.sh

RUN ln -s /keys/destination /etc/ssh/ssh_host_rsa_key && \
    ln -s /keys/destination.pub /etc/ssh/ssh_host_rsa_key.pub && \
    install /usr/share/doc/rsync/support/rrsync /usr/local/bin && \
    \
    SSHD_CONFIG="/etc/ssh/sshd_config" && \
    sed -ir 's|^[#\s]*\(.*/etc/ssh/ssh_host_ecdsa_key\)$|#\1|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(.*/etc/ssh/ssh_host_ed25519_key\)$|#\1|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(PasswordAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(KbdInteractiveAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(AllowTcpForwarding\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(X11Forwarding\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(PermitTunnel\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(PidFile\)\s.*$|\1 /tmp/sshd.pid|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(UsePAM\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    sed -ir 's|^[#\s]*\(GSSAPIAuthentication\)\s.*$|\1 no|' "$SSHD_CONFIG" && \
    \
    INCLUDED_SSH_CONFIG_DIR="/etc/ssh/sshd_config.d" && \
    sed -ir 's|^[#\s]*\(UsePAM\)\s.*$|\1 no|' "$INCLUDED_SSH_CONFIG_DIR"/* && \
    sed -ir 's|^[#\s]*\(GSSAPIAuthentication\)\s.*$|\1 no|' "$INCLUDED_SSH_CONFIG_DIR"/*

##### rsync-tls
COPY /mover-rsync-tls/client.sh \
     /mover-rsync-tls/server.sh \
     /mover-rsync-tls/
RUN chmod a+rx /mover-rsync-tls/*.sh

##### syncthing
COPY --from=syncthing-builder /workspace/syncthing/bin/syncthing /usr/local/bin/syncthing
ENV SYNCTHING_DATA_TRANSFERMODE="sendreceive"
COPY /mover-syncthing/config-template.xml \
     /mover-syncthing/
RUN chmod a+r /mover-syncthing/config-template.xml

COPY /mover-syncthing/config-template.xml \
     /mover-syncthing/stignore-template \
     /mover-syncthing/entry.sh \
     /mover-syncthing/
RUN chmod a+r /mover-syncthing/config-template.xml && \
    chmod a+r /mover-syncthing/stignore-template && \
    chmod a+rx /mover-syncthing/*.sh

##### diskrsync
COPY --from=diskrsync-builder /workspace/diskrsync/bin/diskrsync /usr/local/bin/diskrsync

##### diskrsync-tcp
COPY --from=diskrsync-tcp-builder /workspace/diskrsync-tcp/diskrsync-tcp /diskrsync-tcp

##### Set build metadata
ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

# https://github.com/opencontainers/image-spec/blob/main/annotations.md
LABEL org.opencontainers.image.base.name="registry.access.redhat.com/ubi9-minimal"
LABEL org.opencontainers.image.created="${builddate}"
LABEL org.opencontainers.image.description="VolSync data replication operator"
LABEL org.opencontainers.image.documentation="https://volsync.readthedocs.io/"
LABEL org.opencontainers.image.licenses="AGPL-3.0-or-later"
LABEL org.opencontainers.image.revision="${version}"
LABEL org.opencontainers.image.source="https://github.com/backube/volsync"
LABEL org.opencontainers.image.title="VolSync"
LABEL org.opencontainers.image.vendor="Backube"
LABEL org.opencontainers.image.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
