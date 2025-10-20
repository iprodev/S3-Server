# Multi-stage build for minimal image size
FROM golang:1.21-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /build

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build the binary with optimizations
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(cat VERSION)" \
    -o s3_server \
    ./cmd/s3_server

# Final minimal image
FROM alpine:3.18

# Install runtime dependencies
RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 s3user && \
    adduser -D -u 1000 -G s3user s3user

# Set working directory
WORKDIR /app

# Copy binary from builder
COPY --from=builder /build/s3_server /app/s3_server

# Create necessary directories
RUN mkdir -p /app/data /app/tmp /app/lifecycle && \
    chown -R s3user:s3user /app

# Switch to non-root user
USER s3user

# Expose ports
# 8080: Main API
# 9091: Metrics
EXPOSE 8080 9091

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Default command (can be overridden)
ENTRYPOINT ["/app/s3_server"]
CMD ["-mode", "gateway", "-listen", ":8080"]
