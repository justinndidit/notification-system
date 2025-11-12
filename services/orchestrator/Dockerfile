# services/orchestrator/Dockerfile
FROM golang:1.25-alpine AS builder

WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./
RUN go mod download

# Copy entire source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 GOOS=linux go build -o orchestrator cmd/orchestrator/main.go

# Final stage
FROM alpine:latest

# Add ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates wget

WORKDIR /root/

# Copy binary from builder
COPY --from=builder /app/orchestrator .

# Copy migrations (IMPORTANT - was missing!)
COPY --from=builder /app/internal/database/migrations ./internal/database/migrations

# Don't copy .env - use environment variables from docker-compose instead
# COPY --from=builder /app/.env .

EXPOSE 3002

CMD ["air"]