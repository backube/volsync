# Build the manager binary
FROM golang:1.18 as builder
USER root

WORKDIR /workspace
# Copy the Go Modules manifests
COPY go.mod go.mod
COPY go.sum go.sum
# cache deps before building and copying source so that we don't need to re-download as much
# and so that source changes don't invalidate our downloaded layer
RUN go mod download

# Copy the go source
COPY main.go main.go
COPY api/ api/
COPY controllers/ controllers/

# Build
# We don't vendor modules. Enforce that behavior
ENV GOFLAGS=-mod=readonly
ARG version_arg="(unknown)"
RUN GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager -ldflags "-X=main.volsyncVersion=${version_arg}" main.go

# Verify that FIPS crypto libs are accessible
# Check removed since official images don't support boring crypto
#RUN nm manager | grep -q goboringcrypto

# Final container
FROM registry.access.redhat.com/ubi9-minimal

# Needs openssh in order to generate ssh keys
RUN microdnf --refresh update -y && \
    microdnf --nodocs --setopt=install_weak_deps=0 install -y \
        openssh \
    && microdnf clean all && \
    rm -rf /var/cache/yum

WORKDIR /
COPY --from=builder /workspace/manager .
# uid/gid: nobody/nobody
USER 65534:65534

ENTRYPOINT ["/manager"]
