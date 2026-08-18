# syntax=docker/dockerfile:1

# --- Stage 1: build the SPA ---
FROM oven/bun:1.4.0 AS frontend
WORKDIR /app
COPY web/package.json web/bun.lock ./web/
RUN cd web && bun install --frozen-lockfile
COPY web ./web
# vite outputs to ../internal/web/dist; ensure the parent exists.
RUN mkdir -p internal/web && cd web && bun run build

# --- Stage 2: build the static Go binary ---
FROM golang:1.26-bookworm AS backend
WORKDIR /app
COPY go.mod go.sum ./
# Copy only what the server build needs (generated code is committed).
COPY cmd ./cmd
COPY internal ./internal
COPY gen ./gen
COPY --from=frontend /app/internal/web/dist ./internal/web/dist
ARG VERSION=dev
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -tags netgo,osusergo \
    -ldflags "-s -w -X main.version=${VERSION}" \
    -o /server ./cmd/server

# --- Stage 3: minimal runtime (no glibc) ---
FROM gcr.io/distroless/static-debian12:nonroot
WORKDIR /home/nonroot
COPY --from=backend /server /server
EXPOSE 8080
USER 65532:65532
# DB_DRIVER/DB_DSN and JWT_SECRET are provided at runtime (docker compose
# defaults the app to PostgreSQL). With no DB_DRIVER set, it falls back to
# file-based SQLite, which is ephemeral unless a volume is mounted.
ENTRYPOINT ["/server"]
