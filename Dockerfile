# syntax=docker/dockerfile:1
# Multi-stage: build con cache mounts y runtime distroless sin shell.
# En runtime son obligatorias EXTRACTION_URL y PERSISTENCE_URL (fail-fast al arrancar).

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
FROM gcr.io/distroless/static-debian12:nonroot@sha256:afa5c872c891853ca7fcf1f12c3edb23f7eeef36189728842dd51042ff57f7ab

COPY --from=build /out/api /api
COPY --from=build /out/healthprobe /healthprobe

# nonroot (uid 65532): el binario nunca necesita privilegios de escritura del sistema.
USER nonroot

# Puerto por defecto de la app (PORT=:8000).
EXPOSE 8000

HEALTHCHECK --interval=10s --timeout=3s --retries=3 \
  CMD ["/healthprobe"]

ENTRYPOINT ["/api"]
