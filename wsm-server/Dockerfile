# Build from the repository root:
# docker build -f wsm-server/Dockerfile -t wisemed-wsm:local .
# Run the compiler natively on the builder; cross-compile for the target image.
FROM --platform=$BUILDPLATFORM golang:1.26.8 AS build
ARG WSM_VERSION
WORKDIR /src/wsm-server
COPY wsm-server/go.mod wsm-server/go.sum ./
RUN go mod download
COPY wsm-server/cmd ./cmd
COPY wsm-server/internal ./internal
ARG TARGETOS
ARG TARGETARCH
RUN version="${WSM_VERSION:-$(date -u +%Y%m%d-%H%M%S)}" \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -ldflags "-X main.buildVersion=$version" -o /out/wsm-server ./cmd/wsm-server \
 && CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath -o /out/wsmctl ./cmd/wsmctl
COPY readersv3/modules/localhttp/ui/index.html readersv3/modules/localhttp/ui/app.js readersv3/modules/localhttp/ui/styles.css readersv3/modules/localhttp/ui/wss-remote.js /out/control/

FROM scratch
COPY --from=build /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt
COPY --from=build /out/wsm-server /out/wsmctl /opt/wsm-server/bin/
COPY --from=build /out/control/ /opt/wsm-server/control/
WORKDIR /opt/wsm-server
USER 65532:65532
EXPOSE 8443
EXPOSE 8090
ENTRYPOINT ["/opt/wsm-server/bin/wsm-server"]
CMD ["-config", "/etc/wsm-server/config.yaml"]
