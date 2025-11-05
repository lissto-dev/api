# Build stage
FROM golang:1.24-alpine AS builder

# Set working directory
WORKDIR /app

# Install git and ca-certificates
RUN apk add --no-cache git ca-certificates

# Configure Git for private modules (build arg for token)
ARG GITHUB_TOKEN
RUN if [ -n "$GITHUB_TOKEN" ]; then \
      git config --global credential.helper store && \
      echo "https://x-access-token:${GITHUB_TOKEN}@github.com" > ~/.git-credentials && \
      chmod 600 ~/.git-credentials; \
    fi && \
    git config --global url."https://github.com/lissto-dev/controller-playground".insteadOf "https://github.com/lissto-dev/controller"

# Set GOPRIVATE for private repositories
ENV GOPRIVATE=github.com/lissto-dev/*
ENV GOWORK=off

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o main ./cmd/server

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create non-root user with specific UID (65532 is standard nonroot user)
RUN adduser -D -u 65532 -s /bin/sh nonroot

# Set working directory
WORKDIR /app

# Copy the binary from builder stage
COPY --from=builder --chown=65532:65532 /app/main .

# Copy API keys example (for reference)
COPY --from=builder --chown=65532:65532 /app/api-keys.example.yaml .

# Switch to non-root user
USER 65532

# Expose port
EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

# Run the application
CMD ["./main"]


