# Build stage
FROM golang:1.26-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make

# Set working directory
WORKDIR /app

# Copy go mod files
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download && go mod verify

# Copy source code
COPY . .

# Build the binary
RUN CGO_ENABLED=0 \
    go build -v -ldflags="-s -w" -o /mapart .

# Final stage
FROM alpine:latest

# Install runtime dependencies
RUN apk --no-cache add ca-certificates tzdata

# Create non-root user
RUN addgroup -g 1000 mapart && \
    adduser -D -u 1000 -G mapart mapart

# Set working directory
WORKDIR /workspace

# Copy binary from builder
COPY --from=builder /mapart /usr/local/bin/mapart

# Create directories
RUN mkdir -p /workspace/input /workspace/output && \
    chown -R mapart:mapart /workspace

# Switch to non-root user
USER mapart

# Set default volumes
VOLUME ["/workspace/input", "/workspace/output"]

# Set entrypoint
ENTRYPOINT ["mapart"]

# Default command (show help)
CMD ["-h"]

# Labels
LABEL maintainer="mapartscript"
LABEL description="Convert images to Minecraft map art with NBT schematics"
LABEL version="1.0"

# Health check (optional - requires image input)
# HEALTHCHECK --interval=30s --timeout=3s \
#   CMD mapart -h || exit 1

# Example usage:
# docker run --rm -v $(pwd)/input:/workspace/input:ro -v $(pwd)/output:/workspace/output mapartscript -i /workspace/input/image.png -o /workspace/output/result.png -x 5 -y 3
