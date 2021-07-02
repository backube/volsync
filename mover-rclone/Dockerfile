FROM registry.access.redhat.com/ubi8-minimal

ARG rclone_version=v1.55.1

RUN microdnf update -y && \
    microdnf install -y \
      acl \
    && \
    rpm -ivh \
      https://github.com/rclone/rclone/releases/download/${rclone_version}/rclone-${rclone_version}-linux-amd64.rpm \
    && microdnf clean all && \
    rm -rf /var/cache/yum

COPY active.sh \
     /

RUN chmod a+rx /active.sh

ARG builddate_arg="(unknown)"
ARG version_arg="(unknown)"
ENV builddate="${builddate_arg}"
ENV version="${version_arg}"

LABEL org.label-schema.build-date="${builddate}" \
      org.label-schema.description="rclone-based data mover for Scribe" \
      org.label-schema.license="AGPL v3" \
      org.label-schema.name="scribe-mover-rclone" \
      org.label-schema.schema-version="1.0" \
      org.label-schema.vcs-ref="${version}" \
      org.label-schema.vcs-url="https://github.com/backube/scribe" \
      org.label-schema.vendor="Backube" \
      org.label-schema.version="${version}"

ENTRYPOINT [ "/bin/bash" ]
