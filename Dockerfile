# Build stage
FROM golang:1.22.3-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o proxy .

# Runtime stage
FROM alpine:latest

RUN apk --no-cache add ca-certificates

WORKDIR /root/

# Copy the binary from builder
COPY --from=builder /app/proxy .

# Copy config.example.yml to config.yml
COPY --from=builder /app/config.example.yml ./config.yml

# Expose the default port from config
EXPOSE 6666

# Run the proxy
CMD ["./proxy"]