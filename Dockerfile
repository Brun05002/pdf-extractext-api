# Dockerfile BASE (bootstrap): build estático multi-stage + runtime no-root.
# El endurecimiento (distroless/optimización fina de capas, pinned digest,
# build cache mounts) es ownership de la issue #8.
#
# Requisito en runtime (fail-fast al arrancar): EXTRACTION_URL y
# PERSISTENCE_URL deben proveerse con -e / --env-file.

# ---- Stage 1: build ----
FROM golang:1.22-alpine AS build
WORKDIR /src

# Capas de dependencias primero para aprovechar el cache de Docker.
COPY go.mod go.sum ./
RUN go mod download

COPY . .
RUN CGO_ENABLED=0 GOOS=linux \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api

# ---- Stage 2: runtime ----
FROM alpine:3.20
RUN adduser -D -u 10001 appuser
USER appuser
WORKDIR /app

COPY --from=build /out/api /usr/local/bin/api

# Puerto por defecto de la app (PORT=:3000).
EXPOSE 3000

# Probe informal sobre GET / (única ruta viva hoy en el binario).
# ${PORT} conserva el formato ":3000" que usa la app.
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
  CMD wget -q -O /dev/null "http://127.0.0.1${PORT:-:3000}/" || exit 1

ENTRYPOINT ["api"]
