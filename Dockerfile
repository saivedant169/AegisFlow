FROM golang:1.26.2-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o aegisflow ./cmd/aegisflow
RUN CGO_ENABLED=0 go build -o aegisctl ./cmd/aegisctl

FROM alpine:3.20
RUN apk add --no-cache ca-certificates curl jq bash
WORKDIR /app
COPY --from=builder /app/aegisflow .
COPY --from=builder /app/aegisctl /usr/local/bin/
COPY configs/demo.yaml /app/configs/aegisflow.yaml
COPY configs/policy-packs/ /app/configs/policy-packs/
COPY scripts/demo.sh /app/scripts/
RUN chmod +x /app/scripts/*.sh
EXPOSE 8080 8081 8082
ENTRYPOINT ["./aegisflow"]
