# Build stage
FROM golang:1.23-alpine AS builder

WORKDIR /app

# Cache dependencies.
COPY go.mod go.sum ./
RUN go mod download

# Copy source code.
COPY cmd/ cmd/
COPY internal/ internal/

# Build the binary.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o cerebro ./cmd/cerebro

# Runtime stage
FROM alpine:3.20

# CA certificates for TLS to api.cerebras.ai.
RUN apk --no-cache add ca-certificates

# Copy binary and default config.
COPY --from=builder /app/cerebro /usr/local/bin/cerebro
COPY config.example.yaml /etc/cerebro/config.yaml

EXPOSE 8080

# Default config path — override with CEREBRO_CONFIG env var.
ENV CEREBRO_CONFIG=/etc/cerebro/config.yaml

CMD ["cerebro"]
