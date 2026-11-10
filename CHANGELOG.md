# Changelog

Todos los cambios notables de este proyecto se documentan en este archivo.

El formato está basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/),
y este proyecto adhiere a [Semantic Versioning](https://semver.org/lang/es/).

## [Unreleased]

### Added

- Cliente de HashiCorp Consul basado solo en `net/http` (`register`,
  `deregister`, `health` y `kv`) con timeout acotado y header ACL
  `X-Consul-Token` en todas las peticiones.
- Registro automático del servicio con ID `api-<hostname>`, IP real de la red
  `services`, Tags y Meta para Traefik v3.5, health check HTTP y reintentos con
  backoff. Ante `SIGTERM`/`SIGINT` se deregistra, se espera el drenaje de
  Traefik y recién después se apaga la aplicación.
- Configuración dinámica desde Consul KV (`pdf-extractext/config/api/`) con
  precedencia variable de entorno explícita > KV > default y comportamiento
  fail-open al arrancar.
- Health gate con caché TTL por servicio antes de invocar Extraction o
  Persistence; sin instancias sanas responde `503 DEPENDENCY_UNAVAILABLE` y con
  `CONSUL_HEALTH_FAIL_OPEN` controla el comportamiento ante caída de Consul.
- Persistence se consume a través del router interno de Traefik usando
  `PERSISTENCE_HOST` como header `Host`, igual que Extraction.
- Documentación de variables, flujo de registro, health gate y KV; cobertura
  del paquete `internal/consul` por encima del 80 %.
- SAGA de `/upload-pdf`: spool temporal con SHA-256, precheck por checksum en
  Persistence y fallo cerrado si la consulta no está disponible.
- Reintentos acotados (máximo 3, con `Retry-After`) para cuerpos rebobinables
  en Extraction y para Persistence; `/extract` mantiene un único intento.
- Tag `rate-limit@file` en el registro de Consul y puerto único `PORT` para el
  listener y el health check, eliminando `SERVICE_PORT`.

### Changed

- Streaming multipart real con `StreamRequestBody` y
  `DisablePreParseMultipartForm`: el PDF no se materializa antes del handler y
  el límite `MAX_FILE_SIZE_MB` se aplica durante la lectura.
- El health gate coalesce consultas concurrentes con `singleflight` y no cachea
  fallos fail-open.
- Shutdown unificado en 45 s con 5 s de drenaje; el compose usa
  `stop_grace_period: 55s`.
- `PATCH /documents/{id}` exige `filename` no vacío.
- El problem+json de un upstream se decodifica con un límite de 64 KiB.

### Fixed

- Fugas de goroutines en `ExtractionClient` al fallar la construcción del
  request o cancelarse el contexto.
- El access-log ahora registra el status real de los errores globales.
- `/extract` valida `content` no vacío y `page_count > 0`, incluyendo respuestas
  nil del extractor.
- `MAX_FILE_SIZE_MB` debe ser positivo y `ApplyKV` ya no deja configuración
  parcial si un valor es inválido.

## [1.0.0] - 2026-10-07

### Added

- Orquestador de extracción: endpoints `POST /extract` y `POST /upload-pdf`
  con streaming multipart, timeout estricto de 30s y propagación de
  `X-Request-ID` hacia el microservicio de extracción.
- CRUD de persistencia: cliente HTTP hacia el microservicio de persistencia
  (`Save`, `FindByID`, `FindAll`, `Update`, `Delete`) con mapeo de errores de
  dominio (`ErrNotFound`, `ErrConflict`) y endpoints públicos
  `GET/PATCH/DELETE /documents[/:id]`.
- Traducción global de errores (`CustomErrorHandler`) al formato estándar
  `{"code": "...", "message": "..."}`.
- Middlewares: generación y propagación de `X-Request-ID`, logging
  estructurado con `log/slog` (salida JSON).
- Quality gates en CI: `go vet`, `golangci-lint` (errcheck, govet,
  staticcheck, gosec, gocritic, ...) y tests con `-race -cover` vía Makefile y
  GitHub Actions.
- Healthprobe compilado (`cmd/healthprobe`) para la imagen distroless, con
  `HEALTHCHECK` nativo en el Dockerfile.
- Imagen Docker endurecida: multi-stage, digests pinneados, cache mounts de
  BuildKit y runtime distroless.
- Documentación OpenAPI 3.0.3 del contrato público en `docs/openapi.yaml`.
- Licencia MIT.
