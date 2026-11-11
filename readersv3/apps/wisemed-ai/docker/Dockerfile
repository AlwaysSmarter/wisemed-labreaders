FROM node:22-bookworm-slim AS frontend
WORKDIR /src/apps/wisemed-ai/web
COPY apps/wisemed-ai/web/package*.json ./
RUN npm ci --no-audit --no-fund
COPY apps/wisemed-ai/web/ ./
RUN npm run build
FROM golang:1.24-bookworm AS backend
ARG VERSION=1.3.0
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . ./
COPY --from=frontend /src/modules/wisemedai/web/dist ./modules/wisemedai/web/dist
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-X wisemed-labreaders/readersv3/shared/appmeta.Version=${VERSION} -X wisemed-labreaders/readersv3/shared/appmeta.VersionMarker=WISEMED_APP_VERSION=${VERSION}" -o /wisemed-ai ./apps/wisemed-ai
FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl && rm -rf /var/lib/apt/lists/* && useradd -u 10001 -m -s /usr/sbin/nologin wisemed-ai
WORKDIR /app
COPY --from=backend /wisemed-ai /app/wisemed-ai
COPY apps/wisemed-ai/deployments/ /defaults/
COPY apps/wisemed-ai/docker/entrypoint.sh /app/entrypoint.sh
RUN mkdir -p /app/deployments && chown -R wisemed-ai:wisemed-ai /app && chmod +x /app/entrypoint.sh
USER wisemed-ai
EXPOSE 19120
HEALTHCHECK --interval=10s --timeout=3s --start-period=30s CMD curl -fsS http://127.0.0.1:19120/ready/ai || exit 1
ENTRYPOINT ["/app/entrypoint.sh"]
