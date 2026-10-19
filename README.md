# pdf-extractext-api

Microservicio **API de borde** del sistema pdf-extractext. Recibe archivos PDF
por HTTP, aplica los límites del borde (tamaño máximo del cuerpo, validación de
formato) y orquesta la extracción de texto (`pdf-extractext-extractor`) y su
persistencia (`pdf-extractext-persistence`).

- **Stack:** Go 1.27 + Fiber v2 (fasthttp). Streaming 100% en memoria: cero
  volcados a disco en el hot path.
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
   └──► persistence (:8002) ──► MongoDB (:27017)   red `data`
```

El orquestador completo de contenedores (compose, redes, smoke checks) vive en
el repo hermano `pdf-extractext-infrastructure`; esta API está declarada y
expuesta allí.

## Variables de entorno

Se validan al arranque con fail-fast: si falta una obligatoria, el proceso no
levanta.

| Variable | Requerida | Default | Descripción |
|---|---|---|---|
| `PORT` | No | `:8000` | Puerto de escucha. **Formato `net/http`: con dos puntos iniciales** |
| `EXTRACTION_URL` | **Sí** | — | URL base del microservicio de extracción. Ej.: `http://extraction:8001` |
| `PERSISTENCE_URL` | **Sí** | — | URL base del microservicio de persistencia. Ej.: `http://persistence:8002`. La consumen `/upload-pdf` y el CRUD `/documents` |
| `MAX_FILE_SIZE_MB` | No | `20` | Límite duro del cuerpo HTTP en MB, aplicado vía `BodyLimit`. Cuenta el multipart COMPLETO (fronteras + headers + archivo) |

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
(504), `SERVICE_OVERLOADED` (503), `INTERNAL_ERROR` (500). Los errores de
upstream (4xx/503) se re-emiten preservando su `type`, `code` y `detail`; el
header `X-Request-ID` correlaciona los logs de los tres servicios.

## Desarrollo

```bash
# Calidad
make ci          # vet + lint + test
make vet         # go vet ./...
make test        # go test -race -cover ./...

# Ejecutar en local (requiere las URLs del extractor y del persistence)
export EXTRACTION_URL=http://localhost:8001
export PERSISTENCE_URL=http://localhost:8002
go run ./cmd/api
```

El binario `cmd/healthprobe` implementa el `HEALTHCHECK` de la imagen
distroless contra `GET /`.

## Docker

```bash
docker build -t pdf-extractext-api:local .
docker run --rm -p 8000:8000 \
  -e EXTRACTION_URL=http://extraction:8001 \
  -e PERSISTENCE_URL=http://persistence:8002 \
  pdf-extractext-api:local
```

Dentro de las redes del compose de `pdf-extractext-infrastructure`, los hosts
`extraction` y `persistence` resuelven por DNS de Docker.

## Estructura

```
pdf-extractext-api/
├── cmd/
│   ├── api/          # entrypoint: config + wiring de rutas
│   └── healthprobe/  # probe para la imagen distroless
├── internal/
│   ├── api/          # handlers: root, extract, upload_pdf, documents, errors
│   ├── core/         # config (fail-fast) + middleware X-Request-ID
│   └── services/     # clientes HTTP: extraction + persistence
└── docs/
    └── openapi.yaml  # especificación del contrato público
```

## CI

`.github/workflows/ci.yml` ejecuta `go vet`, `golangci-lint` y
`go test -race -cover` en cada push/PR a `main`.
