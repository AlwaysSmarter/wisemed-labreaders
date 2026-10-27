FROM --platform=$BUILDPLATFORM golang:1.26.8 AS build
WORKDIR /src
COPY readersv3/go.mod readersv3/go.sum ./
RUN go mod download
COPY readersv3/core ./core
COPY readersv3/modules ./modules
COPY readersv3/shared ./shared
COPY readersv3/apps/runner ./apps/runner
ARG APP
COPY readersv3/apps/${APP}/main.go ./apps/${APP}/main.go
COPY readersv3/tools/docker-runtime ./tools/docker-runtime
ARG APP_VERSION
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X wisemed-labreaders/readersv3/shared/appmeta.Version=$APP_VERSION" \
    -o /out/app ./apps/${APP} \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X wisemed-labreaders/readersv3/shared/appmeta.Version=$APP_VERSION" \
    -o /out/docker-runtime ./tools/docker-runtime

# libc supports the dynamically loaded PKCS#11 driver; xmllint validates CNAS XML.
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates tzdata libxml2-utils opensc libpcsclite1 \
 && rm -rf /var/lib/apt/lists/* \
 && mkdir -p /data/deployments /opt/wisemed/bin /tmp \
 && chown -R 65532:65532 /data
RUN mkdir -p /data/data && chown 65532:65532 /data/data
ARG APP
ARG APP_VERSION
ENV WISEMED_CONTAINER=1 WISEMED_DOCKER_APP=${APP} HOME=/data
LABEL eu.wisemed.app=${APP} org.opencontainers.image.version=${APP_VERSION}
COPY --from=build /out/ /opt/wisemed/bin/
COPY readersv3/apps/${APP}/deployments/config.install.yaml /opt/wisemed/defaults/config.install.yaml
COPY readersv3/apps/${APP}/deployments/help/ /opt/wisemed/defaults/help/
COPY --chmod=755 readersv3/docker/entrypoint.sh /opt/wisemed/entrypoint.sh
WORKDIR /data
USER 65532:65532
HEALTHCHECK --interval=5s --timeout=5s --start-period=20s --retries=6 \
 CMD ["/opt/wisemed/bin/docker-runtime", "health", "/data/deployments/config.yaml"]
ENTRYPOINT ["/opt/wisemed/entrypoint.sh"]
CMD ["-config", "/data/deployments/config.yaml", "-showlog"]
