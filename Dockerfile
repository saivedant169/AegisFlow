FROM --platform=$BUILDPLATFORM golang:1.27.0-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
ARG TARGETOS
ARG TARGETARCH
ARG VERSION=dev
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o aegisflow ./cmd/aegisflow
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-$(go env GOARCH)} go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o aegisctl ./cmd/aegisctl

FROM alpine:3.24
RUN apk add --no-cache ca-certificates curl jq bash
WORKDIR /app
COPY --from=builder /app/aegisflow .
COPY --from=builder /app/aegisctl /usr/local/bin/
COPY configs/demo.yaml /app/configs/aegisflow.yaml
COPY configs/policy-packs/ /app/configs/policy-packs/
COPY scripts/demo.sh /app/scripts/
RUN chmod +x /app/scripts/*.sh
HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 CMD curl -f http://127.0.0.1:8080/health || exit 1
EXPOSE 8080 8081 8082
ENTRYPOINT ["./aegisflow"]
