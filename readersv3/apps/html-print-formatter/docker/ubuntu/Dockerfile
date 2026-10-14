# syntax=docker/dockerfile:1
# Build context: readersv3/
FROM --platform=$BUILDPLATFORM golang:1.25-bookworm AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY core/ ./core/
COPY shared/ ./shared/
COPY modules/ ./modules/
COPY apps/runner/ ./apps/runner/
COPY apps/html-print-formatter/main.go ./apps/html-print-formatter/main.go
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=0.0.0-dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X wisemed-labreaders/readersv3/shared/appmeta.Version=${VERSION}" \
    -o /out/html-print-formatter ./apps/html-print-formatter

FROM ubuntu:24.04 AS runtime
ARG TARGETARCH
ENV DEBIAN_FRONTEND=noninteractive \
    HOME=/home/wisemed \
    CHROME_PATH=/usr/bin/google-chrome-stable \
    PORT=19113 \
    MAX_CONCURRENT_RENDERS=2 \
    MAX_BODY_BYTES=4194304 \
    DEFAULT_RENDER_TIMEOUT_MS=120000
# Use Google's native .deb, avoiding Ubuntu's snap-based chromium launcher.
RUN case "${TARGETARCH}" in amd64|arm64) ;; *) echo "Unsupported architecture: ${TARGETARCH}" >&2; exit 1 ;; esac \
    && apt-get update \
    && apt-get install -y --no-install-recommends ca-certificates curl tini \
        fonts-dejavu-core fonts-liberation fonts-noto-core fonts-noto-color-emoji \
    && curl --fail --show-error --location --retry 3 \
        "https://dl.google.com/linux/direct/google-chrome-stable_current_${TARGETARCH}.deb" \
        -o /tmp/google-chrome.deb \
    && apt-get install -y --no-install-recommends /tmp/google-chrome.deb \
    && chmod 4755 /opt/google/chrome/chrome-sandbox \
    && rm -f /tmp/google-chrome.deb \
    && rm -rf /var/lib/apt/lists/* \
    && groupadd --gid 10001 wisemed \
    && useradd --uid 10001 --gid wisemed --create-home --shell /usr/sbin/nologin wisemed

COPY --from=build /out/html-print-formatter /opt/wisemed/bin/html-print-formatter
COPY apps/html-print-formatter/deployments/config.install.yaml /opt/wisemed/defaults/config.install.yaml
COPY apps/html-print-formatter/deployments/help/ /opt/wisemed/defaults/help/
COPY --chmod=755 apps/html-print-formatter/docker/ubuntu/entrypoint.sh /usr/local/bin/html-print-formatter-entrypoint
# Bind inside the container; the host's published port controls exposure.
RUN sed -i 's/127\.0\.0\.1:19113/0.0.0.0:19113/g; s|help_dir: ./help|help_dir: /opt/wisemed/defaults/help|' \
        /opt/wisemed/defaults/config.install.yaml \
    && mkdir -p /app/deployments \
    && chown -R wisemed:wisemed /app /opt/wisemed/bin
WORKDIR /app
USER 10001:10001
VOLUME ["/app/deployments"]
EXPOSE 19113
STOPSIGNAL SIGTERM
HEALTHCHECK --interval=30s --timeout=5s --start-period=45s --retries=3 \
    CMD curl --fail --silent "http://127.0.0.1:${PORT}/health" >/dev/null || exit 1
ENTRYPOINT ["/usr/bin/tini", "--", "/usr/local/bin/html-print-formatter-entrypoint"]
CMD ["-config", "/app/deployments/config.yaml", "-showlog"]
