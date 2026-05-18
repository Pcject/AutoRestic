# Dockerfile
# Stage 1: Build Go backend
FROM golang:1.25.10-alpine3.22 AS backend-builder

ENV GOTOOLCHAIN=auto

WORKDIR /build
COPY backend/go.mod backend/go.sum ./
RUN go mod download

COPY backend/ .
RUN CGO_ENABLED=0 go build -o autorestic .

# Stage 2: Build frontend
FROM --platform=$BUILDPLATFORM node:20-alpine AS frontend-builder

WORKDIR /app
COPY frontend/package*.json ./
RUN npm ci

COPY frontend/ .
RUN npm run build

# Stage 3: Final image with Nginx + Go API
FROM alpine:3.22

RUN apk add --no-cache ca-certificates fuse rclone restic nginx

WORKDIR /app

# Copy Go binary and migrations
COPY --from=backend-builder /build/autorestic .
COPY --from=backend-builder /build/migrations ./migrations

# Copy frontend static files
COPY --from=frontend-builder /app/dist /app/frontend

# Copy Nginx config
COPY nginx.conf /etc/nginx/http.d/default.conf

# Copy entrypoint
COPY scripts/entrypoint.sh /app/entrypoint.sh
RUN chmod +x /app/entrypoint.sh

# Create data directory
RUN mkdir -p /app/data /run/nginx

# Remove default nginx config if exists
RUN rm -f /etc/nginx/http.d/default.conf.bak

EXPOSE 80

STOPSIGNAL SIGTERM

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD wget -q -O - http://127.0.0.1/health >/dev/null || exit 1

ENTRYPOINT ["/app/entrypoint.sh"]
