# syntax=docker/dockerfile:1

# ---- Build stage ----
FROM golang:1.23 AS builder
WORKDIR /src

# Pre-cache modules
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy source
COPY . .

# Build the binary for the target platform (defaults are provided by BuildKit)
ARG TARGETOS
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags='-s -w' -o /out/otlp-log-processor ./cmd/otlp-log-processor

# ---- Runtime stage ----
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /app
COPY --from=builder /out/otlp-log-processor /app/otlp-log-processor

EXPOSE 4317
ENTRYPOINT ["/app/otlp-log-processor"]
CMD ["-listenAddr", ":4317"]
