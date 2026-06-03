FROM ubuntu:24.04

ARG GO_VERSION=1.24.0

ENV DEBIAN_FRONTEND=noninteractive
ENV GOROOT=/usr/local/go
ENV GOPATH=/go
ENV PATH=/usr/local/go/bin:/go/bin:/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin

RUN apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl git bash rsync build-essential \
    && rm -rf /var/lib/apt/lists/*

RUN curl -fsSL "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz" -o /tmp/go.tgz \
    && rm -rf /usr/local/go \
    && tar -C /usr/local -xzf /tmp/go.tgz \
    && rm -f /tmp/go.tgz \
    && go version

WORKDIR /opt/update-server

COPY apps/update-server/docker/ubuntu/entrypoint.sh /usr/local/bin/wmlr-update-server-entrypoint

RUN chmod +x /usr/local/bin/wmlr-update-server-entrypoint \
    && mkdir -p /opt/update-server/deployments /go/pkg/mod /root/.cache/go-build

EXPOSE 19090

VOLUME ["/opt/wmlr", "/opt/update-server/deployments", "/go/pkg/mod", "/root/.cache/go-build"]

ENTRYPOINT ["/usr/local/bin/wmlr-update-server-entrypoint"]
