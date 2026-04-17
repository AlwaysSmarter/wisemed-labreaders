FROM golang:1.23-alpine AS builder
WORKDIR /src
COPY . .
RUN go mod tidy && go build -o /out/wisemedws ./cmd/server

FROM alpine:3.20
WORKDIR /app
COPY --from=builder /out/wisemedws /app/wisemedws
COPY deployments/wisemedws.example.yaml /app/deployments/wisemedws.yaml
EXPOSE 8090
CMD ["/app/wisemedws", "-config", "/app/deployments/wisemedws.yaml"]
