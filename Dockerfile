# syntax=docker/dockerfile:1
# Dockerfile endurecido (Issue #12): multi-stage, digests pinneados,
# BuildKit cache mounts, runtime distroless y healthcheck nativo.
#
# Requisito en runtime (fail-fast al arrancar): EXTRACTION_URL y
# PERSISTENCE_URL deben proveerse con -e / --env-file.

# ---- Stage 1: build ----
# La directiva go de go.mod exige >= 1.27.1; se pinnea el digest igualmente.
FROM golang:1.27-alpine@sha256:8a5910f31396cd4d89662f56c68b3ae31d374308270a1c3bd96672ee5ed43414 AS build
WORKDIR /src

# Capas de dependencias primero; cache mounts aceleran rebuilds en CI.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    go mod download

COPY . .
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
    go build -trimpath -ldflags="-s -w" -o /out/healthprobe ./cmd/healthprobe

# ---- Stage 2: runtime (distroless, sin shell ni package manager) ----
FROM gcr.io/distroless/static-debian12@sha256:d75cdd72874d4790092fcb1b058493ecf6bb5bf2b2b897045b00ff01d91843f2

COPY --from=build /out/api /api
COPY --from=build /out/healthprobe /healthprobe

# Puerto por defecto de la app (PORT=:8000).
EXPOSE 8000

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD ["/healthprobe"]

ENTRYPOINT ["/api"]
