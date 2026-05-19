FROM golang:1.24 AS build
WORKDIR /src
COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    cd /src && go build -o /out/update-server ./apps/update-server

FROM debian:bookworm-slim
WORKDIR /app
COPY --from=build /out/update-server /app/update-server
COPY output/update-server/deployments /app/deployments
EXPOSE 19090
VOLUME ["/app/deployments"]
ENTRYPOINT ["/app/update-server","-config","/app/deployments/config.yaml"]
