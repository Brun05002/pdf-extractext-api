# Changelog

Todos los cambios notables de este proyecto se documentan en este archivo.

El formato está basado en [Keep a Changelog](https://keepachangelog.com/es-ES/1.1.0/),
y este proyecto adhiere a [Semantic Versioning](https://semver.org/lang/es/).

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
