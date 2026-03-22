# ── Stage 1: build frontend ───────────────────────────────────────────────────
FROM node:22-alpine AS frontend
WORKDIR /app/frontend
COPY frontend/package*.json ./
RUN npm ci
COPY frontend/ ./
# vite.config.js sets outDir: '../web/dist'
RUN npm run build

# ── Stage 2: build Go binary ──────────────────────────────────────────────────
FROM golang:1.25-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# overlay the freshly built frontend assets
COPY --from=frontend /app/web/dist ./web/dist
RUN go build -o dep-health .

# ── Stage 3: minimal runtime ──────────────────────────────────────────────────
FROM alpine:latest
RUN apk --no-cache add ca-certificates git
WORKDIR /app
COPY --from=builder /app/dep-health .

EXPOSE 8080
# Railway injects $PORT; fall back to 8080 for local docker runs
CMD sh -c "./dep-health serve --port ${PORT:-8080} --db /app/dep-health.db"
