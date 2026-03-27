FROM golang:1.26-alpine AS builder
RUN apk add --no-cache git ca-certificates
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /aegisflow ./cmd/aegisflow

FROM gcr.io/distroless/static:nonroot
COPY --from=builder /aegisflow /aegisflow
COPY --from=builder /app/configs/aegisflow.yaml /etc/aegisflow/aegisflow.yaml
EXPOSE 8080 8081
USER nonroot:nonroot
ENTRYPOINT ["/aegisflow"]
CMD ["--config", "/etc/aegisflow/aegisflow.yaml"]
