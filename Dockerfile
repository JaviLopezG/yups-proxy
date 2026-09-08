# Stage 1: Build the YUPS executable
FROM golang:latest AS builder

WORKDIR /src

# Download dependencies first for better layer caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source tree and compile static binary
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -trimpath -ldflags="-s -w" -o /bin/yups ./cmd/yups

# Stage 2: Minimal, secure runtime container
FROM alpine:latest

# Install CA certificates for outbound HTTPS requests (metadata scraping)
RUN apk --no-cache add ca-certificates tzdata

# Create dedicated non-privileged user and group
RUN addgroup -S yups && adduser -S yups -G yups

USER yups
WORKDIR /home/yups

# Copy statically compiled binary from builder stage
COPY --from=builder /bin/yups /usr/local/bin/yups

# Expose default HTTP service port
EXPOSE 8080

# Environment defaults
ENV PORT=8080 \
    HOST=0.0.0.0 \
    BASE_URL=https://yups.io \
    YUPS_ACCESS_LOG=true \
    CACHE_TTL=24h

ENTRYPOINT ["/usr/local/bin/yups"]
