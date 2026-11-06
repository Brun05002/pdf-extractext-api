# pdf-extractext-api

Microservicio **API de borde** del sistema pdf-extractext. Recibe archivos PDF
por HTTP, aplica los límites del borde (tamaño máximo del cuerpo, validación de
formato) y orquesta la extracción de texto (`pdf-extractext-extractor`) y su
persistencia (`pdf-extractext-persistence`).

- **Stack:** Go 1.27 + Fiber v2 (fasthttp). El cuerpo multipart se reenvía al
  extractor en streaming; el default `MAX_FILE_SIZE_MB=16` se fija justo en el
  umbral en que fasthttp empieza a volcar a disco (16 MiB), de modo que el
  hot path no toca disco.
- **Estado:** implementado y declarado en el compose de
  `pdf-extractext-infrastructure` (servicio `api`, ruteado por Traefik). Ver
  [CHANGELOG.md](CHANGELOG.md) (v1.0.0).

## Arquitectura

```
Traefik (:80)                        red `services`
   │
   ▼
pdf-extractext-api (:8000)
   ├──► extraction  (:8001)          extracción de texto (Python/PyMuPDF)
   ├──► persistence (:8002) ──► MongoDB (:27017)   red `data`
   └──► Consul      (:8500)          registro, health checks y KV
```

El orquestador completo de contenedores (compose, redes, smoke checks) vive en
el repo hermano `pdf-extractext-infrastructure`; esta API está declarada y
expuesta allí.

## Variables de entorno

La configuración se resuelve con precedencia **variable de entorno explícita >
Consul KV > default**. Si `CONSUL_ENABLED=true` y Consul no responde al
arrancar, se loguea un warning y el servicio arranca con env/defaults
(fail-open).

| Variable | Default | Descripción |
|---|---|---|
| `PORT` | `:8000` | Puerto de escucha. **Formato `net/http`: con dos puntos iniciales** |
| `EXTRACTION_URL` | `http://traefik:8090` | URL base del extractor por el entrypoint interno de Traefik. `http://extraction:8001` solo para uso aislado sin Traefik |
| `EXTRACTION_HOST` | `extraction.pdf-extractext.localhost` | Host header para el router interno de Traefik hacia Extraction |
| `PERSISTENCE_URL` | `http://traefik:8090` | URL base de Persistence por el entrypoint interno de Traefik. La consumen `/upload-pdf` y el CRUD `/documents` |
| `PERSISTENCE_HOST` | `persistence.pdf-extractext.localhost` | Host header para el router interno de Traefik hacia Persistence |
| `MAX_FILE_SIZE_MB` | `16` | Límite duro del cuerpo HTTP en MB, aplicado vía `BodyLimit`. Cuenta el multipart COMPLETO |
| `CONSUL_ENABLED` | `false` | Habilita registro, health gate y lectura de KV |
| `CONSUL_HTTP_ADDR` | `http://consul:8500` | Dirección HTTP del agente Consul |
| `CONSUL_HTTP_TOKEN` | — | Token ACL enviado en `X-Consul-Token` en todas las peticiones |
| `SERVICE_NAME` | `pdf-extractext-api` | Nombre con el que se registra el servicio |
| `SERVICE_PORT` | `8000` | Puerto publicado en el registro de Consul |
| `SERVICE_HEALTH_PATH` | `/` | Ruta del check HTTP que ejecuta Consul |
| `CONSUL_HEALTH_CACHE_TTL` | `3s` | TTL del caché en memoria del health gate |
| `CONSUL_HEALTH_FAIL_OPEN` | `true` | Si Consul no responde, permite la llamada al upstream (si es `false`, responde 503) |

## Integración con Consul y Traefik

### Registro y ciclo de vida

- El ID es `api-<hostname>`; la `Address` se obtiene abriendo un socket UDP
  contra Consul para tomar la IP real del contenedor en la red `services`
  (nunca loopback).
- Los `Tags` de Traefik v3.5 habilitan el servicio (`traefik.enable=true`),
  declaran el router `api` con `Host(`pdf-extractext.localhost`)`,
  `entrypoints=https`, `tls=true` y el puerto `8000`; las mismas claves se
  replican en `Meta` junto con `version` y `commit`.
- El check HTTP apunta a `http://<address>:8000<health_path>` con
  `Interval=10s`, `Timeout=2s` y `DeregisterCriticalServiceAfter=1m`.
- El registro ocurre después de bindear el listener; si Consul falla se
  reintenta con backoff exponencial sin impedir el arranque.
- Ante `SIGTERM`/`SIGINT` el orden es: deregister en Consul → esperar 5 s para
  el drenaje de Traefik → `app.ShutdownWithTimeout`.

### Health gate

Antes de llamar a Extraction o Persistence se consulta
`GET /v1/health/service/<nombre>?passing=true`. El resultado se cachea en
memoria con `sync.RWMutex` durante `CONSUL_HEALTH_CACHE_TTL` (default `3s`).

- Si no hay instancias sanas se responde `503` problem+json con code
  `DEPENDENCY_UNAVAILABLE` y **no** se invoca al upstream.
- Si Consul no responde: con `CONSUL_HEALTH_FAIL_OPEN=true` (default) se
  permite la llamada y se loguea un warning; con `false`, se responde `503`.
- El gate envuelve `ExtractionClient` y `PersistenceRepository` mediante
  decoradores, sin cambiar sus interfaces.

### Configuración dinámica (KV)

Con `CONSUL_ENABLED=true` se lee
`GET /v1/kv/pdf-extractext/config/api/?recurse` y se decodifican los valores
Base64. Las claves aplicables son `EXTRACTION_URL`, `EXTRACTION_HOST`,
`PERSISTENCE_URL`, `PERSISTENCE_HOST` y `MAX_FILE_SIZE_MB`; las desconocidas se
ignoran y los secretos (`MONGO_URI`, `CONSUL_HTTP_TOKEN`) nunca se leen desde
KV. La precedencia es variable de entorno explícita > KV > default y, si KV
falla, se usan env/defaults con un warning.

## Endpoints

| Método | Ruta | Descripción |
|---|---|---|
| `GET` | `/` | Health público (lo consume el `healthprobe` de Docker). |
| `POST` | `/extract` | Extrae texto: reenvía el PDF al extractor (contrato C1). Stateless. |
| `POST` | `/upload-pdf` | Extrae y **persiste** (C1 + C2). Devuelve el documento creado. |
| `GET` | `/documents` | Lista los documentos persistidos. |
| `GET` | `/documents/{id}` | Obtiene un documento. |
| `PATCH` | `/documents/{id}` | Renombra un documento (`filename`). |
| `DELETE` | `/documents/{id}` | Elimina un documento. |

### `GET /`

Health. Lo consume el `healthprobe` del contenedor y los balanceadores.

```json
{ "service": "pdf-extractext-api", "status": "UP", "time": "2026-10-18T12:00:00Z" }
```

### `POST /extract`

Recibe el PDF (`multipart/form-data`, campo `file`, extensión `.pdf`) y lo
reenvía al extractor streameando en memoria. No persiste nada.

- **Response 200 (contrato C1 público):**

```json
{ "content": "<texto extraído, Markdown>", "page_count": 42 }
```

| Status | Causa |
|---|---|
| 400 | Falta el campo multipart `file` |
| 413 | El cuerpo supera `MAX_FILE_SIZE_MB` (rechazado por `BodyLimit`) |
| 415 | El archivo no tiene extensión `.pdf` |
| 422 | El extractor rechazó el contenido (`INVALID_PDF_CONTENT`), propagado tal cual |
| 500 | No se pudo abrir el archivo subido |
| 502 | Fallo de red/timeout o `500` del extractor |
| 503 | Extractor saturado (`SERVICE_OVERLOADED`), propagado con su `Retry-After` |

### `POST /upload-pdf`

Extrae (C1) y persiste (C2) en una sola llamada, con timeout estricto de 30 s y
propagación de `X-Request-ID` hacia el extractor.

- **Response 201:** el documento persistido
  `{ "id": "...", "filename": "...", "extracted_text": "...", "checksum": "..." }`.
- **409:** ya existe un documento con el mismo `checksum` (barrera anti-TOCTOU
  del índice único).

| Status | Causa |
|---|---|
| 400 | Falta el campo multipart `file` |
| 413 | El cuerpo supera `MAX_FILE_SIZE_MB` |
| 415 | El archivo no tiene extensión `.pdf` |
| 422 | Contenido PDF inválido, propagado del extractor |
| 502 | Fallo de red/timeout del extractor o de persistencia |
| 503 | Extractor saturado, propagado con su `Retry-After` |
| 504 | El extractor excedió el timeout |

### `/documents`

- `GET /documents` → `200` con la lista de
  `{id, filename, extracted_text, checksum}`.
- `GET /documents/{id}` → `200` con el documento; `404` si no existe; `400` si el
  id no es hexadecimal de 24 caracteres (validado antes de llamar a Persistence).
- `PATCH /documents/{id}` → `200`; renombra (`{"filename": "..."}`); `400` si el
  cuerpo o el id son inválidos; `404`; `409`.
- `DELETE /documents/{id}` → `204`; `400` si el id es inválido; `404`.

### Formato de error (RFC 9457)

Todos los errores responden `Content-Type: application/problem+json` con los
campos `type` (URN `urn:pdf-extractext:<slug>`), `title`, `status`, `detail` +
el miembro de extensión `code`:

```json
{
  "type": "urn:pdf-extractext:not-found",
  "title": "Recurso no encontrado",
  "status": 404,
  "detail": "documento no encontrado",
  "code": "NOT_FOUND"
}
```

Códigos: `INVALID_REQUEST` (400), `NOT_FOUND` (404), `METHOD_NOT_ALLOWED`
(405), `CONFLICT` (409), `UNSUPPORTED_MEDIA_TYPE` (415), `REQUEST_TOO_LARGE`
(413), `INVALID_PDF_CONTENT` (422), `BAD_GATEWAY` (502), `GATEWAY_TIMEOUT`
(504), `SERVICE_OVERLOADED` (503), `DEPENDENCY_UNAVAILABLE` (503),
`INTERNAL_ERROR` (500). Los errores de
upstream (4xx/503) se re-emiten preservando su `type`, `code` y `detail`; el
header `X-Request-ID` correlaciona los logs de los tres servicios.

## Desarrollo

```bash
# Calidad
make ci          # vet + lint + test
make vet         # go vet ./...
make test        # go test -race -cover ./...

# Ejecutar en local dentro de la red del stack (Traefik enruta por Host).
export EXTRACTION_URL=http://traefik:8090
export EXTRACTION_HOST=extraction.pdf-extractext.localhost
export PERSISTENCE_URL=http://traefik:8090
export PERSISTENCE_HOST=persistence.pdf-extractext.localhost
go run ./cmd/api

# Aislado sin Traefik: usar http://extraction:8001 y http://persistence:8002
# con EXTRACTION_HOST/PERSISTENCE_HOST vacíos.
```

El binario `cmd/healthprobe` implementa el `HEALTHCHECK` de la imagen
distroless contra `GET /`.

## Docker

```bash
docker build -t pdf-extractext-api:local .
# Ruta principal del stack (Traefik enruta por Host hacia Extraction):
docker run --rm -p 8000:8000 \
  -e EXTRACTION_URL=http://traefik:8090 \
  -e PERSISTENCE_URL=http://traefik:8090 \
  -e PERSISTENCE_HOST=persistence.pdf-extractext.localhost \
  pdf-extractext-api:local
```

Dentro de las redes del compose de `pdf-extractext-infrastructure`, los hosts
`traefik`, `extraction` y `persistence` resuelven por DNS de Docker. Sin
Traefik (aislado), usar `EXTRACTION_URL=http://extraction:8001`.

## Estructura

```
pdf-extractext-api/
├── cmd/
│   ├── api/          # entrypoint: config + wiring de rutas
│   └── healthprobe/  # probe para la imagen distroless
├── internal/
│   ├── api/          # handlers: root, extract, upload_pdf, documents, errors
│   ├── consul/       # cliente net/http: registro, health, KV y ciclo de vida
│   ├── core/         # config (env > KV > default) + middleware X-Request-ID
│   └── services/     # clientes HTTP + health gate: extraction/persistence
└── docs/
    └── openapi.yaml  # especificación del contrato público
```

## CI

`.github/workflows/ci.yml` ejecuta `go vet`, `golangci-lint` y
`go test -race -cover` en cada push/PR a `main`.
