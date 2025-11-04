# Build stage
FROM golang:1.25.3-alpine3.22 AS builder

# Install build dependencies
RUN apk --no-cache add ca-certificates wget gcc musl-dev sqlite-dev

WORKDIR /app

# Download and install Ookla Speedtest CLI
RUN wget -O speedtest.tgz https://install.speedtest.net/app/cli/ookla-speedtest-1.2.0-linux-x86_64.tgz && \
    tar xzf speedtest.tgz && \
    mv speedtest /usr/local/bin/ && \
    rm speedtest.tgz

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application with CGO enabled for SQLite
RUN CGO_ENABLED=1 GOOS=linux go build -a -ldflags '-extldflags "-static"' -o proxy .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates sqlite

WORKDIR /root/

# Copy the binary and speedtest CLI from builder
COPY --from=builder /app/proxy .
COPY --from=builder /usr/local/bin/speedtest /usr/local/bin/

# Copy config.example.yml to config.yml
COPY --from=builder /app/config.example.yml ./config.yml

# Create data directory for database and GeoIP
RUN mkdir -p /root/data

# Expose ports (SOCKS5 and API)
EXPOSE 6666 8080

# Run the proxy
CMD ["./proxy"]