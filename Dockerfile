# Build stage
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod ./
# Copy ALL source files explicitly to prevent cache issues
COPY cmd/ ./cmd/
COPY internal/ ./internal/
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o humanmcp ./cmd/server/

# Runtime stage
FROM alpine:3.19
RUN apk add --no-cache ca-certificates tzdata iptables ip6tables
# Tailscale userspace mode — no TUN device needed on Fly.io
RUN apk add --no-cache tailscale && \
    mkdir -p /var/lib/tailscale /var/run/tailscale
WORKDIR /app
COPY --from=builder /app/humanmcp .
COPY start.sh .
RUN chmod +x start.sh && mkdir -p /data/content
ENV PORT=8080
ENV CONTENT_DIR=/data/content
ENV TS_USERSPACE=true
LABEL io.modelcontextprotocol.server.name="io.github.kapoost/humanmcp"
EXPOSE 8080
CMD ["./start.sh"]
