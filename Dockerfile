# Build the manager binary
FROM golang:1.16 as builder

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
ARG VERSION="(unknown)"
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 GO111MODULE=on go build -a -o manager -ldflags "-X=main.volsyncVersion=${VERSION}" main.go

# Final container
FROM registry.access.redhat.com/ubi8-minimal:8.3

# Needs openssh in order to generate ssh keys
RUN microdnf --refresh update && \
    microdnf --nodocs install \
        openssh \
    && microdnf clean all

WORKDIR /
COPY --from=builder /workspace/manager .
# uid/gid: nobody/nobody
USER 65534:65534

ENTRYPOINT ["/manager"]
